package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
)

type fakeReader struct {
	items []AuthorizationWithAppointment
	byID  map[string]AuthorizationWithAppointment
	err   error
}

func (f *fakeReader) ListByUser(_ context.Context, _ string) ([]AuthorizationWithAppointment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *fakeReader) GetByID(_ context.Context, _, id string) (AuthorizationWithAppointment, error) {
	if f.err != nil {
		return AuthorizationWithAppointment{}, f.err
	}
	if it, ok := f.byID[id]; ok {
		return it, nil
	}
	return AuthorizationWithAppointment{}, documents.ErrAuthorizationNotFound
}

type fakeProfiles struct {
	snap documents.ProfileSnapshot
	err  error
}

func (f *fakeProfiles) GetProfile(_ context.Context, _ string) (documents.ProfileSnapshot, error) {
	if f.err != nil {
		return documents.ProfileSnapshot{}, f.err
	}
	return f.snap, nil
}

type fakePDF struct {
	data []byte
	err  error
}

func (f *fakePDF) GenerateAuthorization(_ documents.ProfileSnapshot, _ AuthorizationWithAppointment) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.data != nil {
		return f.data, nil
	}
	return []byte("%PDF-fake"), nil
}

func testItem(userID, authID string) AuthorizationWithAppointment {
	return AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID:            authID,
			UserID:        userID,
			AppointmentID: "appt-1",
			Type:          documents.AuthorizationTypeBloodTest,
			Status:        documents.AuthorizationStatusInProgress,
		},
		Appointment: documents.CompletedAppointment{
			ID:     "appt-1",
			UserID: userID,
			Type:   documents.AppointmentTypeMedicinaGeneral,
		},
	}
}

func TestListOrderedOwnUser(t *testing.T) {
	items := []AuthorizationWithAppointment{
		testItem("u1", "a2"), testItem("u1", "a1"),
	}
	lister := NewListAuthorizations(&fakeReader{items: items})
	got, err := lister.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("list debería pasar: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("citas = %d, quiero 2", len(got))
	}
}

func TestListEmpty(t *testing.T) {
	lister := NewListAuthorizations(&fakeReader{})
	got, err := lister.Execute(context.Background(), "u1")
	if err != nil || len(got) != 0 {
		t.Fatalf("vacío debería dar [] sin error: %v %v", got, err)
	}
	if _, err := lister.Execute(context.Background(), " "); err == nil {
		t.Fatal("userId vacío debería fallar")
	}
}

func TestListReaderError(t *testing.T) {
	lister := NewListAuthorizations(&fakeReader{err: errors.New("mongo caído")})
	if _, err := lister.Execute(context.Background(), "u1"); err == nil {
		t.Fatal("error de infra debería propagarse")
	}
}

func TestPDFHappyPath(t *testing.T) {
	item := testItem("u1", "a1")
	getter := NewGetAuthorizationPDF(
		&fakeProfiles{snap: documents.ProfileSnapshot{UserID: "u1"}},
		&fakeReader{byID: map[string]AuthorizationWithAppointment{"a1": item}},
		&fakePDF{data: []byte("%PDF-fake")},
	)
	res, err := getter.Execute(context.Background(), "u1", "a1")
	if err != nil {
		t.Fatalf("feliz debería pasar: %v", err)
	}
	if len(res.Bytes) == 0 || res.FileName == "" {
		t.Fatal("faltan bytes o fileName")
	}
}

func TestPDFForeignOrMissing(t *testing.T) {
	getter := NewGetAuthorizationPDF(
		&fakeProfiles{snap: documents.ProfileSnapshot{UserID: "u1"}},
		&fakeReader{byID: map[string]AuthorizationWithAppointment{}},
		&fakePDF{},
	)
	if _, err := getter.Execute(context.Background(), "u1", "ajena"); !errors.Is(err, documents.ErrAuthorizationNotFound) {
		t.Fatalf("ajena debería ser ErrAuthorizationNotFound: %v", err)
	}
	if _, err := getter.Execute(context.Background(), "", ""); !errors.Is(err, documents.ErrAuthorizationNotFound) {
		t.Fatalf("vacío debería ser ErrAuthorizationNotFound: %v", err)
	}
}

func TestPDFMissingProfile(t *testing.T) {
	item := testItem("u1", "a1")
	getter := NewGetAuthorizationPDF(
		&fakeProfiles{err: documents.ErrProfileNotFound},
		&fakeReader{byID: map[string]AuthorizationWithAppointment{"a1": item}},
		&fakePDF{},
	)
	if _, err := getter.Execute(context.Background(), "u1", "a1"); !errors.Is(err, documents.ErrProfileNotFound) {
		t.Fatalf("sin perfil debería ser ErrProfileNotFound: %v", err)
	}
}

func TestPDFGenerationFails(t *testing.T) {
	item := testItem("u1", "a1")
	getter := NewGetAuthorizationPDF(
		&fakeProfiles{snap: documents.ProfileSnapshot{UserID: "u1"}},
		&fakeReader{byID: map[string]AuthorizationWithAppointment{"a1": item}},
		&fakePDF{err: errors.New("pdf roto")},
	)
	if _, err := getter.Execute(context.Background(), "u1", "a1"); !errors.Is(err, documents.ErrStorage) {
		t.Fatalf("pdf roto debería ser ErrStorage: %v", err)
	}
}
