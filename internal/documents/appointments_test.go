package documents

import (
	"testing"
	"time"
)

func TestAppointmentTypeSpanishLabel(t *testing.T) {
	cases := map[AppointmentType]string{
		AppointmentTypeMedicinaGeneral: "Medicina general",
		AppointmentTypeOdontologia:     "Odontología",
		AppointmentTypePsicologia:      "Psicología",
		AppointmentType("OTRO"):        "",
	}
	for typ, want := range cases {
		if got := typ.SpanishLabel(); got != want {
			t.Fatalf("etiqueta %q = %q, quiero %q", typ, got, want)
		}
	}
}

func TestCompletedAppointmentValidate(t *testing.T) {
	valid := CompletedAppointment{
		ID:        "a1",
		UserID:    "u1",
		Type:      AppointmentTypeMedicinaGeneral,
		Date:      "2026-05-10",
		StartTime: "08:00",
		EndTime:   "08:30",
		Physician: Physician{PhysicianID: "MED-1", Name: "Dra. X"},
		MedicalCenter: MedicalCenter{
			MedicalCenterID: "SEDE-1",
			Name:            "Sede Norte",
			Location:        "Bogota",
		},
		Status:      AppointmentStatusCompleted,
		Notes:       "Control.",
		CompletedAt: time.Now().UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("válida rechazada: %v", err)
	}
	bad := valid
	bad.Type = AppointmentType("INVENTADO")
	if err := bad.Validate(); err == nil {
		t.Fatal("tipo inválido debería fallar")
	}
	bad = valid
	bad.Status = "PENDING"
	if err := bad.Validate(); err == nil {
		t.Fatal("status inválido debería fallar")
	}
	bad = valid
	bad.Date = "10/05/2026"
	if err := bad.Validate(); err == nil {
		t.Fatal("fecha no YYYY-MM-DD debería fallar")
	}
	if !validValue("a", []string{"a", "b"}) || validValue("z", []string{"a", "b"}) {
		t.Fatal("validValue incorrecto")
	}
}
