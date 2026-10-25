package storage

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// Store implementa profile.Store sobre MongoDB.
type Store struct {
	client    *mongo.Client
	database  *mongo.Database
	profiles  *mongo.Collection
	outbox    *mongo.Collection
	processed *mongo.Collection
}

// NewStore crea el Store sobre la base de datos indicada.
func NewStore(client *mongo.Client, database string) *Store {
	db := client.Database(database)
	return &Store{
		client:    client,
		database:  db,
		profiles:  db.Collection(CollectionProfiles),
		outbox:    db.Collection(CollectionOutbox),
		processed: db.Collection(CollectionProcessed),
	}
}

// CreateProfile registra el evento procesado, crea el perfil y encola el
// ProfileCreated del outbox en una sola transacción (sección 5.3).
func (s *Store) CreateProfile(
	ctx context.Context,
	p profile.Profile,
	out profile.OutboxEvent,
	processed profile.ProcessedEvent,
) error {
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		// El marcador se inserta primero: una clave duplicada aborta la
		// transacción y permite distinguir el evento duplicado.
		if _, err := s.processed.InsertOne(txCtx, newProcessedDocument(processed)); err != nil {
			switch {
			case isDuplicate(err):
				return profile.ErrDuplicateEvent
			default:
				return err
			}
		}
		if _, err := s.profiles.InsertOne(txCtx, newProfileDocument(p)); err != nil {
			switch {
			case isDuplicate(err):
				return profile.ErrProfileAlreadyExists
			default:
				return err
			}
		}
		_, err := s.outbox.InsertOne(txCtx, newOutboxDocument(out))
		return err
	})
}

// ApplyEmailChange registra el evento procesado y actualiza la copia del email
// del perfil en una sola transacción (D8-B). Solo aplica el cambio si el
// evento es más reciente que el último aplicado.
func (s *Store) ApplyEmailChange(
	ctx context.Context,
	change profile.EmailChange,
	processed profile.ProcessedEvent,
) (bool, error) {
	var applied bool
	err := s.withTransaction(ctx, func(txCtx context.Context) error {
		if _, err := s.processed.InsertOne(txCtx, newProcessedDocument(processed)); err != nil {
			switch {
			case isDuplicate(err):
				return profile.ErrDuplicateEvent
			default:
				return err
			}
		}

		// Solo se actualiza si el evento es más reciente que el último cambio
		// aplicado; un evento repetido o antiguo no modifica nada.
		filter := bson.M{
			"_id":              binaryUUID(change.UserID),
			"email_updated_at": bson.M{"$lt": change.OccurredAt},
		}
		update := bson.M{
			"$set": bson.M{
				"email":            change.Email,
				"email_verified":   change.EmailVerified,
				"email_updated_at": change.OccurredAt,
				"updated_at":       time.Now().UTC(),
			},
		}
		result, err := s.profiles.UpdateOne(txCtx, filter, update)
		if err != nil {
			return err
		}
		if result.MatchedCount > 0 {
			applied = true
			return nil
		}

		// Sin coincidencia: o no existe el perfil, o el evento es antiguo.
		err = s.profiles.FindOne(txCtx, bson.M{"_id": binaryUUID(change.UserID)}).Err()
		switch {
		case errors.Is(err, mongo.ErrNoDocuments):
			return profile.ErrProfileNotFound
		case err != nil:
			return err
		default:
			return nil
		}
	})
	return applied, err
}

// GetProfile devuelve el perfil por su id.
func (s *Store) GetProfile(ctx context.Context, id uuid.UUID) (profile.Profile, error) {
	var doc profileDocument
	err := s.profiles.FindOne(ctx, bson.M{"_id": binaryUUID(id)}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return profile.Profile{}, profile.ErrProfileNotFound
	}
	if err != nil {
		return profile.Profile{}, err
	}
	return doc.toProfile()
}

// UpdateProfile aplica un PATCH de phone y/o address y devuelve el perfil
// actualizado (sección 9). No publica eventos.
func (s *Store) UpdateProfile(ctx context.Context, id uuid.UUID, patch profile.Patch) (profile.Profile, error) {
	changes := bson.M{"updated_at": time.Now().UTC()}
	if patch.Phone != nil {
		changes["phone"] = *patch.Phone
	}
	if patch.Address != nil {
		changes["address_line"] = patch.Address.Line
		changes["city"] = patch.Address.City
	}

	var doc profileDocument
	filter := bson.M{"_id": binaryUUID(id)}
	update := bson.M{"$set": changes}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := s.profiles.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return profile.Profile{}, profile.ErrProfileNotFound
	}
	if err != nil {
		return profile.Profile{}, err
	}
	return doc.toProfile()
}

// withTransaction ejecuta fn dentro de una transacción de MongoDB,
// reintentando automáticamente los errores transitorios (sección 8.1).
// Las funciones que usan transacciones multidocumento requieren un réplica
// set: MongoDB Atlas lo es y el contenedor local se configura como uno (D9).
func (s *Store) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	session, err := s.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.WithoutCancel(ctx))

	_, err = session.WithTransaction(ctx, func(txCtx context.Context) (any, error) {
		if err := fn(txCtx); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}
