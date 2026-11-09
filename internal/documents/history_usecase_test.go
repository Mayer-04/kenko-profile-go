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

type hFakeReader struct {
	snap documents_ProfileSnapshotAlias
	err  error
}

type documents_ProfileSnapshotAlias = ProfileSnapshot

func (f *hFakeReader) GetProfile(_ context.Context, _ string) (ProfileSnapshot, error) {
	if f.err != nil {
		return ProfileSnapshot{}, f.err
	}
	return f.snap, nil
}

type hFakeAppointments struct {
	list []CompletedAppointment
	err  error
}

func (f *hFakeAppointments) ListAppointments(_ context.Context, _ string) ([]CompletedAppointment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

type hFakeRepo struct {
	saved []MedicalHistoryExport
	err   error
}

func (f *hFakeRepo) SaveHistoryExport(_ context.Context, e MedicalHistoryExport) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, e)
	return nil
}

type hFakePDF struct {
	err   error
	calls int
}

func (f *hFakePDF) Generate(_ ProfileSnapshot, _ []CompletedAppointment, _ MedicalHistoryExport) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return []byte("%PDF-fake"), nil
}

type hFakeStorage struct {
	keys []string
	cts  []string
	err  error
}

func (f *hFakeStorage) Upload(_ context.Context, key string, _ []byte, ct string) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	f.cts = append(f.cts, ct)
	return nil
}

type hFakePublisher struct {
	events []HistoryEvent
	err    error
}

func (f *hFakePublisher) PublishMedicalHistoryGenerated(_ context.Context, e HistoryEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

type hFixedClock struct{ t time.Time }

func (f hFixedClock) Now() time.Time { return f.t }

type hSeqIDs struct {
	ids []string
	i   int
}

func (s *hSeqIDs) NewID() string {
	if s.i >= len(s.ids) {
		return s.ids[len(s.ids)-1]
	}
	id := s.ids[s.i]
	s.i++
	return id
}

func hSnapshot() ProfileSnapshot {
	uid, _ := uuid.NewV7()
	return ProfileSnapshot{
		UserID:               uid.String(),
		FirstName:            "María",
		LastName:             "Gómez",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
}

func hAppointment(userID string) CompletedAppointment {
	return CompletedAppointment{
		ID:        uuid.Must(uuid.NewV7()).String(),
		UserID:    userID,
		Type:      AppointmentTypeMedicinaGeneral,
		Date:      "2026-05-10",
		StartTime: "08:00",
		EndTime:   "08:30",
		Physician: Physician{PhysicianID: "MED-1", Name: "Dra. Laura Cifuentes"},
		MedicalCenter: MedicalCenter{
			MedicalCenterID: "SEDE-1",
			Name:            "Sede Norte",
			Location:        "Bogota",
		},
		Status:      AppointmentStatusCompleted,
		Notes:       "Control general.",
		CompletedAt: time.Date(2026, 5, 10, 13, 30, 0, 0, time.UTC),
	}
}

func newHistoryUsecase(reader *hFakeReader, appts *hFakeAppointments, repo *hFakeRepo, pdf *hFakePDF, stor *hFakeStorage, pub *hFakePublisher, now time.Time, ids []string, buf *bytes.Buffer) *GenerateHistory {
	log := slog.New(slog.NewTextHandler(buf, nil))
	return NewGenerateHistory(reader, appts, repo, pdf, stor, pub, hFixedClock{t: now}, &hSeqIDs{ids: ids}, "test-bucket", log)
}

func TestHistoryExecuteHappyPath(t *testing.T) {
	snap := hSnapshot()
	appts := []CompletedAppointment{hAppointment(snap.UserID), hAppointment(snap.UserID)}
	pdfID, _ := uuid.NewV7()
	eventID, _ := uuid.NewV7()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: appts}
	repo := &hFakeRepo{}
	pdfGen := &hFakePDF{}
	stor := &hFakeStorage{}
	pub := &hFakePublisher{}
	var buf bytes.Buffer
	uc := newHistoryUsecase(reader, fa, repo, pdfGen, stor, pub, now, []string{pdfID.String(), eventID.String()}, &buf)

	corr, _ := uuid.NewV7()
	res, err := uc.Execute(context.Background(), HistoryCommand{
		UserID: snap.UserID, FirstName: "Maria",
		Email: "jdanielrr98@gmail.com", CorrelationID: corr.String(),
	})
	if err != nil {
		t.Fatalf("feliz debería pasar: %v", err)
	}
	if res.PdfID != pdfID.String() || res.RecordCount != 2 {
		t.Fatalf("resultado incorrecto: %+v", res)
	}
	wantKey := "medical-history/" + snap.UserID + "/" + pdfID.String() + ".pdf"
	if len(stor.keys) != 1 || stor.keys[0] != wantKey || stor.cts[0] != "application/pdf" {
		t.Fatalf("S3 incorrecto: %v %v", stor.keys, stor.cts)
	}
	if len(repo.saved) != 1 || repo.saved[0].FileKey != wantKey {
		t.Fatalf("repo incorrecto: %+v", repo.saved)
	}
	if len(pub.events) != 1 {
		t.Fatal("debería publicar 1 evento")
	}
	evt := pub.events[0]
	if evt.Data.PdfID != pdfID.String() || evt.Data.RecordCount != 2 {
		t.Fatalf("evento incorrecto: %+v", evt.Data)
	}
	if evt.Metadata.CorrelationID != corr.String() || evt.Metadata.EventID != eventID.String() {
		t.Fatalf("metadata incorrecta: %+v", evt.Metadata)
	}
	if strings.Contains(buf.String(), "jdanielrr98@gmail.com") {
		t.Fatal("log contiene email completo")
	}
}

func TestHistoryExecuteNoProfile(t *testing.T) {
	reader := &hFakeReader{err: ErrProfileNotFound}
	fa := &hFakeAppointments{}
	var buf bytes.Buffer
	uc := newHistoryUsecase(reader, fa, &hFakeRepo{}, &hFakePDF{}, &hFakeStorage{}, &hFakePublisher{}, time.Now().UTC(), []string{"a", "b"}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: "x"}); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("debería ser ErrProfileNotFound: %v", err)
	}
}

