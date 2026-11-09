package authzapi

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
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v5"
)

func testSecret() string { return "dev-secret" }

func signTestToken(t *testing.T, secret, userID string, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"userId": userID, "firstName": "Maria", "email": "m@example.com", "exp": exp.Unix(),
	})
	payloadSeg := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payloadSeg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payloadSeg + "." + sig
}

type stubReader struct {
	items []authz.AuthorizationWithAppointment
	byID  map[string]authz.AuthorizationWithAppointment
}

func (s stubReader) ListByUser(_ context.Context, _ string) ([]authz.AuthorizationWithAppointment, error) {
	return s.items, nil
}

func (s stubReader) GetByID(_ context.Context, _, id string) (authz.AuthorizationWithAppointment, error) {
	if it, ok := s.byID[id]; ok {
		return it, nil
	}
	return authz.AuthorizationWithAppointment{}, documents.ErrAuthorizationNotFound
}

type stubProfiles struct{}

func (stubProfiles) GetProfile(_ context.Context, _ string) (documents.ProfileSnapshot, error) {
	return documents.ProfileSnapshot{UserID: "u1"}, nil
}

type stubPDF struct{ err error }

func (s stubPDF) GenerateAuthorization(_ documents.ProfileSnapshot, _ authz.AuthorizationWithAppointment) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []byte("%PDF-fake"), nil
}

func testHandler(items []authz.AuthorizationWithAppointment, byID map[string]authz.AuthorizationWithAppointment) *Handler {
	return testHandlerWithNotify(items, byID, nil)
}

func testHandlerWithNotify(items []authz.AuthorizationWithAppointment, byID map[string]authz.AuthorizationWithAppointment, notifier *authz.NotifyAuthorization) *Handler {
	lister := authz.NewListAuthorizations(stubReader{items: items, byID: byID})
	getter := authz.NewGetAuthorizationPDF(stubProfiles{}, stubReader{items: items, byID: byID}, stubPDF{})
	return NewHandler(lister, getter, notifier, testSecret())
}

func TestListUnauthorized(t *testing.T) {
	h := testHandler(nil, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/authorizations", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token debería ser 401, fue %d", rec.Code)
	}
}

func TestListHappyPath(t *testing.T) {
	uid, _ := uuid.NewV7()
	item := authz.AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID: "a1", UserID: uid.String(), AppointmentID: "appt-1",
			Type: documents.AuthorizationTypeBloodTest, Status: documents.AuthorizationStatusInProgress,
		},
		Appointment: documents.CompletedAppointment{
			ID: "appt-1", UserID: uid.String(), Type: documents.AppointmentTypeMedicinaGeneral,
			Physician: documents.Physician{Name: "Dra. X"},
		},
	}
	h := testHandler([]authz.AuthorizationWithAppointment{item}, map[string]authz.AuthorizationWithAppointment{"a1": item})
	token := signTestToken(t, testSecret(), uid.String(), time.Now().Add(time.Hour))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/authorizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.List(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("lista debería ser 200, fue %d", rec.Code)
	}
}

func TestPDFNotFound(t *testing.T) {
	uid, _ := uuid.NewV7()
	h := testHandler(nil, map[string]authz.AuthorizationWithAppointment{})
	token := signTestToken(t, testSecret(), uid.String(), time.Now().Add(time.Hour))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/authorizations/ajena/pdf", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "ajena"}})
	if err := h.PDF(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajena debería ser 404, fue %d", rec.Code)
	}
}

func TestPDFHappyPath(t *testing.T) {
	uid, _ := uuid.NewV7()
	item := authz.AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID: "a1", UserID: uid.String(), AppointmentID: "appt-1",
			Type: documents.AuthorizationTypeBloodTest, Status: documents.AuthorizationStatusApproved,
		},
		Appointment: documents.CompletedAppointment{ID: "appt-1", UserID: uid.String()},
	}
	h := testHandler([]authz.AuthorizationWithAppointment{item}, map[string]authz.AuthorizationWithAppointment{"a1": item})
	token := signTestToken(t, testSecret(), uid.String(), time.Now().Add(time.Hour))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/authorizations/a1/pdf", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "a1"}})
	if err := h.PDF(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("pdf debería ser 200, fue %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Fatal("falta Content-Disposition")
	}
}

func testNotifyHandler(byID map[string]authz.AuthorizationWithAppointment) *Handler {
	lister := authz.NewListAuthorizations(stubReader{})
	getter := authz.NewGetAuthorizationPDF(stubProfiles{}, stubReader{byID: byID}, stubPDF{})
	eid, _ := uuid.NewV7()
	notifier := authz.NewNotifyAuthorization(
		stubProfiles{}, stubReader{byID: byID}, stubPDF{}, stubNStorage{}, stubNRepo{},
		stubNPublisher{}, stubNClock{}, &stubNIDs{ids: []string{eid.String()}},
		"test-bucket", testSlog(),
	)
	return NewHandler(lister, getter, notifier, testSecret())
}

func TestNotifyUnauthorized(t *testing.T) {
	h := testNotifyHandler(nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/authorizations/a1/notify", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "a1"}})
	if err := h.Notify(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token debería ser 401, fue %d", rec.Code)
	}
}

func TestNotifyNotFound(t *testing.T) {
	uid, _ := uuid.NewV7()
	h := testNotifyHandler(map[string]authz.AuthorizationWithAppointment{})
	token := signTestToken(t, testSecret(), uid.String(), time.Now().Add(time.Hour))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/authorizations/ajena/notify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "ajena"}})
	if err := h.Notify(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajena debería ser 404, fue %d", rec.Code)
	}
}

func TestNotifyAccepted(t *testing.T) {
	uid, _ := uuid.NewV7()
	authID := uuid.Must(uuid.NewV7()).String()
	item := authz.AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID: authID, UserID: uid.String(), AppointmentID: uuid.Must(uuid.NewV7()).String(),
			Type: documents.AuthorizationTypeBloodTest, Status: documents.AuthorizationStatusInProgress,
		},
		Appointment: documents.CompletedAppointment{ID: "appt-1", UserID: uid.String()},
	}
	h := testNotifyHandler(map[string]authz.AuthorizationWithAppointment{authID: item})
	token := signTestToken(t, testSecret(), uid.String(), time.Now().Add(time.Hour))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/authorizations/"+authID+"/notify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: authID}})
	if err := h.Notify(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("notify debería ser 202, fue %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if body["authorizationId"] != authID || body["status"] == "" {
		t.Fatalf("respuesta incompleta: %v", body)
	}
}

type stubNStorage struct{}

func (stubNStorage) Upload(_ context.Context, _ string, _ []byte, _ string) error { return nil }

type stubNRepo struct{}

func (stubNRepo) SetAuthorizationFileKey(_ context.Context, _, _, _ string) error { return nil }

type stubNPublisher struct{}

func (stubNPublisher) PublishAuthorizationStatusChanged(_ context.Context, _ authz.NotifyEvent) error {
	return nil
}

type stubNClock struct{ t time.Time }

func (s stubNClock) Now() time.Time {
	if s.t.IsZero() {
		return time.Now().UTC()
	}
	return s.t
}

type stubNIDs struct {
	ids []string
	i   int
}

func (s *stubNIDs) NewID() string {
	id := s.ids[s.i]
	if s.i < len(s.ids)-1 {
		s.i++
	}
	return id
}

func testSlog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
