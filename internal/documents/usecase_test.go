package documents

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

type fakeReader struct {
	snap  ProfileSnapshot
	err   error
	calls int
}

func (f *fakeReader) GetProfile(_ context.Context, _ string) (ProfileSnapshot, error) {
	f.calls++
	if f.err != nil {
		return ProfileSnapshot{}, f.err
	}
	return f.snap, nil
}

type fakeRepo struct {
	saved []Certificate
	err   error
}

func (f *fakeRepo) Save(_ context.Context, c Certificate) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, c)
	return nil
}

type fakePDF struct {
	data  []byte
	err   error
	calls int
}

func (f *fakePDF) Generate(_ ProfileSnapshot, _ Certificate) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.data != nil {
		return f.data, nil
	}
	return []byte("%PDF-fake"), nil
}

type fakeStorage struct {
	keys []string
	cts  []string
	err  error
}

func (f *fakeStorage) Upload(_ context.Context, key string, _ []byte, ct string) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	f.cts = append(f.cts, ct)
	return nil
}

type fakePublisher struct {
	events []CertificateEvent
	err    error
}

func (f *fakePublisher) PublishCertificateGenerated(_ context.Context, e CertificateEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type seqIDs struct {
	ids []string
	i   int
}

func (s *seqIDs) NewID() string {
	if s.i >= len(s.ids) {
		last := s.ids[len(s.ids)-1]
		return last
	}
	id := s.ids[s.i]
	s.i++
	return id
}

func testSnapshot() ProfileSnapshot {
	uid, _ := uuid.NewV7()
	return ProfileSnapshot{
		UserID:               uid.String(),
		FirstName:            "María Fernanda",
		LastName:             "Gómez Rojas",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
}

func newUsecase(reader *fakeReader, repo *fakeRepo, pdfGen *fakePDF, stor *fakeStorage, pub *fakePublisher, now time.Time, ids []string, buf *bytes.Buffer) *GenerateCertificate {
	log := slog.New(slog.NewTextHandler(buf, nil))
	return NewGenerateCertificate(reader, repo, pdfGen, stor, pub, fixedClock{t: now}, &seqIDs{ids: ids}, "test-bucket", log)
}

func TestExecuteHappyPath(t *testing.T) {
	snap := testSnapshot()
	certID, _ := uuid.NewV7()
	eventID, _ := uuid.NewV7()
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	uc := newUsecase(reader, repo, pdfGen, stor, pub, now, []string{certID.String(), eventID.String()}, &buf)

	corr, _ := uuid.NewV7()
	res, err := uc.Execute(context.Background(), GenerateCommand{
		UserID: snap.UserID, FirstName: "Maria Fernanda",
		Email: "maria.gomez@example.com", CorrelationID: corr.String(),
	})
	if err != nil {
		t.Fatalf("feliz debería pasar: %v", err)
	}
	if res.CertificateID != certID.String() {
		t.Fatalf("certificateId = %q, quiero %q", res.CertificateID, certID.String())
	}
	if !res.AffiliationStartDate.Equal(snap.AffiliationStartDate) {
		t.Fatalf("affiliationStartDate no se copió del perfil")
	}
	// S3
	wantKey := "certificates/" + snap.UserID + "/" + certID.String() + ".pdf"
	if len(stor.keys) != 1 || stor.keys[0] != wantKey {
		t.Fatalf("key S3 = %v, quiero %q", stor.keys, wantKey)
	}
	if stor.cts[0] != "application/pdf" {
		t.Fatalf("contentType = %q", stor.cts[0])
	}
	// Mongo
	if len(repo.saved) != 1 || repo.saved[0].FileKey != wantKey {
		t.Fatalf("repo no guardó con fileKey correcta: %+v", repo.saved)
	}
	// Evento exacto
	if len(pub.events) != 1 {
		t.Fatalf("debería publicar 1 evento")
	}
	evt := pub.events[0]
	if evt.Metadata.EventID != eventID.String() || evt.Metadata.EventVersion != "1" || evt.Metadata.CorrelationID != corr.String() {
		t.Fatalf("metadata incorrecta: %+v", evt.Metadata)
	}
	if evt.Data.CertificateID != certID.String() || evt.Data.UserID != snap.UserID || evt.Data.Email != "maria.gomez@example.com" {
		t.Fatalf("data incorrecta: %+v", evt.Data)
	}
	if evt.Data.File.Bucket != "test-bucket" || evt.Data.File.Key != wantKey || evt.Data.File.ContentType != "application/pdf" {
		t.Fatalf("file incorrecto: %+v", evt.Data.File)
	}
	if !strings.HasPrefix(evt.Data.File.FileName, "certificado-afiliacion-") {
		t.Fatalf("fileName incorrecto: %q", evt.Data.File.FileName)
	}
	// Sin PII completa en logs
	out := buf.String()
	if strings.Contains(out, "maria.gomez@example.com") {
		t.Fatalf("log contiene email completo")
	}
}

func TestExecuteProfileNotFoundDoesNothing(t *testing.T) {
	reader := &fakeReader{err: ErrProfileNotFound}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	now := time.Now().UTC()
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, now, []string{c1.String(), c2.String()}, &buf)

	_, err := uc.Execute(context.Background(), GenerateCommand{UserID: c1.String()})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("debería ser ErrProfileNotFound, fue %v", err)
	}
	if pdfGen.calls != 0 || len(stor.keys) != 0 || len(repo.saved) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe generar, subir, guardar ni publicar si no hay perfil")
	}
}

func TestExecuteUploadFailsDoesNotSaveOrPublish(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{err: errors.New("s3 caído")}
	pub := &fakePublisher{}
	var buf bytes.Buffer
	now := time.Now().UTC()
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, now, []string{c1.String(), c2.String()}, &buf)

	_, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID})
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("debería ser ErrStorage, fue %v", err)
	}
	if len(repo.saved) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe guardar ni publicar si falla la subida")
	}
}

func TestExecutePublishFailsKeepsSaved(t *testing.T) {
	snap := testSnapshot()
	reader := &fakeReader{snap: snap}
	repo := &fakeRepo{}
	pdfGen := &fakePDF{}
	stor := &fakeStorage{}
	pub := &fakePublisher{err: errors.New("bus caído")}
	var buf bytes.Buffer
	now := time.Now().UTC()
	c1, _ := uuid.NewV7()
	c2, _ := uuid.NewV7()
	uc := newUsecase(reader, repo, pdfGen, stor, pub, now, []string{c1.String(), c2.String()}, &buf)

	_, err := uc.Execute(context.Background(), GenerateCommand{UserID: snap.UserID})
	if !errors.Is(err, ErrEventPublish) {
		t.Fatalf("debería ser ErrEventPublish, fue %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatal("debería quedar guardado aunque falle la publicación")
	}
}
