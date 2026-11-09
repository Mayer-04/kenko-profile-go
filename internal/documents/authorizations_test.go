package documents

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func validAuthzAppointment() CompletedAppointment {
	uid := uuid.Must(uuid.NewV7()).String()
	return CompletedAppointment{
		ID:        uuid.Must(uuid.NewV7()).String(),
		UserID:    uid,
		Type:      AppointmentTypeMedicinaGeneral,
		Date:      "2026-05-10",
		StartTime: "08:00",
		EndTime:   "08:30",
		Physician: Physician{PhysicianID: "MED-1", Name: "Dra. Laura Cifuentes"},
		MedicalCenter: MedicalCenter{
			MedicalCenterID: "SEDE-1",
			Name:            "Sede Norte",
			Location:        "Bogota",
		},
		Status:      AppointmentStatusCompleted,
		Notes:       "Control general.",
		CompletedAt: time.Date(2026, 5, 10, 13, 30, 0, 0, time.UTC),
	}
}

func validAuthz(appt CompletedAppointment) Authorization {
	created := appt.CompletedAt.UTC()
	return Authorization{
		ID:            uuid.Must(uuid.NewV7()).String(),
		UserID:        appt.UserID,
		AppointmentID: appt.ID,
		Type:          AuthorizationTypeBloodTest,
		Status:        AuthorizationStatusInProgress,
		CreatedAt:     created,
		UpdatedAt:     created,
	}
}

func TestAuthorizationLabels(t *testing.T) {
	if AuthorizationTypeBloodTest.Label() != "Examen de sangre" ||
		AuthorizationTypeXRay.Label() != "Radiografía" ||
		AuthorizationTypeDentalCleaning.Label() != "Limpieza dental" {
		t.Fatal("etiquetas de tipo incorrectas")
	}
	if AuthorizationStatusInProgress.Label() != "En proceso" ||
		AuthorizationStatusRejected.Label() != "Rechazada" ||
		AuthorizationStatusApproved.Label() != "Aprobada" {
		t.Fatal("etiquetas de estado incorrectas")
	}
	if RejectionReasonMissingDocumentation.Label() != "Falta de documentación" ||
		RejectionReasonNotCurrentlyAffiliated.Label() != "La persona no está afiliada actualmente" ||
		RejectionReasonCreatedInError.Label() != "Autorización creada por error" {
		t.Fatal("etiquetas de motivo incorrectas")
	}
	if AuthorizationType("X").Label() != "" || AuthorizationStatus("X").Label() != "" || RejectionReason("X").Label() != "" {
		t.Fatal("desconocidos deberían dar cadena vacía")
	}
}

func TestAuthorizationTypeSlug(t *testing.T) {
	cases := map[AuthorizationType]string{
		AuthorizationTypeBloodTest:      "examen-de-sangre",
		AuthorizationTypeXRay:           "radiografia",
		AuthorizationTypeDentalCleaning: "limpieza-dental",
		AuthorizationType("OTRO"):       "autorizacion",
	}
	for typ, want := range cases {
		if got := typ.Slug(); got != want {
			t.Fatalf("slug %q = %q, quiero %q", typ, got, want)
		}
	}
}

func TestAuthorizationCompatibility(t *testing.T) {
	cases := []struct {
		appt  AppointmentType
		auth  AuthorizationType
		valid bool
	}{
		{AppointmentTypeMedicinaGeneral, AuthorizationTypeBloodTest, true},
		{AppointmentTypeMedicinaGeneral, AuthorizationTypeXRay, true},
		{AppointmentTypeMedicinaGeneral, AuthorizationTypeDentalCleaning, false},
		{AppointmentTypeOdontologia, AuthorizationTypeDentalCleaning, true},
		{AppointmentTypeOdontologia, AuthorizationTypeBloodTest, false},
		{AppointmentTypeOdontologia, AuthorizationTypeXRay, false},
		{AppointmentTypePsicologia, AuthorizationTypeBloodTest, false},
		{AppointmentTypePsicologia, AuthorizationTypeXRay, false},
		{AppointmentTypePsicologia, AuthorizationTypeDentalCleaning, false},
	}
	for _, tc := range cases {
		if got := CompatibleAuthorization(tc.appt, tc.auth); got != tc.valid {
			t.Fatalf("compatible(%s,%s)=%v, quiero %v", tc.appt, tc.auth, got, tc.valid)
		}
	}
}

