package historyapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v5"
)

func testSecret() string { return "dev-secret" }

func signTestToken(t *testing.T, secret, userID, firstName, email string, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"userId": userID, "firstName": firstName, "email": email, "exp": exp.Unix(),
	})
	payloadSeg := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payloadSeg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payloadSeg + "." + sig
}

type stubReader struct {
	snap documents.ProfileSnapshot
	err  error
}

func (s stubReader) GetProfile(_ context.Context, _ string) (documents.ProfileSnapshot, error) {
	if s.err != nil {
		return documents.ProfileSnapshot{}, s.err
	}
	return s.snap, nil
}

type stubAppointments struct {
	list []documents.CompletedAppointment
	err  error
}

func (s stubAppointments) ListAppointments(_ context.Context, _ string) ([]documents.CompletedAppointment, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

type stubRepo struct{ saved int }

func (s *stubRepo) SaveHistoryExport(_ context.Context, _ documents.MedicalHistoryExport) error {
	s.saved++
	return nil
}

type stubPDF struct{}

func (stubPDF) Generate(_ documents.ProfileSnapshot, _ []documents.CompletedAppointment, _ documents.MedicalHistoryExport) ([]byte, error) {
	return []byte("%PDF-fake"), nil
}

type stubStorage struct{}

func (stubStorage) Upload(_ context.Context, _ string, _ []byte, _ string) error { return nil }

type stubPublisher struct{}

func (stubPublisher) PublishMedicalHistoryGenerated(_ context.Context, _ documents.HistoryEvent) error {
	return nil
}

type stubClock struct{ t time.Time }

func (s stubClock) Now() time.Time { return s.t }

type stubIDs struct {
	ids []string
	i   int
}

func (s *stubIDs) NewID() string {
	id := s.ids[s.i]
	if s.i < len(s.ids)-1 {
		s.i++
	}
	return id
}

func testHandler(t *testing.T, reader stubReader, appts stubAppointments) *Handler {
	t.Helper()
	pdfID, _ := uuid.NewV7()
	eventID, _ := uuid.NewV7()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := documents.NewGenerateHistory(
		reader, appts, &stubRepo{}, stubPDF{}, stubStorage{}, stubPublisher{},
		stubClock{t: now}, &stubIDs{ids: []string{pdfID.String(), eventID.String()}},
		"test-bucket", log,
	)
	return NewHandler(uc, testSecret())
}

func serveCreate(t *testing.T, h *Handler, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/medical-history", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.Create(c); err != nil {
		t.Fatalf("handler devolvió error: %v", err)
	}
	return rec
}

func TestCreateUnauthorized(t *testing.T) {
	uid, _ := uuid.NewV7()
	h := testHandler(t, stubReader{}, stubAppointments{})
	if rec := serveCreate(t, h, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token debería ser 401, fue %d", rec.Code)
	}
	expired := signTestToken(t, testSecret(), uid.String(), "Maria", "m@example.com", time.Now().Add(-time.Hour))
	if rec := serveCreate(t, h, "Bearer "+expired); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expirado debería ser 401, fue %d", rec.Code)
	}
}

func TestCreateNoRecords(t *testing.T) {
	uid, _ := uuid.NewV7()
	snap := documents.ProfileSnapshot{UserID: uid.String()}
	h := testHandler(t, stubReader{snap: snap}, stubAppointments{list: []documents.CompletedAppointment{}})
	token := signTestToken(t, testSecret(), uid.String(), "Maria", "m@example.com", time.Now().Add(time.Hour))
	if rec := serveCreate(t, h, "Bearer "+token); rec.Code != http.StatusNotFound {
		t.Fatalf("sin citas debería ser 404, fue %d", rec.Code)
	}
}

func TestCreateHappyPath(t *testing.T) {
	uid, _ := uuid.NewV7()
	snap := documents.ProfileSnapshot{
		UserID: uid.String(), FirstName: "María", LastName: "Gómez",
		DocumentType: "CC", DocumentNumber: "1020304050",
		AffiliationStartDate: time.Now().UTC(),
	}
	appts := []documents.CompletedAppointment{{ID: "a1", UserID: uid.String()}}
	h := testHandler(t, stubReader{snap: snap}, stubAppointments{list: appts})
	token := signTestToken(t, testSecret(), uid.String(), "Maria Fernanda", "jdanielrr98@gmail.com", time.Now().Add(time.Hour))
	rec := serveCreate(t, h, "Bearer "+token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("feliz debería ser 201, fue %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if body["pdfId"] == nil || body["recordCount"] == nil {
		t.Fatalf("respuesta incompleta: %v", body)
	}
}
