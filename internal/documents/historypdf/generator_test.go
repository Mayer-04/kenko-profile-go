package historypdf

import (
	"strings"
	"testing"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/gofrs/uuid/v5"
)

func TestGenerateHistoryStartsWithPDF(t *testing.T) {
	uid, _ := uuid.NewV7()
	pdfID, _ := uuid.NewV7()
	snap := documents.ProfileSnapshot{
		UserID:               uid.String(),
		FirstName:            "María",
		LastName:             "Gómez Rojas",
		DocumentType:         "CC",
		DocumentNumber:       "1020304050",
		AffiliationStartDate: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
	appts := []documents.CompletedAppointment{
		{
			ID:        uuid.Must(uuid.NewV7()).String(),
			UserID:    uid.String(),
			Type:      documents.AppointmentTypeMedicinaGeneral,
			Date:      "2026-05-10",
			StartTime: "08:00",
			EndTime:   "08:30",
			Physician: documents.Physician{PhysicianID: "MED-1", Name: "Dra. Laura Cifuentes"},
			MedicalCenter: documents.MedicalCenter{
				MedicalCenterID: "SEDE-1",
				Name:            "Sede Norte",
				Location:        "Calle 127 # 45-10, Bogota",
			},
			Status:      documents.AppointmentStatusCompleted,
			Notes:       "Control general sin hallazgos.",
			CompletedAt: time.Date(2026, 5, 10, 13, 30, 0, 0, time.UTC),
		},
	}
	exp := documents.MedicalHistoryExport{
		ID:          pdfID.String(),
		UserID:      uid.String(),
		RecordCount: len(appts),
		FileKey:     "medical-history/" + uid.String() + "/" + pdfID.String() + ".pdf",
		GeneratedAt: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
	}
	data, err := NewGenerator().Generate(snap, appts, exp)
	if err != nil {
		t.Fatalf("generar falló: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatal("no empieza con %PDF")
	}
	for _, want := range []string{"1020304050", "Medicina general", "10/05/2026", "Laura Cifuentes"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("pdf debería contener %q", want)
		}
	}
}
