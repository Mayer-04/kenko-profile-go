package pdf

import (
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	"github.com/gofrs/uuid/v5"
)

func authzSnapshot() documents.ProfileSnapshot {
	uid, _ := uuid.NewV7()
	return documents.ProfileSnapshot{
		UserID:               uid.String(),
		FirstName:            "María",
		LastName:             "Gómez Rojas",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		Email:                "maria.gomez@example.com",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
}

func authzItem(snap documents.ProfileSnapshot, status documents.AuthorizationStatus, reason documents.RejectionReason) authz.AuthorizationWithAppointment {
	apptID := uuid.Must(uuid.NewV7()).String()
	return authz.AuthorizationWithAppointment{
		Authorization: documents.Authorization{
			ID:              uuid.Must(uuid.NewV7()).String(),
			UserID:          snap.UserID,
			AppointmentID:   apptID,
			Type:            documents.AuthorizationTypeBloodTest,
			Status:          status,
			RejectionReason: reason,
			CreatedAt:       time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC),
			UpdatedAt:       time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC),
		},
		Appointment: documents.CompletedAppointment{
			ID:        apptID,
			UserID:    snap.UserID,
			Type:      documents.AppointmentTypeMedicinaGeneral,
			Date:      "2026-09-15",
			StartTime: "09:30",
			EndTime:   "10:00",
			Physician: documents.Physician{PhysicianID: "MED-1", Name: "Dra. Laura Cifuentes"},
			MedicalCenter: documents.MedicalCenter{
				MedicalCenterID: "SEDE-1",
				Name:            "Sede Norte",
				Location:        "Calle 127 # 45-10, Bogota",
			},
			Status:      documents.AppointmentStatusCompleted,
			Notes:       "Control general.",
			CompletedAt: time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC),
		},
	}
}

func TestGenerateAuthorizationStartsWithPDF(t *testing.T) {
	snap := authzSnapshot()
	item := authzItem(snap, documents.AuthorizationStatusApproved, "")
	data, err := NewGenerator().GenerateAuthorization(snap, item)
	if err != nil {
		t.Fatalf("generar falló: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatal("no empieza con %PDF")
	}
	for _, want := range []string{
		"Autorizaci", item.Authorization.ID, "Examen de sangre",
		"15/09/2026", "1020304050", "maria.gomez@example.com",
		"Laura Cifuentes", "Aprobada",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("pdf debería contener %q", want)
		}
	}
	if strings.Contains(string(data), "Motivo del rechazo") {
		t.Fatal("aprobada no debe traer motivo")
	}
}

func TestGenerateAuthorizationRejectedShowsReason(t *testing.T) {
	snap := authzSnapshot()
	item := authzItem(snap, documents.AuthorizationStatusRejected, documents.RejectionReasonMissingDocumentation)
	data, err := NewGenerator().GenerateAuthorization(snap, item)
	if err != nil {
		t.Fatalf("generar falló: %v", err)
	}
	for _, want := range []string{"Rechazada", "Motivo del rechazo", "Falta de documentaci"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("rechazada debería contener %q", want)
		}
	}
}
