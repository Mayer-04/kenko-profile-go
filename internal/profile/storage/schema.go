package storage

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EnsureSchema crea las colecciones con sus validadores e índices si aún no
// existen (sección 8.2). Es idempotente y se ejecuta al arrancar cualquiera de
// los procesos del servicio. Los validadores solo se aplican al crear la
// colección: modificarlos después exigiría permisos de administración.
func (s *Store) EnsureSchema(ctx context.Context, outboxTTLDays int) error {
	existing, err := s.database.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return fmt.Errorf("listando las colecciones: %w", err)
	}
	created := make(map[string]bool, len(existing))
	for _, name := range existing {
		created[name] = true
	}

	if !created[CollectionProfiles] {
		opts := options.CreateCollection().SetValidator(profilesValidator())
		if err := s.database.CreateCollection(ctx, CollectionProfiles, opts); err != nil {
			return fmt.Errorf("creando la colección %s: %w", CollectionProfiles, err)
		}
	}

	if !created[CollectionOutbox] {
		opts := options.CreateCollection().SetValidator(outboxValidator())
		if err := s.database.CreateCollection(ctx, CollectionOutbox, opts); err != nil {
			return fmt.Errorf("creando la colección %s: %w", CollectionOutbox, err)
		}
	}
	// TTL que borra los eventos publicados tras outboxTTLDays días (opción A
	// de D6); los pendientes tienen published_at nulo y nunca expiran.
	// El segundo índice respalda la consulta de reclamo del Relay (opción A de D5).
	outboxIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "published_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(outboxTTLDays) * secondsPerDay),
		},
		{
			Keys: bson.D{
				{Key: "published_at", Value: 1},
				{Key: "locked_at", Value: 1},
			},
		},
	}
	if _, err := s.outbox.Indexes().CreateMany(ctx, outboxIndexes); err != nil {
		return fmt.Errorf("creando los índices de %s: %w", CollectionOutbox, err)
	}

	if !created[CollectionProcessed] {
		opts := options.CreateCollection().SetValidator(processedValidator())
		if err := s.database.CreateCollection(ctx, CollectionProcessed, opts); err != nil {
			return fmt.Errorf("creando la colección %s: %w", CollectionProcessed, err)
		}
	}
	// Índice único por event_id: la idempotencia se apoya en él (opción A de D4).
	processedIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	if _, err := s.processed.Indexes().CreateMany(ctx, processedIndexes); err != nil {
		return fmt.Errorf("creando los índices de %s: %w", CollectionProcessed, err)
	}
	return nil
}

const secondsPerDay = 24 * 60 * 60

// profilesValidator exige los campos de la sección 7.1 con sus tipos.
func profilesValidator() any {
	required := bson.A{
		"_id", "document_type", "document_number", "first_name", "last_name",
		"birth_date", "gender", "email", "email_verified", "phone",
		"address_line", "city", "email_updated_at", "created_at", "updated_at",
	}
	return bson.D{{Key: "$jsonSchema", Value: bson.M{
		"bsonType": "object",
		"required": required,
		"properties": bson.M{
			"_id":              bson.M{"bsonType": "binData"},
			"document_type":    bson.M{"bsonType": "string", "minLength": 1},
			"document_number":  bson.M{"bsonType": "string", "minLength": 1},
			"first_name":       bson.M{"bsonType": "string", "minLength": 1},
			"last_name":        bson.M{"bsonType": "string", "minLength": 1},
			"birth_date":       bson.M{"bsonType": "date"},
			"gender":           bson.M{"enum": bson.A{"male", "female", "other"}},
			"email":            bson.M{"bsonType": "string", "minLength": 3},
			"email_verified":   bson.M{"bsonType": "bool"},
			"phone":            bson.M{"bsonType": "string", "minLength": 1},
			"address_line":     bson.M{"bsonType": "string", "minLength": 1},
			"city":             bson.M{"bsonType": "string", "minLength": 1},
			"email_updated_at": bson.M{"bsonType": "date"},
			"created_at":       bson.M{"bsonType": "date"},
			"updated_at":       bson.M{"bsonType": "date"},
		},
	}}}
}

// outboxValidator exige los campos de la sección 8.2.
func outboxValidator() any {
	required := bson.A{
		"_id", "event_type", "aggregate_id", "correlation_id",
		"payload", "created_at", "attempts",
	}
	return bson.D{{Key: "$jsonSchema", Value: bson.M{
		"bsonType": "object",
		"required": required,
		"properties": bson.M{
			"_id":            bson.M{"bsonType": "binData"},
			"event_type":     bson.M{"bsonType": "string", "minLength": 1},
			"aggregate_id":   bson.M{"bsonType": "binData"},
			"correlation_id": bson.M{"bsonType": "string", "minLength": 1},
			"payload":        bson.M{"bsonType": "string", "minLength": 1},
			"created_at":     bson.M{"bsonType": "date"},
			"published_at":   bson.M{"bsonType": "date"},
			"attempts":       bson.M{"bsonType": "int"},
			"last_error":     bson.M{"bsonType": "string"},
			"locked_at":      bson.M{"bsonType": "date"},
			"locked_by":      bson.M{"bsonType": "string"},
		},
	}}}
}

// processedValidator exige los campos de la sección 8.2.
func processedValidator() any {
	return bson.D{{Key: "$jsonSchema", Value: bson.M{
		"bsonType": "object",
		"required": bson.A{"_id", "event_id", "processed_at"},
		"properties": bson.M{
			"_id":          bson.M{"bsonType": "binData"},
			"event_id":     bson.M{"bsonType": "binData"},
			"processed_at": bson.M{"bsonType": "date"},
		},
	}}}
}
