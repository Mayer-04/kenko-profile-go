package profile

import (
	"context"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
)

// PatchInput son los únicos campos mutables por PATCH (4.2):
// phone y el objeto address completo.
type PatchInput struct {
	Phone       *string
	AddressLine *string
	City        *string
	HasAddress  bool
}

// Store abstrae la persistencia transaccional (worker) y las lecturas.
type Store interface {
	// CreateFromRegistration guarda perfil + processed + outbox en una
	// transacción. Devuelve duplicated=true si el eventId ya se procesó.
	CreateFromRegistration(ctx context.Context, evt events.UserRegistered, now time.Time) (profileID string, duplicated bool, err error)
	// ApplyEmailChanged actualiza email si el evento es más reciente.
	// Clave solo por userId; oldEmail se usa para no aplicar si no coincide.
	// Devuelve notFound=true si la proyección no existe (caso terminal:
	// se registra processed y el worker borra el mensaje, sin reintento).
	ApplyEmailChanged(ctx context.Context, evt events.EmailChanged) (updated bool, duplicated bool, notFound bool, err error)
	FindByID(ctx context.Context, id string) (Profile, error)
	Patch(ctx context.Context, id string, in PatchInput, now time.Time) (Profile, error)

	// Outbox (Relay, D5-A).
	ClaimPending(ctx context.Context, batch int, lease time.Duration, now time.Time) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, outboxID string, now time.Time) error
	MarkFailed(ctx context.Context, outboxID string, msg string, now time.Time) error
}
