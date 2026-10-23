// Package relay publica los eventos pendientes del outbox en
// kenko-dev-bus con PutEvents (sección 6): solo marca lo aceptado,
// reintenta lo fallido con attempts/last_error.
package relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

// Relay sondea el outbox con reclamo atómico (D5-A).
type Relay struct {
	store      profile.Store
	eb         *eventbridge.Client
	bus        string
	log        *slog.Logger
	interval   time.Duration
	batch      int
	lease      time.Duration
}

// New crea el relay con DI explícita.
func New(store profile.Store, eb *eventbridge.Client, bus string, log *slog.Logger, interval time.Duration, batch int, lease time.Duration) *Relay {
	return &Relay{store: store, eb: eb, bus: bus, log: log, interval: interval, batch: batch, lease: lease}
}

// Run publica por lotes hasta que ctx se cancela. No registra ciclos vacíos.
func (r *Relay) Run(ctx context.Context) error {
	r.log.Info("relay.started", "bus", r.bus)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			r.publishOnce(ctx)
		}
	}
}

func (r *Relay) publishOnce(ctx context.Context) {
	now := time.Now().UTC()
	claimed, err := r.store.ClaimPending(ctx, r.batch, r.lease, now)
	if err != nil {
		r.log.Error("relay.claim_failed")
		return
	}
	if len(claimed) == 0 {
		return
	}
	entries := make([]types.PutEventsRequestEntry, 0, len(claimed))
	sent := make([]profile.OutboxEvent, 0, len(claimed))
	for _, e := range claimed {
		detail, err := json.Marshal(e.Payload)
		if err != nil {
			_ = r.store.MarkFailed(ctx, e.EventID, "marshal payload", now)
			continue
		}
		entries = append(entries, types.PutEventsRequestEntry{
			Source:       aws.String("kenko.profile"),
			DetailType:   aws.String(e.EventType),
			EventBusName: aws.String(r.bus),
			Detail:       aws.String(string(detail)),
		})
		sent = append(sent, e)
	}
	if len(entries) == 0 {
		return
	}
	out, err := r.eb.PutEvents(ctx, &eventbridge.PutEventsInput{Entries: entries})
	if err != nil {
		r.log.Error("relay.put_failed", "count", len(entries))
		for _, e := range sent {
			_ = r.store.MarkFailed(ctx, e.EventID, "putevents call failed", now)
		}
		return
	}
	// Marcar solo lo aceptado (6): revisar FailedEntryCount y entradas fallidas.
	var ok, failed int
	for i, res := range out.Entries {
		if i >= len(sent) {
			break
		}
		if res.EventId != nil && aws.ToString(res.ErrorCode) == "" {
			if err := r.store.MarkPublished(ctx, sent[i].EventID, now); err != nil {
				r.log.Error("relay.mark_failed", "eventId", sent[i].EventID)
				continue
			}
			ok++
		} else {
			failed++
			msg := aws.ToString(res.ErrorCode) + " " + aws.ToString(res.ErrorMessage)
			_ = r.store.MarkFailed(ctx, sent[i].EventID, msg, now)
		}
	}
	if ok > 0 {
		r.log.Info("relay.batch_published", "count", ok)
	}
	if failed > 0 {
		r.log.Warn("relay.partial_failure", "failed", failed, "published", ok)
	}
}
