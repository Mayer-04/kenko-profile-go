package seed

import (
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

func TestMakeExtraUsersDeterministic(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	first := MakeExtraUsers(7, now)
	second := MakeExtraUsers(7, now)
	if len(first) != 7 {
		t.Fatalf("extras = %d, quiero 7", len(first))
	}
	seenIDs := map[string]bool{}
	seenDocs := map[string]bool{}
	for i, u := range first {
		if u.UserID != second[i].UserID {
			t.Fatal("id no determinista")
		}
		if _, err := uuid.FromString(u.UserID); err != nil {
			t.Fatalf("userId no UUID: %v", err)
		}
		if seenIDs[u.UserID] || seenDocs[u.DocumentNumber] {
			t.Fatal("id o documento duplicado")
		}
		seenIDs[u.UserID] = true
		seenDocs[u.DocumentNumber] = true
		if !strings.HasSuffix(u.Email, "@ejemplo.invalid") {
			t.Fatalf("correo extra debe ser @ejemplo.invalid: %q", u.Email)
		}
		if allowedEmail(u.Email) {
			t.Fatal("correo extra no debe estar en la lista demo")
		}
		for field, value := range map[string]string{
			"document.type": u.DocumentType, "firstName": u.FirstName,
			"lastName": u.LastName, "birthDate": u.BirthDate, "phone": u.Phone,
			"address": u.AddressLine, "city": u.City,
		} {
			if value == "" {
				t.Fatalf("extra %d: %s vacío", i, field)
			}
		}
		if u.CreatedAt.IsZero() || u.CreatedAt.Location() != time.UTC {
			t.Fatal("createdAt debe ser UTC no cero")
		}
	}
	if MakeExtraUsers(0, now) == nil {
		t.Fatal("cero extras debe dar lista vacía no nula")
	}
}

func TestBuildAuthorizationsCycles(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	users := append(testUsers(), MakeExtraUsers(2, now)...)
	byUser := map[string][]documents.CompletedAppointment{}
	for _, u := range users {
		byUser[u.UserID] = GenerateAppointments(u, now)
	}
	// 5 usuarios → 5 autorizaciones ciclando la tabla 1→2→3→1→2.
	auths, err := BuildAuthorizations(users, byUser, now)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(auths) != 5 {
		t.Fatalf("autorizaciones = %d, quiero 5", len(auths))
	}
	wantTypes := []string{"BLOOD_TEST", "XRAY", "DENTAL_CLEANING", "BLOOD_TEST", "XRAY"}
	for i, a := range auths {
		if string(a.Type) != wantTypes[i] {
			t.Fatalf("auth %d tipo = %s, quiero %s", i, a.Type, wantTypes[i])
		}
		if a.UserID != users[i].UserID {
			t.Fatalf("auth %d no es del usuario %d", i, i)
		}
		if err := a.Validate(); err != nil {
			t.Fatalf("auth %d inválida: %v", i, err)
		}
	}
}
