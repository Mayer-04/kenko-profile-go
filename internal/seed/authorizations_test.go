package seed

import (
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
)

func authzTestUsers() []DemoUser {
	users := testUsers()
	// testUsers ya trae los 3 correos y createdAt válidos.
	return users
}

func TestBuildAuthorizationsOnePerUser(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	users := authzTestUsers()
	byUser := map[string][]documents.CompletedAppointment{}
	for _, u := range users {
		byUser[u.UserID] = GenerateAppointments(u, now)
	}
	first, err := BuildAuthorizations(users, byUser, now)
	if err != nil {
		t.Fatalf("build falló: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("autorizaciones = %d, quiero 3", len(first))
	}
	// Tres tipos y tres estados cubiertos.
	types := map[documents.AuthorizationType]bool{}
	statuses := map[documents.AuthorizationStatus]bool{}
	for _, a := range first {
		types[a.Type] = true
		statuses[a.Status] = true
		if err := a.Validate(); err != nil {
			t.Fatalf("autorización inválida: %v", err)
		}
	}
	for _, typ := range []documents.AuthorizationType{
		documents.AuthorizationTypeBloodTest,
		documents.AuthorizationTypeXRay,
		documents.AuthorizationTypeDentalCleaning,
	} {
		if !types[typ] {
			t.Fatalf("falta tipo %s", typ)
		}
	}
	for _, st := range []documents.AuthorizationStatus{
		documents.AuthorizationStatusInProgress,
		documents.AuthorizationStatusRejected,
		documents.AuthorizationStatusApproved,
	} {
		if !statuses[st] {
			t.Fatalf("falta estado %s", st)
		}
	}
	// Determinismo: dos ejecuciones dan los mismos ids.
	second, err := BuildAuthorizations(users, byUser, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("id no determinista")
		}
		if first[i].UserID != users[i].UserID {
			t.Fatal("autorización no es del usuario")
		}
	}
	// Reglas de updatedAt.
	for _, a := range first {
		if a.Status == documents.AuthorizationStatusInProgress {
			if !a.UpdatedAt.Equal(a.CreatedAt) {
				t.Fatal("IN_PROGRESS: updatedAt debe igualar createdAt")
			}
		} else if a.UpdatedAt.Before(a.CreatedAt) || a.UpdatedAt.After(now) {
			t.Fatal("updatedAt fuera de rango")
		}
	}
	// Motivo solo en rechazadas.
	for _, a := range first {
		if a.Status == documents.AuthorizationStatusRejected && a.RejectionReason == "" {
			t.Fatal("rechazada sin motivo")
		}
		if a.Status != documents.AuthorizationStatusRejected && a.RejectionReason != "" {
			t.Fatal("motivo fuera de rechazada")
		}
	}
}

func TestBuildAuthorizationsRejectsIncompatible(t *testing.T) {
	now := time.Now().UTC()
	users := authzTestUsers()
	// Solo citas de psicología: ninguna asignación es compatible.
	byUser := map[string][]documents.CompletedAppointment{}
	for _, u := range users {
		all := GenerateAppointments(u, now)
		var psico []documents.CompletedAppointment
		for _, a := range all {
			if a.Type == documents.AppointmentTypePsicologia {
				psico = append(psico, a)
			}
		}
		byUser[u.UserID] = psico
	}
	if _, err := BuildAuthorizations(users, byUser, now); err == nil {
		t.Fatal("sin cita compatible debería fallar")
	}
}
