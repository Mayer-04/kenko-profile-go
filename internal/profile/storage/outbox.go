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

// ClaimPending reclama hasta limit eventos pendientes marcándolos atómicamente
// como bloqueados por este Relay (opción A de D5: sondeo con reclamo). Los
// eventos quedan libres de nuevo cuando expira el bloqueo, por si el Relay
// muere antes de publicarlos.
func (s *Store) ClaimPending(
	ctx context.Context,
	limit int,
	lockTTL time.Duration,
	owner string,
) ([]profile.OutboxEvent, error) {
	now := time.Now().UTC()
	// Un evento es reclamable si nunca se bloqueó o si su bloqueo ya expiró.
	lockDeadline := now.Add(-lockTTL)

	claimed := []profile.OutboxEvent{}
	for range limit {
		filter := bson.M{
			"published_at": nil,
			"$or": bson.A{
				bson.M{"locked_at": nil},
				bson.M{"locked_at": bson.M{"$lt": lockDeadline}},
			},
		}
		update := bson.M{"$set": bson.M{"locked_at": now, "locked_by": owner}}
		opts := options.FindOneAndUpdate()

		var doc outboxDocument
		err := s.outbox.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
		if errors.Is(err, mongo.ErrNoDocuments) {
			break // no quedan eventos reclamables
		}
		if err != nil {
			return nil, err
		}
		event, err := doc.toOutboxEvent()
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, event)
	}
	return claimed, nil
}

// MarkPublished marca el evento como publicado. Solo lo llama el Relay con
// entradas que EventBridge aceptó (sección 6).
func (s *Store) MarkPublished(ctx context.Context, id uuid.UUID) error {
	filter := bson.M{"_id": binaryUUID(id)}
	update := bson.M{
		"$set":   bson.M{"published_at": time.Now().UTC()},
		"$unset": bson.M{"locked_at": "", "locked_by": ""},
	}
	_, err := s.outbox.UpdateOne(ctx, filter, update)
	return err
}

// MarkFailed registra un intento fallido de publicación: incrementa attempts,
// guarda last_error y renueva el bloqueo para que el evento no se reintente
// hasta que expire el TTL del bloqueo (backoff natural, sección 6).
func (s *Store) MarkFailed(ctx context.Context, id uuid.UUID, cause string) error {
	filter := bson.M{"_id": binaryUUID(id)}
	update := bson.M{
		"$inc": bson.M{"attempts": 1},
		"$set": bson.M{"last_error": cause, "locked_at": time.Now().UTC()},
	}
	_, err := s.outbox.UpdateOne(ctx, filter, update)
	return err
}
