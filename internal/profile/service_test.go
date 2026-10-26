package profile

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
	"github.com/gofrs/uuid/v5"
)

func TestValidatePatchOK(t *testing.T) {
	raw := map[string]any{
		"phone":   "+573009998877",
		"address": map[string]any{"line": "Calle 10 # 5-20", "city": "Medellin"},
	}
	in, err := ValidatePatch(raw)
	if err != nil {
		t.Fatalf("patch válido rechazado: %v", err)
	}
	if in.Phone == nil || !in.HasAddress {
		t.Fatal("campos no mapeados")
	}
}

func TestValidatePatchRejectsEmailAndPassword(t *testing.T) {
	for _, raw := range []map[string]any{
		{"email": "x@y.com"},
		{"password": "secret"},
		{"phone": "+57300", "email": "x@y.com"},
		{"first_name": "Otro"},
		{"address": map[string]any{"line": "solo linea"}},
		{"address": map[string]any{"line": "L", "city": "C", "extra": "x"}},
		{},
	} {
		if _, err := ValidatePatch(raw); !errors.Is(err, ErrInvalidPatch) {
			t.Fatalf("debería rechazar %v", raw)
		}
	}
}

// fakeStore implementa Store para probar ApplyEmailChanged sin Mongo.
type fakeStore struct {
	updated  bool
	dup      bool
	notFound bool
	err      error
}

func (f fakeStore) CreateFromRegistration(ctx context.Context, evt events.UserRegistered, now time.Time) (string, bool, error) {
	return "", false, nil
}

func (f fakeStore) ApplyEmailChanged(ctx context.Context, evt events.EmailChanged) (bool, bool, bool, error) {
	return f.updated, f.dup, f.notFound, f.err
}

func (f fakeStore) FindByID(ctx context.Context, id string) (Profile, error) {
	return Profile{}, ErrNotFound
}

func (f fakeStore) Patch(ctx context.Context, id string, in PatchInput, now time.Time) (Profile, error) {
	return Profile{}, ErrNotFound
}

func (f fakeStore) ClaimPending(ctx context.Context, batch int, lease time.Duration, now time.Time) ([]OutboxEvent, error) {
	return nil, nil
}

func (f fakeStore) MarkPublished(ctx context.Context, outboxID string, now time.Time) error {
	return nil
}

func (f fakeStore) MarkFailed(ctx context.Context, outboxID string, msg string, now time.Time) error {
	return nil
}

func testEmailChangedEvent() events.EmailChanged {
	uid, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	cid, _ := uuid.NewV7()
	return events.EmailChanged{
		Metadata: events.Metadata{
			EventID:       eid.String(),
			EventVersion:  "1",
			OccurredAt:    "2026-10-26T10:00:00Z",
			CorrelationID: cid.String(),
		},
		Data: events.EmailChangedData{
			UserID:    uid.String(),
			OldEmail:  "prev@example.com",
			NewEmail:  "next@example.com",
			FirstName: "Maria Fernanda",
		},
	}
}

func newTestService(f fakeStore, buf *bytes.Buffer) *Service {
	h := slog.NewTextHandler(buf, nil)
	return NewService(f, slog.New(h))
}

func TestApplyEmailChangedBranchesAreTerminal(t *testing.T) {
	cases := []struct {
		name  string
		store fakeStore
	}{
		{"updated", fakeStore{updated: true}},
		{"duplicate", fakeStore{dup: true}},
		{"notFound", fakeStore{notFound: true}},
		{"stale", fakeStore{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			svc := newTestService(tc.store, &buf)
			if err := svc.ApplyEmailChanged(context.Background(), testEmailChangedEvent()); err != nil {
				t.Fatalf("%s debería ser terminal sin error: %v", tc.name, err)
			}
		})
	}
}

func TestApplyEmailChangedInfraErrorPropagates(t *testing.T) {
	var buf bytes.Buffer
	svc := newTestService(fakeStore{err: errors.New("db caído")}, &buf)
	if err := svc.ApplyEmailChanged(context.Background(), testEmailChangedEvent()); err == nil {
		t.Fatal("error de infra debería propagarse para reintento/DLQ")
	}
}

func TestApplyEmailChangedNeverLogsPII(t *testing.T) {
	var buf bytes.Buffer
	svc := newTestService(fakeStore{updated: true}, &buf)
	evt := testEmailChangedEvent()
	if err := svc.ApplyEmailChanged(context.Background(), evt); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, pii := range []string{evt.Data.NewEmail, evt.Data.OldEmail, evt.Data.FirstName} {
		if pii != "" && strings.Contains(out, pii) {
			t.Fatalf("log contiene PII %q: %q", pii, out)
		}
	}
}
