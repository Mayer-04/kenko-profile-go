package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoStore implementa profile.Store sobre MongoDB con transacciones
// multidocumento (perfil + processed + outbox en una sola transacción).
type MongoStore struct {
	client   *mongodriver.Client
	db       *mongodriver.Database
	profiles *mongodriver.Collection
	outbox   *mongodriver.Collection
	proc     *mongodriver.Collection
}

// NewMongoStore construye el store sobre una conexión existente.
func NewMongoStore(c *platformmongo.Client) *MongoStore {
	return &MongoStore{
		client:   c.Raw,
		db:       c.DB,
		profiles: c.DB.Collection(platformmongo.ProfilesCollection),
		outbox:   c.DB.Collection(platformmongo.OutboxCollection),
		proc:     c.DB.Collection(platformmongo.ProcessedCollection),
	}
}

func uuidBinary(s string) (bson.Binary, error) {
	id, err := uuid.FromString(s)
	if err != nil {
		return bson.Binary{}, err
	}
	return platformmongo.UUIDToBinary(id), nil
}

func profileCreatedPayload(p profile.Profile, meta map[string]string) bson.M {
	return bson.M{
		"metadata": bson.M{
			"eventId":       meta["eventId"],
			"eventVersion":  "1",
			"occurredAt":    meta["occurredAt"],
			"correlationId": meta["correlationId"],
		},
		"data": bson.M{
			"userId":   meta["userId"],
			"document": bson.M{"type": p.DocumentType, "number": p.DocumentNumber},
			"person": bson.M{
				"firstName": p.FirstName,
				"lastName":  p.LastName,
				"birthDate": p.BirthDate.Format("2006-01-02"),
				"gender":    p.Gender,
			},
			"contact": bson.M{
				"email":         p.Email,
				"emailVerified": p.EmailVerified,
				"phone":         p.Phone,
			},
			"address": bson.M{"line": p.AddressLine, "city": p.City},
		},
	}
}

