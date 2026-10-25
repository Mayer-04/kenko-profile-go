// Package relay publica en EventBridge los eventos pendientes del outbox
// (sección 6).
package relay

import (
	"context"
	"log/slog"
	"time"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// Result es el resultado de publicar una entrada; EventBridge los devuelve en
// el mismo orden en que se enviaron.
type Result struct {
	Success      bool
	ErrorCode    string
	ErrorMessage string
}

// Publisher envía eventos al bus de EventBridge (implementado por
// platform/eventbridge con los parámetros de la sección 7.2).
type Publisher interface {
	Publish(ctx context.Context, entries []Entry) ([]Result, error)
}

// Entry es un evento listo para PutEvents.
type Entry struct {
	Source     string
	DetailType string
	Detail     string
}

// OutboxStore es el puerto de persistencia que el Relay necesita (opción A de
// D5: sondeo con reclamo atómico).
type OutboxStore interface {
	// ClaimPending reclama eventos pendientes marcándolos como bloqueados.
	ClaimPending(ctx context.Context, limit int, lockTTL time.Duration, owner string) ([]profile.OutboxEvent, error)
	// MarkPublished marca un evento como publicado (solo si EventBridge lo aceptó).
	MarkPublished(ctx context.Context, id uuid.UUID) error
	// MarkFailed registra un intento fallido y agenda el reintento.
	MarkFailed(ctx context.Context, id uuid.UUID, cause string) error
}

// Config tiene los parámetros del Relay.
type Config struct {
	// PollInterval es cada cuánto se consulta el outbox.
	PollInterval time.Duration
	// BatchSize es el máximo de eventos por ciclo (y por PutEvents).
	BatchSize int
	// LockTTL es cuánto dura el bloqueo de un evento reclamado.
	LockTTL time.Duration
	// Owner identifica esta instancia de Relay en los bloqueos.
	Owner string
}

// Relay lee el outbox y publica los eventos pendientes en EventBridge.
type Relay struct {
	store     OutboxStore
	publisher Publisher
	logger    *slog.Logger
	cfg       Config
}

// NewRelay crea el Relay con sus dependencias inyectadas.
func NewRelay(store OutboxStore, publisher Publisher, logger *slog.Logger, cfg Config) *Relay {
	return &Relay{store: store, publisher: publisher, logger: logger, cfg: cfg}
}

// Run publica los eventos pendientes hasta que el contexto se cancela.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		r.publishPending(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// publishPending reclama un lote del outbox y lo publica en EventBridge.
// Marca como publicado solo lo que EventBridge aceptó; el resto se reintenta
// (sección 6).
func (r *Relay) publishPending(ctx context.Context) {
	events, err := r.store.ClaimPending(ctx, r.cfg.BatchSize, r.cfg.LockTTL, r.cfg.Owner)
	if err != nil {
		r.logger.Error("fallo al reclamar eventos del outbox", "error", err)
		return
	}
	if len(events) == 0 {
		return // ciclo vacío: no se registra (sección 11.3)
	}

	entries := make([]Entry, 0, len(events))
	for _, event := range events {
		entries = append(entries, Entry{
			Source:     profile.SourceProfile,
			DetailType: event.EventType,
			Detail:     event.Payload,
		})
	}

	results, err := r.publisher.Publish(ctx, entries)
	if err != nil {
		r.logger.Error("fallo de PutEvents", "eventos", len(events), "error", err)
		r.markAllFailed(ctx, events, err.Error())
		return
	}

	published, failed := r.applyResults(ctx, events, results)
	switch {
	case failed == 0:
		r.logger.Info("lote publicado", "eventos", published)
	case published == 0:
		r.logger.Warn("lote con fallos totales en PutEvents", "eventos", failed)
	default:
		r.logger.Warn("fallos parciales en PutEvents", "publicados", published, "fallidos", failed)
	}
}

// applyResults marca cada evento según el resultado de su entrada.
func (r *Relay) applyResults(ctx context.Context, events []profile.OutboxEvent, results []Result) (published, failed int) {
	for i, result := range results {
		event := events[i]
		if result.Success {
			if err := r.store.MarkPublished(ctx, event.ID); err != nil {
				r.logger.Error("fallo al marcar como publicado", "eventoId", event.ID, "error", err)
			}
			published++
			continue
		}
		cause := result.ErrorCode
		if result.ErrorMessage != "" {
			cause += ": " + result.ErrorMessage
		}
		if err := r.store.MarkFailed(ctx, event.ID, cause); err != nil {
			r.logger.Error("fallo al registrar el intento fallido", "eventoId", event.ID, "error", err)
		}
		failed++
	}
	return published, failed
}

func (r *Relay) markAllFailed(ctx context.Context, events []profile.OutboxEvent, cause string) {
	for _, event := range events {
		if err := r.store.MarkFailed(ctx, event.ID, cause); err != nil {
			r.logger.Error("fallo al registrar el intento fallido", "eventoId", event.ID, "error", err)
		}
	}
}
