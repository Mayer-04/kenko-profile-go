package authz

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

type nFakeReader struct {
	item AuthorizationWithAppointment
	err  error
}

func (f *nFakeReader) GetByID(_ context.Context, _, _ string) (AuthorizationWithAppointment, error) {
	if f.err != nil {
		return AuthorizationWithAppointment{}, f.err
	}
	return f.item, nil
}

func (f *nFakeReader) ListByUser(_ context.Context, _ string) ([]AuthorizationWithAppointment, error) {
	return []AuthorizationWithAppointment{f.item}, nil
}

type nFakeProfiles struct {
	snap documents.ProfileSnapshot
	err  error
}

func (f *nFakeProfiles) GetProfile(_ context.Context, _ string) (documents.ProfileSnapshot, error) {
	if f.err != nil {
		return documents.ProfileSnapshot{}, f.err
	}
	return f.snap, nil
}

type nFakePDF struct{ err error }

func (f *nFakePDF) GenerateAuthorization(_ documents.ProfileSnapshot, _ AuthorizationWithAppointment) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []byte("%PDF-fake"), nil
}

type nFakeStorage struct {
	keys []string
	err  error
}

func (f *nFakeStorage) Upload(_ context.Context, key string, _ []byte, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	return nil
}

type nFakeRepo struct {
	saved int
	err   error
}

func (f *nFakeRepo) SetAuthorizationFileKey(_ context.Context, _, _, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.saved++
	return nil
}

type nFakePublisher struct {
	events []NotifyEvent
	err    error
}

func (f *nFakePublisher) PublishAuthorizationStatusChanged(_ context.Context, e NotifyEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

type nFixedClock struct{ t time.Time }

func (f nFixedClock) Now() time.Time { return f.t }

type nSeqIDs struct {
	ids []string
	i   int
}

func (s *nSeqIDs) NewID() string {
	if s.i >= len(s.ids) {
		return s.ids[len(s.ids)-1]
	}
	id := s.ids[s.i]
	s.i++
	return id
}

func nItem(userID string) AuthorizationWithAppointment {
	authID := uuid.Must(uuid.NewV7()).String()
	return AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID: authID, UserID: userID, AppointmentID: uuid.Must(uuid.NewV7()).String(),
			Type: documents.AuthorizationTypeBloodTest, Status: documents.AuthorizationStatusInProgress,
		},
		Appointment: documents.CompletedAppointment{ID: "appt-1", UserID: userID},
	}
}

func nSnapshot(userID string) documents.ProfileSnapshot {
	return documents.ProfileSnapshot{
		UserID: userID, FirstName: "María", LastName: "Gómez",
		DocumentType: "CC", DocumentNumber: "1020304050", Email: "maria.gomez@example.com",
	}
}

func newNotify(reader *nFakeReader, stor *nFakeStorage, repo *nFakeRepo, pub *nFakePublisher, now time.Time, ids []string, buf *bytes.Buffer) *NotifyAuthorization {
	log := slog.New(slog.NewTextHandler(buf, nil))
	return NewNotifyAuthorization(
		&nFakeProfiles{snap: nSnapshot("u")}, reader, &nFakePDF{}, stor, repo, pub,
		nFixedClock{t: now}, &nSeqIDs{ids: ids}, "test-bucket", log,
	)
}

