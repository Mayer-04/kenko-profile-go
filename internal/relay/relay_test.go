package relay

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// fakeOutbox implementa relay.OutboxStore registrando lo que recibe.
type fakeOutbox struct {
	toClaim  []profile.OutboxEvent
	claimErr error

	claimedTTLS []time.Duration
	published   []string
	failed      []markFailure
}

type markFailure struct {
	id    string
	cause string
}

func (f *fakeOutbox) ClaimPending(_ context.Context, _ int, lockTTL time.Duration, _ string) ([]profile.OutboxEvent, error) {
	f.claimedTTLS = append(f.claimedTTLS, lockTTL)
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	events := f.toClaim
	f.toClaim = nil
	return events, nil
}

func (f *fakeOutbox) MarkPublished(_ context.Context, id uuid.UUID) error {
	f.published = append(f.published, id.String())
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, id uuid.UUID, cause string) error {
	f.failed = append(f.failed, markFailure{id: id.String(), cause: cause})
	return nil
}

// fakePublisher implementa relay.Publisher.
type fakePublisher struct {
	publishErr error
	results    []Result
	entries    [][]Entry
}

func (f *fakePublisher) Publish(_ context.Context, entries []Entry) ([]Result, error) {
	copied := append([]Entry{}, entries...)
	f.entries = append(f.entries, copied)
	if f.publishErr != nil {
		return nil, f.publishErr
	}
	return f.results, nil
}

func outboxEvent(id string) profile.OutboxEvent {
	return profile.OutboxEvent{
		ID:        uuid.MustParse(id),
		EventType: profile.TypeProfileCreated,
		Payload:   `{"metadata":{"eventId":"x"},"data":{}}`,
	}
}

func testRelay(t *testing.T) (*Relay, *fakeOutbox, *fakePublisher) {
	t.Helper()
	store := &fakeOutbox{}
	publisher := &fakePublisher{}
	logger := slog.New(slog.DiscardHandler)
	r := NewRelay(store, publisher, logger, Config{
		PollInterval: time.Millisecond,
		BatchSize:    10,
		LockTTL:      time.Minute,
		Owner:        "relay-de-prueba",
	})
	return r, store, publisher
}

func TestPublishPendingSinEventosNoPublica(t *testing.T) {
	r, store, publisher := testRelay(t)

	r.publishPending(t.Context())

	if len(publisher.entries) != 0 {
		t.Errorf("sin eventos no debe llamarse a PutEvents")
	}
	if len(store.claimedTTLS) != 1 {
		t.Errorf("debe reclamar el outbox una vez por ciclo")
	}
}

func TestPublishPendingPublicaElLote(t *testing.T) {
	r, store, publisher := testRelay(t)

	event := outboxEvent("11111111-1117-7111-8111-111111111111")
	store.toClaim = []profile.OutboxEvent{event}
	publisher.results = []Result{{Success: true}}

	r.publishPending(t.Context())

	if len(publisher.entries) != 1 {
		t.Fatalf("PutEvents llamados = %d", len(publisher.entries))
	}
	entry := publisher.entries[0][0]
	if entry.Source != profile.SourceProfile {
		t.Errorf("source = %q, se quiere %q", entry.Source, profile.SourceProfile)
	}
	if entry.DetailType != profile.TypeProfileCreated {
		t.Errorf("detail-type = %q", entry.DetailType)
	}
	if entry.Detail != event.Payload {
		t.Errorf("detail = %q, se quiere el payload del outbox", entry.Detail)
	}
	if len(store.published) != 1 || store.published[0] != event.ID.String() {
		t.Errorf("publicados = %v, se quiere el evento reclamado", store.published)
	}
	if len(store.failed) != 0 {
		t.Errorf("fallidos = %v, se quiere ninguno", store.failed)
	}
}

func TestPublishPendingFallosParciales(t *testing.T) {
	r, store, publisher := testRelay(t)

	store.toClaim = []profile.OutboxEvent{
		outboxEvent("11111111-1117-7111-8111-111111111111"),
		outboxEvent("22222222-2227-7222-8222-222222222222"),
	}
	publisher.results = []Result{
		{Success: true},
		{Success: false, ErrorCode: "InternalFailure", ErrorMessage: "entrada rechazada"},
	}

	r.publishPending(t.Context())

	if len(store.published) != 1 {
		t.Errorf("publicados = %v, se quiere solo el aceptado", store.published)
	}
	if len(store.failed) != 1 {
		t.Fatalf("fallidos = %d, se quiere 1", len(store.failed))
	}
	failure := store.failed[0]
	if failure.id != "22222222-2227-7222-8222-222222222222" {
		t.Errorf("el evento fallido registrado es %q", failure.id)
	}
	if failure.cause != "InternalFailure: entrada rechazada" {
		t.Errorf("causa = %q", failure.cause)
	}
}

func TestPublishPendingErrorDePutEventsMarcaTodoFallido(t *testing.T) {
	r, store, publisher := testRelay(t)

	store.toClaim = []profile.OutboxEvent{
		outboxEvent("11111111-1117-7111-8111-111111111111"),
		outboxEvent("22222222-2227-7222-8222-222222222222"),
	}
	publisher.publishErr = errors.New("credenciales expiradas")

	r.publishPending(t.Context())

	if len(store.published) != 0 {
		t.Errorf("nada debe marcarse como publicado, publicado = %v", store.published)
	}
	if len(store.failed) != 2 {
		t.Fatalf("fallidos = %d, se quiere los 2 eventos", len(store.failed))
	}
	for _, failure := range store.failed {
		if failure.cause != "credenciales expiradas" {
			t.Errorf("causa = %q", failure.cause)
		}
	}
}

func TestPublishPendingFalloDeReclamoNoPublica(t *testing.T) {
	r, store, publisher := testRelay(t)

	store.claimErr = errors.New("réplica sin primaria")

	r.publishPending(t.Context())

	if len(publisher.entries) != 0 {
		t.Errorf("con fallo de reclamo no se publica nada")
	}
}

func TestRunTerminaConContextoCancelado(t *testing.T) {
	r, _, _ := testRelay(t)

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run debe terminar sin error al cancelar: %v", err)
	}
}