// CreateFromRegistration implementa 4.1 paso 4: idempotencia + escritura
// atómica del perfil y el outbox en la misma transacción.
func (s *MongoStore) CreateFromRegistration(ctx context.Context, evt events.UserRegistered, now time.Time) (string, bool, error) {
	userBin, err := uuidBinary(evt.Data.UserID)
	if err != nil {
		return "", false, fmt.Errorf("%w: userId", profile.ErrInvalidPayload)
	}
	birth, _ := time.Parse("2006-01-02", evt.Data.Person.BirthDate)
	emailAt, _ := time.Parse(time.RFC3339, evt.Metadata.OccurredAt)
	createdAt := emailAt.UTC()
	if createdAt.IsZero() {
		createdAt = now.UTC()
	}
	p := profile.Profile{
		ID:              userBin,
		DocumentType:    evt.Data.Document.Type,
		DocumentNumber:  evt.Data.Document.Number,
		FirstName:       evt.Data.Person.FirstName,
		LastName:        evt.Data.Person.LastName,
		BirthDate:       birth,
		Gender:          evt.Data.Person.Gender,
		Email:           evt.Data.Contact.Email,
		EmailVerified:   evt.Data.Contact.EmailVerified,
		EmailOccurredAt: emailAt.UTC(),
		Phone:           evt.Data.Contact.Phone,
		AddressLine:     evt.Data.Address.Line,
		City:            evt.Data.Address.City,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	newEventID := platformmongo.MustUUID7().String()
	payload := profileCreatedPayload(p, map[string]string{
		"eventId":       newEventID,
		"occurredAt":    createdAt.Format(time.RFC3339),
		"correlationId": evt.Metadata.CorrelationID,
		"userId":        evt.Data.UserID,
	})
	outbox := profile.OutboxEvent{
		ID:            platformmongo.UUIDToBinary(platformmongo.MustUUID7()),
		EventType:     "ProfileCreated",
		AggregateID:   userBin,
		CorrelationID: evt.Metadata.CorrelationID,
		EventID:       newEventID,
		Payload:       payload,
		CreatedAt:     now.UTC(),
		Attempts:      0,
	}

	sess, err := s.client.StartSession()
	if err != nil {
		return "", false, err
	}
	defer sess.EndSession(ctx)

	var duplicated bool
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		duplicated = false
		// Idempotencia D4-A: reclamar eventId antes de insertar.
		res := s.proc.FindOne(ctx, bson.M{"event_id": evt.Metadata.EventID})
		if res.Err() == nil {
			duplicated = true
			return nil, nil
		}
		if res.Err() != mongodriver.ErrNoDocuments {
			return nil, res.Err()
		}
		if _, err := s.profiles.InsertOne(ctx, p); err != nil {
			if mongodriver.IsDuplicateKeyError(err) {
				// Perfil ya existe para este userId: tratar como duplicado
				// (segunda entrega del mismo registro) sin crear outbox.
				if _, ierr := s.proc.InsertOne(ctx, profile.ProcessedEvent{
					EventID:     evt.Metadata.EventID,
					ProcessedAt: now.UTC(),
				}); ierr != nil && !mongodriver.IsDuplicateKeyError(ierr) {
					return nil, ierr
				}
				duplicated = true
				return nil, nil
			}
			return nil, err
		}
		if _, err := s.outbox.InsertOne(ctx, outbox); err != nil {
			return nil, err
		}
		if _, err := s.proc.InsertOne(ctx, profile.ProcessedEvent{
			EventID:     evt.Metadata.EventID,
			ProcessedAt: now.UTC(),
		}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return "", false, err
	}
	return evt.Data.UserID, duplicated, nil
}

// ApplyEmailChanged implementa D8-B: actualiza email solo si el evento es
// más reciente que el último aplicado. Idempotente por eventId.
func (s *MongoStore) ApplyEmailChanged(ctx context.Context, evt events.UserEmailChanged) (bool, bool, error) {
	userBin, err := uuidBinary(evt.Data.UserID)
	if err != nil {
		return false, false, fmt.Errorf("%w: userId", profile.ErrInvalidPayload)
	}
	evtAt, _ := time.Parse(time.RFC3339, evt.Data.OccurredAt)
	sess, err := s.client.StartSession()
	if err != nil {
		return false, false, err
	}
	defer sess.EndSession(ctx)

	var updated, duplicated bool
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		updated, duplicated = false, false
		if res := s.proc.FindOne(ctx, bson.M{"event_id": evt.Metadata.EventID}); res.Err() == nil {
			duplicated = true
			return nil, nil
		} else if res.Err() != mongodriver.ErrNoDocuments {
			return nil, res.Err()
		}
		updRes, err := s.profiles.UpdateOne(ctx,
			bson.M{"_id": userBin, "email_occurred_at": bson.M{"$lt": evtAt.UTC()}},
			bson.M{"$set": bson.M{
				"email":             evt.Data.Email,
				"email_verified":    evt.Data.EmailVerified,
				"email_occurred_at": evtAt.UTC(),
				"updated_at":        time.Now().UTC(),
			}},
		)
		if err != nil {
			return nil, err
		}
		updated = updRes.ModifiedCount == 1
		if _, err := s.proc.InsertOne(ctx, profile.ProcessedEvent{
			EventID:     evt.Metadata.EventID,
			ProcessedAt: time.Now().UTC(),
		}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return updated, duplicated, err
}

// FindByID recupera el perfil por su id (GET /profiles/{id}).
func (s *MongoStore) FindByID(ctx context.Context, id string) (profile.Profile, error) {
	bin, err := uuidBinary(strings.TrimSpace(id))
	if err != nil {
		return profile.Profile{}, profile.ErrNotFound
	}
	var p profile.Profile
	if err := s.profiles.FindOne(ctx, bson.M{"_id": bin}).Decode(&p); err != nil {
		if err == mongodriver.ErrNoDocuments {
			return profile.Profile{}, profile.ErrNotFound
		}
		return profile.Profile{}, err
	}
	return p, nil
}

// Patch modifica solo phone y address (4.2). No publica eventos.
func (s *MongoStore) Patch(ctx context.Context, id string, in profile.PatchInput, now time.Time) (profile.Profile, error) {
	bin, err := uuidBinary(strings.TrimSpace(id))
	if err != nil {
		return profile.Profile{}, profile.ErrNotFound
	}
	set := bson.M{"updated_at": now.UTC()}
	if in.Phone != nil {
		set["phone"] = *in.Phone
	}
	if in.HasAddress {
		set["address_line"] = *in.AddressLine
		set["city"] = *in.City
	}
	err = s.profiles.FindOneAndUpdate(ctx,
		bson.M{"_id": bin},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Err()
	if err != nil {
		if err == mongodriver.ErrNoDocuments {
			return profile.Profile{}, profile.ErrNotFound
		}
		return profile.Profile{}, err
	}
	return s.FindByID(ctx, id)
}

// ClaimPending reclama atómicamente un lote del outbox (D5-A): solo eventos
// con published_at nulo y sin reclamo vigente.
func (s *MongoStore) ClaimPending(ctx context.Context, batch int, lease time.Duration, now time.Time) ([]profile.OutboxEvent, error) {
	if batch <= 0 {
		batch = 10
	}
	cur, err := s.outbox.Find(ctx,
		bson.M{
			"published_at": nil,
			"$or": []bson.M{
				{"claimed_until": nil},
				{"claimed_until": bson.M{"$lte": now.UTC()}},
			},
		},
		options.Find().SetLimit(int64(batch)).SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var ids []bson.Binary
	for cur.Next(ctx) {
		var e profile.OutboxEvent
		if err := cur.Decode(&e); err != nil {
			continue
		}
		ids = append(ids, e.ID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	claimed := make([]profile.OutboxEvent, 0, len(ids))
	until := now.UTC().Add(lease)
	for _, id := range ids {
		res := s.outbox.FindOneAndUpdate(ctx,
			bson.M{
				"_id":          id,
				"published_at": nil,
				"$or": []bson.M{
					{"claimed_until": nil},
					{"claimed_until": bson.M{"$lte": now.UTC()}},
				},
			},
			bson.M{"$set": bson.M{"claimed_until": until}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		)
		var e profile.OutboxEvent
		if err := res.Decode(&e); err != nil {
			continue // lo reclamó otro Relay
		}
		claimed = append(claimed, e)
	}
	return claimed, nil
}

// MarkPublished marca como publicado solo lo aceptado por EventBridge.
// outboxID es el eventId del evento (campo event_id del outbox).
func (s *MongoStore) MarkPublished(ctx context.Context, outboxID string, now time.Time) error {
	_, err := s.outbox.UpdateOne(ctx, bson.M{"event_id": outboxID},
		bson.M{"$set": bson.M{"published_at": now.UTC()}, "$unset": bson.M{"claimed_until": ""}})
	return err
}

// MarkFailedByBinary reintenta: attempts++ y last_error, libera el reclamo.
func (s *MongoStore) MarkFailedByBinary(ctx context.Context, id bson.Binary, msg string, _ time.Time) error {
	_, err := s.outbox.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc":   bson.M{"attempts": 1},
		"$set":   bson.M{"last_error": msg},
		"$unset": bson.M{"claimed_until": ""},
	})
	return err
}

// MarkFailed implementa Store para compatibilidad (libera reclamo).
// outboxID es el eventId del evento (campo event_id del outbox).
func (s *MongoStore) MarkFailed(ctx context.Context, outboxID string, msg string, now time.Time) error {
	_, err := s.outbox.UpdateOne(ctx, bson.M{"event_id": outboxID}, bson.M{
		"$inc":   bson.M{"attempts": 1},
		"$set":   bson.M{"last_error": msg},
		"$unset": bson.M{"claimed_until": ""},
	})
	return err
}
