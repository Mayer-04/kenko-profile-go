package profile

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Profile es el documento de la colección profiles.
// Correspondencia evento→documento de 7.1. _id = userId de Authentication
// como UUID BSON estándar (8.1). Instantes en UTC (11.1).
type Profile struct {
	ID              bson.Binary `bson:"_id"`
	DocumentType    string      `bson:"document_type" json:"document_type"`
	DocumentNumber  string      `bson:"document_number" json:"document_number"`
	FirstName       string      `bson:"first_name" json:"first_name"`
	LastName        string      `bson:"last_name" json:"last_name"`
	BirthDate       time.Time   `bson:"birth_date" json:"birth_date"`
	Gender          string      `bson:"gender" json:"gender"`
	Email           string      `bson:"email" json:"email"`
	EmailVerified   bool        `bson:"email_verified" json:"email_verified"`
	EmailOccurredAt time.Time   `bson:"email_occurred_at" json:"-"`
	Phone           string      `bson:"phone" json:"phone"`
	AddressLine     string      `bson:"address_line" json:"address_line"`
	City            string      `bson:"city" json:"city"`
	CreatedAt       time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time   `bson:"updated_at" json:"updated_at"`
}

// OutboxEvent es el documento de outbox_events (8.2 + D5-A/D6-A).
type OutboxEvent struct {
	ID           bson.Binary  `bson:"_id"`
	EventType    string       `bson:"event_type"`
	AggregateID  bson.Binary  `bson:"aggregate_id"`
	CorrelationID string      `bson:"correlation_id"`
	EventID      string       `bson:"event_id"`
	Payload      bson.M       `bson:"payload"`
	CreatedAt    time.Time    `bson:"created_at"`
	PublishedAt  *time.Time   `bson:"published_at"`
	ClaimedUntil *time.Time   `bson:"claimed_until"`
	Attempts     int          `bson:"attempts"`
	LastError    string       `bson:"last_error,omitempty"`
}

// ProcessedEvent registra eventos de entrada ya aplicados (D4-A).
type ProcessedEvent struct {
	EventID     string    `bson:"event_id"`
	ProcessedAt time.Time `bson:"processed_at"`
}
