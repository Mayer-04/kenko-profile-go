// Package storage implementa el puerto profile.Store con MongoDB y la
// publicación del outbox (sección 8).
package storage

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// Colecciones del servicio (sección 8.2).
const (
	CollectionProfiles  = "profiles"
	CollectionOutbox    = "outbox_events"
	CollectionProcessed = "processed_events"
)

// uuidBinarySubtype es el subtipo BSON estándar para UUID (0x04).
const uuidBinarySubtype byte = 0x04

// profileDocument refleja el documento de la colección profiles, según la
// correspondencia de la sección 7.1.
type profileDocument struct {
	ID             bson.Binary `bson:"_id"`
	DocumentType   string      `bson:"document_type"`
	DocumentNumber string      `bson:"document_number"`
	FirstName      string      `bson:"first_name"`
	LastName       string      `bson:"last_name"`
	BirthDate      time.Time   `bson:"birth_date"`
	Gender         string      `bson:"gender"`
	Email          string      `bson:"email"`
	EmailVerified  bool        `bson:"email_verified"`
	Phone          string      `bson:"phone"`
	AddressLine    string      `bson:"address_line"`
	City           string      `bson:"city"`
	EmailUpdatedAt time.Time   `bson:"email_updated_at"`
	CreatedAt      time.Time   `bson:"created_at"`
	UpdatedAt      time.Time   `bson:"updated_at"`
}

func newProfileDocument(p profile.Profile) profileDocument {
	return profileDocument{
		ID:             binaryUUID(p.ID),
		DocumentType:   p.DocumentType,
		DocumentNumber: p.DocumentNumber,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		BirthDate:      p.BirthDate,
		Gender:         p.Gender,
		Email:          p.Email,
		EmailVerified:  p.EmailVerified,
		Phone:          p.Phone,
		AddressLine:    p.AddressLine,
		City:           p.City,
		EmailUpdatedAt: p.EmailUpdatedAt,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func (d profileDocument) toProfile() (profile.Profile, error) {
	id, err := uuidFromBinary(d.ID)
	if err != nil {
		return profile.Profile{}, fmt.Errorf("leyendo el _id del perfil: %w", err)
	}
	return profile.Profile{
		ID:             id,
		DocumentType:   d.DocumentType,
		DocumentNumber: d.DocumentNumber,
		FirstName:      d.FirstName,
		LastName:       d.LastName,
		BirthDate:      d.BirthDate,
		Gender:         d.Gender,
		Email:          d.Email,
		EmailVerified:  d.EmailVerified,
		Phone:          d.Phone,
		AddressLine:    d.AddressLine,
		City:           d.City,
		EmailUpdatedAt: d.EmailUpdatedAt,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}, nil
}

// outboxDocument refleja el documento de outbox_events (sección 8.2). Los
// campos opcionales se omiten cuando son nulos: las consultas por null de
// MongoDB también casan con campos ausentes.
type outboxDocument struct {
	ID            bson.Binary `bson:"_id"`
	EventType     string      `bson:"event_type"`
	AggregateID   bson.Binary `bson:"aggregate_id"`
	CorrelationID string      `bson:"correlation_id"`
	Payload       string      `bson:"payload"`
	CreatedAt     time.Time   `bson:"created_at"`
	PublishedAt   *time.Time  `bson:"published_at,omitempty"`
	Attempts      int32       `bson:"attempts"`
	LastError     *string     `bson:"last_error,omitempty"`
	LockedAt      *time.Time  `bson:"locked_at,omitempty"`
	LockedBy      *string     `bson:"locked_by,omitempty"`
}

func newOutboxDocument(e profile.OutboxEvent) outboxDocument {
	return outboxDocument{
		ID:            binaryUUID(e.ID),
		EventType:     e.EventType,
		AggregateID:   binaryUUID(e.AggregateID),
		CorrelationID: e.CorrelationID,
		Payload:       e.Payload,
		CreatedAt:     e.CreatedAt,
		Attempts:      0,
	}
}

func (d outboxDocument) toOutboxEvent() (profile.OutboxEvent, error) {
	id, err := uuidFromBinary(d.ID)
	if err != nil {
		return profile.OutboxEvent{}, fmt.Errorf("leyendo el _id del outbox: %w", err)
	}
	aggregateID, err := uuidFromBinary(d.AggregateID)
	if err != nil {
		return profile.OutboxEvent{}, fmt.Errorf("leyendo el aggregate_id del outbox: %w", err)
	}
	return profile.OutboxEvent{
		ID:            id,
		EventType:     d.EventType,
		AggregateID:   aggregateID,
		CorrelationID: d.CorrelationID,
		Payload:       d.Payload,
		CreatedAt:     d.CreatedAt,
	}, nil
}

// processedDocument refleja el documento de processed_events (opción A de D4).
type processedDocument struct {
	ID          bson.Binary `bson:"_id"`
	EventID     bson.Binary `bson:"event_id"`
	ProcessedAt time.Time   `bson:"processed_at"`
}

func newProcessedDocument(e profile.ProcessedEvent) processedDocument {
	return processedDocument{
		ID:          binaryUUID(e.ID),
		EventID:     binaryUUID(e.EventID),
		ProcessedAt: e.ProcessedAt,
	}
}

// binaryUUID convierte un UUID a su representación BSON estándar.
func binaryUUID(id uuid.UUID) bson.Binary {
	return bson.Binary{Subtype: uuidBinarySubtype, Data: id[:]}
}

// uuidFromBinary convierte un binario BSON a UUID.
func uuidFromBinary(b bson.Binary) (uuid.UUID, error) {
	var id uuid.UUID
	if b.Subtype != uuidBinarySubtype || len(b.Data) != len(id) {
		return uuid.Nil(), fmt.Errorf("valor binario no es un UUID estándar (subtipo %d, %d bytes)", b.Subtype, len(b.Data))
	}
	copy(id[:], b.Data)
	return id, nil
}

// isDuplicate indica si el error es una clave duplicada (E11000).
func isDuplicate(err error) bool {
	return err != nil && mongo.IsDuplicateKeyError(err)
}
