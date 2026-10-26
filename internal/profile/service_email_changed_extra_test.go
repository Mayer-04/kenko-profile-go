package profile

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
)

// captureStore verifica que el Service reenvía el evento intacto al Store.
type captureStore struct {
	fakeStore
	got events.EmailChanged
}

func (c *captureStore) ApplyEmailChanged(ctx context.Context, evt events.EmailChanged) (bool, bool, bool, error) {
	c.got = evt
	return c.fakeStore.ApplyEmailChanged(ctx, evt)
}

func (c *captureStore) CreateFromRegistration(ctx context.Context, evt events.UserRegistered, now time.Time) (string, bool, error) {
	return "", false, nil
}

func TestApplyEmailChangedAllBranchesNoPII(t *testing.T) {
	branches := []struct {
		name  string
		store fakeStore
	}{
		{"updated", fakeStore{updated: true}},
		{"duplicate", fakeStore{dup: true}},
		{"notFound", fakeStore{notFound: true}},
		{"stale", fakeStore{}},
	}
	for _, tc := range branches {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			svc := newTestService(tc.store, &buf)
			evt := testEmailChangedEvent()
			if err := svc.ApplyEmailChanged(context.Background(), evt); err != nil {
				t.Fatalf("rama %s terminal: %v", tc.name, err)
			}
			out := buf.String()
			for _, pii := range []string{evt.Data.NewEmail, evt.Data.OldEmail, evt.Data.FirstName} {
				if pii != "" && strings.Contains(out, pii) {
					t.Fatalf("rama %s loguea PII %q", tc.name, pii)
				}
			}
			if !strings.Contains(out, evt.Metadata.EventID) {
				t.Fatalf("rama %s debería loguear eventId", tc.name)
			}
		})
	}
}

func TestApplyEmailChangedLogNames(t *testing.T) {
	cases := []struct {
		name     string
		store    fakeStore
		wantLog  string
		wantProf bool
	}{
		{"duplicate", fakeStore{dup: true}, "profile.duplicate_skipped", false},
		{"notFound", fakeStore{notFound: true}, "profile.email_user_not_found", true},
		{"updated", fakeStore{updated: true}, "profile.email_updated", true},
		{"stale", fakeStore{}, "profile.email_stale_ignored", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			svc := newTestService(tc.store, &buf)
			evt := testEmailChangedEvent()
			if err := svc.ApplyEmailChanged(context.Background(), evt); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(buf.String(), tc.wantLog) {
				t.Fatalf("se esperaba log %q, got %q", tc.wantLog, buf.String())
			}
			if tc.wantProf && !strings.Contains(buf.String(), evt.Data.UserID) {
				t.Fatalf("se esperaba profileId en log")
			}
		})
	}
}

func TestApplyEmailChangedForwardsEvent(t *testing.T) {
	var buf bytes.Buffer
	cs := &captureStore{fakeStore: fakeStore{updated: true}}
	svc := NewService(cs, slog.New(slog.NewTextHandler(&buf, nil)))
	want := testEmailChangedEvent()
	if err := svc.ApplyEmailChanged(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if cs.got.Metadata.EventID != want.Metadata.EventID ||
		cs.got.Data.UserID != want.Data.UserID ||
		cs.got.Data.NewEmail != want.Data.NewEmail ||
		cs.got.Data.OldEmail != want.Data.OldEmail {
		t.Fatalf("evento no reenviado intacto: %+v", cs.got)
	}
}

func TestApplyEmailChangedDuplicatePriority(t *testing.T) {
	var buf bytes.Buffer
	svc := newTestService(fakeStore{dup: true, notFound: true}, &buf)
	if err := svc.ApplyEmailChanged(context.Background(), testEmailChangedEvent()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "profile.duplicate_skipped") {
		t.Fatalf("duplicado debe ganar sobre notFound, got %q", buf.String())
	}
}
