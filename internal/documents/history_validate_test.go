package documents

import (
	"testing"
	"time"
)

func validHistoryAppointment() CompletedAppointment {
	return CompletedAppointment{
		ID:        "a1",
		UserID:    "u1",
		Type:      AppointmentTypeOdontologia,
		Date:      "2026-05-10",
		StartTime: "09:30",
		EndTime:   "10:15",
		Physician: Physician{PhysicianID: "MED-1", Name: "Dra. X"},
		MedicalCenter: MedicalCenter{
			MedicalCenterID: "SEDE-1",
			Name:            "Sede Norte",
			Location:        "Bogota",
		},
		Status:      AppointmentStatusCompleted,
		Notes:       "Limpieza dental.",
		CompletedAt: time.Now().UTC(),
	}
}

func TestCompletedAppointmentValidateBranches(t *testing.T) {
	cases := map[string]func(*CompletedAppointment){
		"id vacío":        func(a *CompletedAppointment) { a.ID = " " },
		"user vacío":      func(a *CompletedAppointment) { a.UserID = "" },
		"start inválido":  func(a *CompletedAppointment) { a.StartTime = "25:00" },
		"end inválido":    func(a *CompletedAppointment) { a.EndTime = "x" },
		"médico vacío":    func(a *CompletedAppointment) { a.Physician.Name = " " },
		"sede vacía":      func(a *CompletedAppointment) { a.MedicalCenter.Name = "" },
		"notes vacías":    func(a *CompletedAppointment) { a.Notes = "" },
		"completed zero":  func(a *CompletedAppointment) { a.CompletedAt = time.Time{} },
		"tipo vacío":      func(a *CompletedAppointment) { a.Type = "" },
		"tipo inventado":  func(a *CompletedAppointment) { a.Type = "CARDIOLOGIA" },
		"status distinto": func(a *CompletedAppointment) { a.Status = "PENDING" },
		"fecha distinta":  func(a *CompletedAppointment) { a.Date = "2026/05/10" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := validHistoryAppointment()
			mutate(&a)
			if err := a.Validate(); err == nil {
				t.Fatalf("debió fallar: %s", name)
			}
		})
	}
}

func TestMedicalHistoryExportValidate(t *testing.T) {
	valid := MedicalHistoryExport{
		ID: "p1", UserID: "u1", RecordCount: 6,
		FileKey: "medical-history/u1/p1.pdf", GeneratedAt: time.Now().UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("válido rechazado: %v", err)
	}
	for name, mutate := range map[string]func(*MedicalHistoryExport){
		"id vacío":      func(e *MedicalHistoryExport) { e.ID = "" },
		"user vacío":    func(e *MedicalHistoryExport) { e.UserID = " " },
		"record neg":    func(e *MedicalHistoryExport) { e.RecordCount = -1 },
		"filekey vacío": func(e *MedicalHistoryExport) { e.FileKey = "" },
		"generated zero": func(e *MedicalHistoryExport) {
			e.GeneratedAt = time.Time{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := valid
			mutate(&e)
			if err := e.Validate(); err == nil {
				t.Fatalf("debió fallar: %s", name)
			}
		})
	}
}
