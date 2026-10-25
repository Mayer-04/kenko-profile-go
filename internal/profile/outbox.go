package profile

import (
	"time"

	"uuid"
)

// OutboxEvent es un evento pendiente de publicar en EventBridge, guardado en
// la misma transacción que el perfil (patrón Outbox, sección 6).
type OutboxEvent struct {
	ID            uuid.UUID
	EventType     string    // por ejemplo ProfileCreated
	AggregateID   uuid.UUID // id del perfil
	CorrelationID string
	Payload       string // JSON completo del detail (sección 7.2)
	CreatedAt     time.Time
}

// ProcessedEvent registra un evento de entrada ya procesado para garantizar
// idempotencia (opción A de D4).
type ProcessedEvent struct {
	ID          uuid.UUID
	EventID     uuid.UUID
	ProcessedAt time.Time
}
