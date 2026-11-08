package httpapi

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

type stubRepo struct{ saved int }

func (s *stubRepo) Save(_ context.Context, _ documents.Certificate) error { s.saved++; return nil }

type stubPDF struct{}

func (stubPDF) Generate(_ documents.ProfileSnapshot, _ documents.Certificate) ([]byte, error) {
	return []byte("%PDF-fake"), nil
}

type stubStorage struct{}

func (stubStorage) Upload(_ context.Context, _ string, _ []byte, _ string) error { return nil }

type stubPublisher struct{}

func (stubPublisher) PublishCertificateGenerated(_ context.Context, _ documents.CertificateEvent) error {
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

func testHandler(t *testing.T, reader stubReader) *Handler {
	t.Helper()
	certID, _ := uuid.NewV7()
	eventID, _ := uuid.NewV7()
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := documents.NewGenerateCertificate(
		reader, &stubRepo{}, stubPDF{}, stubStorage{}, stubPublisher{},
		stubClock{t: now}, &stubIDs{ids: []string{certID.String(), eventID.String()}},
		"test-bucket", log,
	)
	return NewHandler(uc, testSecret())
}

func serveCreate(t *testing.T, h *Handler, authHeader, corrID string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/certificates", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if corrID != "" {
		req.Header.Set("X-Correlation-Id", corrID)
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
	snap := documents.ProfileSnapshot{UserID: uid.String(), AffiliationStartDate: time.Now().UTC()}
	h := testHandler(t, stubReader{snap: snap})

	if rec := serveCreate(t, h, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token debería ser 401, fue %d", rec.Code)
	}
	if rec := serveCreate(t, h, "Bearer invalido", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token inválido debería ser 401, fue %d", rec.Code)
	}
	expired := signTestToken(t, testSecret(), uid.String(), "Maria", "m@example.com", time.Now().Add(-time.Hour))
	if rec := serveCreate(t, h, "Bearer "+expired, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expirado debería ser 401, fue %d", rec.Code)
	}
}

func TestCreateNotFound(t *testing.T) {
	uid, _ := uuid.NewV7()
	h := testHandler(t, stubReader{err: documents.ErrProfileNotFound})
	token := signTestToken(t, testSecret(), uid.String(), "Maria", "m@example.com", time.Now().Add(time.Hour))
	if rec := serveCreate(t, h, "Bearer "+token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("sin perfil debería ser 404, fue %d", rec.Code)
	}
}

func TestCreateHappyPath(t *testing.T) {
	uid, _ := uuid.NewV7()
	snap := documents.ProfileSnapshot{
		UserID:               uid.String(),
		FirstName:            "María",
		LastName:             "Gómez",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
	h := testHandler(t, stubReader{snap: snap})
	token := signTestToken(t, testSecret(), uid.String(), "Maria Fernanda", "maria.gomez@example.com", time.Now().Add(time.Hour))
	rec := serveCreate(t, h, "Bearer "+token, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("feliz debería ser 201, fue %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if body["certificateId"] == "" || body["issuedAt"] == "" || body["affiliationStartDate"] == "" {
		t.Fatalf("respuesta incompleta: %v", body)
	}
}