func TestHistoryExecuteNoRecords(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{}}
	var buf bytes.Buffer
	pdfGen := &hFakePDF{}
	stor := &hFakeStorage{}
	pub := &hFakePublisher{}
	uc := newHistoryUsecase(reader, fa, &hFakeRepo{}, pdfGen, stor, pub, time.Now().UTC(), []string{"a", "b"}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); !errors.Is(err, ErrNoMedicalRecords) {
		t.Fatalf("debería ser ErrNoMedicalRecords: %v", err)
	}
	if pdfGen.calls != 0 || len(stor.keys) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe generar, subir ni publicar sin citas")
	}
}

func TestHistoryExecuteUploadFails(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{hAppointment(snap.UserID)}}
	repo := &hFakeRepo{}
	pub := &hFakePublisher{}
	var buf bytes.Buffer
	uc := newHistoryUsecase(reader, fa, repo, &hFakePDF{}, &hFakeStorage{err: errors.New("s3 caído")}, pub, time.Now().UTC(), []string{"a", "b"}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); !errors.Is(err, ErrStorage) {
		t.Fatalf("debería ser ErrStorage: %v", err)
	}
	if len(repo.saved) != 0 || len(pub.events) != 0 {
		t.Fatal("no debe guardar ni publicar si falla la subida")
	}
}

func TestHistoryExecutePublishFailsKeepsSaved(t *testing.T) {
	snap := hSnapshot()
	reader := &hFakeReader{snap: snap}
	fa := &hFakeAppointments{list: []CompletedAppointment{hAppointment(snap.UserID)}}
	repo := &hFakeRepo{}
	var buf bytes.Buffer
	uc := newHistoryUsecase(reader, fa, repo, &hFakePDF{}, &hFakeStorage{}, &hFakePublisher{err: errors.New("bus caído")}, time.Now().UTC(), []string{"a", "b"}, &buf)
	if _, err := uc.Execute(context.Background(), HistoryCommand{UserID: snap.UserID}); !errors.Is(err, ErrEventPublish) {
		t.Fatalf("debería ser ErrEventPublish: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatal("debería quedar guardado aunque falle la publicación")
	}
}