func TestAuthorizationValidateFields(t *testing.T) {
	appt := validAuthzAppointment()
	mk := func() Authorization { return validAuthz(appt) }
	cases := map[string]func(*Authorization){
		"id vacío":            func(a *Authorization) { a.ID = " " },
		"id no UUID":          func(a *Authorization) { a.ID = "no-uuid" },
		"user vacío":          func(a *Authorization) { a.UserID = "" },
		"user no UUID":        func(a *Authorization) { a.UserID = "x" },
		"appointment vacío":   func(a *Authorization) { a.AppointmentID = "" },
		"appointment no UUID": func(a *Authorization) { a.AppointmentID = "y" },
		"tipo vacío":          func(a *Authorization) { a.Type = "" },
		"tipo inventado":      func(a *Authorization) { a.Type = "COLONOSCOPY" },
		"status vacío":        func(a *Authorization) { a.Status = "" },
		"status inventado":    func(a *Authorization) { a.Status = "COMPLETED" },
		"createdAt zero":      func(a *Authorization) { a.CreatedAt = time.Time{} },
		"updatedAt zero":      func(a *Authorization) { a.UpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := mk()
			mutate(&a)
			if err := a.Validate(); err == nil {
				t.Fatalf("debió fallar: %s", name)
			}
			if err := a.ValidateForAppointment(appt); err == nil {
				t.Fatalf("debió fallar con cita: %s", name)
			}
		})
	}
}

func TestAuthorizationValidateRules(t *testing.T) {
	appt := validAuthzAppointment()
	base := validAuthz(appt)
	if err := base.ValidateForAppointment(appt); err != nil {
		t.Fatalf("válida rechazada: %v", err)
	}

	// Motivo obligatorio si REJECTED.
	rejected := base
	rejected.Status = AuthorizationStatusRejected
	if err := rejected.Validate(); err == nil {
		t.Fatal("rechazada sin motivo debería fallar")
	}
	rejected.RejectionReason = RejectionReasonMissingDocumentation
	if err := rejected.ValidateForAppointment(appt); err != nil {
		t.Fatalf("rechazada con motivo rechazada: %v", err)
	}

	// Motivo prohibido en otros estados.
	withReason := base
	withReason.RejectionReason = RejectionReasonMissingDocumentation
	if err := withReason.Validate(); err == nil {
		t.Fatal("motivo en IN_PROGRESS debería fallar")
	}

	// updatedAt no anterior a createdAt.
	backwards := base
	backwards.UpdatedAt = backwards.CreatedAt.Add(-time.Hour)
	if err := backwards.Validate(); err == nil {
		t.Fatal("updatedAt anterior debería fallar")
	}

	// userId debe coincidir con la cita.
	other := base
	other.UserID = uuid.Must(uuid.NewV7()).String()
	if err := other.ValidateForAppointment(appt); err == nil {
		t.Fatal("userId distinto debería fallar")
	}

	// appointmentId debe coincidir con la cita.
	mismatch := base
	mismatch.AppointmentID = uuid.Must(uuid.NewV7()).String()
	if err := mismatch.ValidateForAppointment(appt); err == nil {
		t.Fatal("appointmentId distinto debería fallar")
	}

	// Tipo incompatible.
	incomp := base
	apptOdonto := appt
	apptOdonto.Type = AppointmentTypeOdontologia
	apptOdonto.ID = uuid.Must(uuid.NewV7()).String()
	incomp.AppointmentID = apptOdonto.ID
	if err := incomp.ValidateForAppointment(apptOdonto); err == nil {
		t.Fatal("BLOOD_TEST en ODONTOLOGIA debería fallar")
	}

	// Cita inexistente.
	empty := base
	emptyAppt := CompletedAppointment{}
	if err := empty.ValidateForAppointment(emptyAppt); err == nil {
		t.Fatal("cita vacía debería fallar")
	}
}
