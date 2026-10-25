package profile

import (
	"context"
	"time"

	"uuid"
)

// EmailChange es un UserEmailChanged ya validado y listo para aplicar (D8-B).
type EmailChange struct {
	UserID        uuid.UUID
	Email         string
	EmailVerified bool
	OccurredAt    time.Time
}

// Store es el puerto de persistencia del dominio. Los métodos que afectan a
// más de una colección son atómicos: usan una transacción de MongoDB con
// réplica (sección 8.1).
type Store interface {
	// CreateProfile registra el evento procesado, crea el perfil y encola la
	// publicación de ProfileCreated en una sola transacción (sección 5.3).
	// Devuelve ErrDuplicateEvent si el evento ya se procesó y
	// ErrProfileAlreadyExists si ya existe el perfil para otro evento.
	CreateProfile(ctx context.Context, p Profile, out OutboxEvent, processed ProcessedEvent) error

	// ApplyEmailChange registra el evento procesado y actualiza la copia del
	// email del perfil en una sola transacción (D8-B). Devuelve false cuando
	// el evento es más antiguo que el último cambio aplicado; el registro del
	// evento procesado se guarda igualmente.
	ApplyEmailChange(ctx context.Context, change EmailChange, processed ProcessedEvent) (bool, error)

	// GetProfile devuelve el perfil por su id (ErrProfileNotFound si no existe).
	GetProfile(ctx context.Context, id uuid.UUID) (Profile, error)

	// UpdateProfile aplica un PATCH de phone y/o address y devuelve el perfil
	// actualizado. No publica eventos (sección 12).
	UpdateProfile(ctx context.Context, id uuid.UUID, patch Patch) (Profile, error)
}