func TestNotifyHappyPath(t *testing.T) {
	uid, _ := uuid.NewV7()
	item := nItem(uid.String())
	eid, _ := uuid.NewV7()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reader := &nFakeReader{item: item}
	stor := &nFakeStorage{}
	repo := &nFakeRepo{}
	pub := &nFakePublisher{}
	var buf bytes.Buffer
	uc := newNotify(reader, stor, repo, pub, now, []string{eid.String()}, &buf)

	res, err := uc.Execute(context.Background(), NotifyCommand{
		UserID: uid.String(), FirstName: "Maria",
		Email: "jdanielrr98@gmail.com", AuthorizationID: item.Authorization.ID,
	})
	if err != nil {
		t.Fatalf("feliz debería pasar: %v", err)
	}
	if res.AuthorizationID != item.Authorization.ID || res.Status != string(item.Authorization.Status) {
		t.Fatalf("resultado incorrecto: %+v", res)
	}
	wantKey := "authorizations/u/" + item.Authorization.ID + ".pdf"
	if len(stor.keys) != 1 || stor.keys[0] != wantKey {
		t.Fatalf("S3 incorrecto: %v", stor.keys)
	}
	if repo.saved != 1 || len(pub.events) != 1 {
		t.Fatal("debe guardar clave y publicar")
	}
	evt := pub.events[0]
	if evt.Data.AuthorizationType != "Examen de sangre" || evt.Data.Status != "En proceso" {
		t.Fatalf("evento incorrecto: %+v", evt.Data)
	}
	if evt.Data.File.Bucket != "test-bucket" || evt.Data.File.Key != wantKey {
		t.Fatalf("file incorrecto: %+v", evt.Data.File)
	}
	if strings.Contains(buf.String(), "jdanielrr98@gmail.com") {
		t.Fatal("log contiene email completo")
	}
}

func TestNotifyForeignOrMissing(t *testing.T) {
	uc := newNotify(&nFakeReader{err: documents.ErrAuthorizationNotFound}, &nFakeStorage{}, &nFakeRepo{}, &nFakePublisher{}, time.Now().UTC(), []string{"e"}, &bytes.Buffer{})
	if _, err := uc.Execute(context.Background(), NotifyCommand{UserID: "u1", AuthorizationID: "ajena"}); !errors.Is(err, documents.ErrAuthorizationNotFound) {
		t.Fatalf("ajena debería ser ErrAuthorizationNotFound: %v", err)
	}
	if _, err := uc.Execute(context.Background(), NotifyCommand{}); !errors.Is(err, documents.ErrAuthorizationNotFound) {
		t.Fatalf("vacío debería ser ErrAuthorizationNotFound: %v", err)
	}
}

func TestNotifyMissingProfile(t *testing.T) {
	reader := &nFakeReader{item: nItem("u1")}
	uc := NewNotifyAuthorization(
		&nFakeProfiles{err: documents.ErrProfileNotFound}, reader, &nFakePDF{},
		&nFakeStorage{}, &nFakeRepo{}, &nFakePublisher{},
		nFixedClock{t: time.Now().UTC()}, &nSeqIDs{ids: []string{"e"}},
		"b", slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)
	if _, err := uc.Execute(context.Background(), NotifyCommand{UserID: "u1", AuthorizationID: "a1"}); !errors.Is(err, documents.ErrProfileNotFound) {
		t.Fatalf("sin perfil debería ser ErrProfileNotFound: %v", err)
	}
}

func TestNotifyUploadFails(t *testing.T) {
	reader := &nFakeReader{item: nItem("u1")}
	repo := &nFakeRepo{}
	pub := &nFakePublisher{}
	uc := newNotify(reader, &nFakeStorage{err: errors.New("s3 caído")}, repo, pub, time.Now().UTC(), []string{"e"}, &bytes.Buffer{})
	if _, err := uc.Execute(context.Background(), NotifyCommand{UserID: "u1", AuthorizationID: "a1"}); !errors.Is(err, documents.ErrStorage) {
		t.Fatalf("debería ser ErrStorage: %v", err)
	}
	if repo.saved != 0 || len(pub.events) != 0 {
		t.Fatal("no debe guardar ni publicar si falla la subida")
	}
}

func TestNotifyPublishFailsKeepsSaved(t *testing.T) {
	reader := &nFakeReader{item: nItem("u1")}
	repo := &nFakeRepo{}
	uc := newNotify(reader, &nFakeStorage{}, repo, &nFakePublisher{err: errors.New("bus caído")}, time.Now().UTC(), []string{"e"}, &bytes.Buffer{})
	if _, err := uc.Execute(context.Background(), NotifyCommand{UserID: "u1", AuthorizationID: "a1"}); !errors.Is(err, documents.ErrEventPublish) {
		t.Fatalf("debería ser ErrEventPublish: %v", err)
	}
	if repo.saved != 1 {
		t.Fatal("debería quedar guardado aunque falle la publicación")
	}
}
