package documents

import (
	"errors"
	"strings"
	"time"
)

// Especialidades válidas para CompletedAppointment.Type (mayúsculas,
// sin tildes, como en los eventos de Appointments).
var AppointmentTypes = []string{
	"MEDICINA_GENERAL",
	"ODONTOLOGIA",
	"OPTOMETRIA",
	"DERMATOLOGIA",
	"CARDIOLOGIA",
}

// Estado único de la base ficticia: todas las citas están atendidas.
const AppointmentStatusCompleted = "COMPLETED"

// Physician es el médico ficticio que atendió la cita.
type Physician struct {
	PhysicianID string `bson:"physicianId"`
	Name        string `bson:"name"`
}

// MedicalCenter es la sede ficticia donde se atendió la cita.
type MedicalCenter struct {
	MedicalCenterID string `bson:"medicalCenterId"`
	Name            string `bson:"name"`
	Location        string `bson:"location"`
}

// CompletedAppointment es una cita atendida (base ficticia del historial
// médico en PDF). El día se guarda como texto "YYYY-MM-DD" para evitar
// desfases de zona horaria; CompletedAt es Date en UTC.
type CompletedAppointment struct {
	ID            string        `bson:"_id"`
	UserID        string        `bson:"userId"`
	Type          string        `bson:"type"`
	Date          string        `bson:"date"`
	StartTime     string        `bson:"startTime"`
	EndTime       string        `bson:"endTime"`
	Physician     Physician     `bson:"physician"`
	MedicalCenter MedicalCenter `bson:"medicalCenter"`
	Status        string        `bson:"status"`
	Notes         string        `bson:"notes"`
	CompletedAt   time.Time     `bson:"completedAt"`
}

// Validate verifica campos obligatorios y formatos de la cita.
func (a CompletedAppointment) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("appointment id obligatorio")
	}
	if strings.TrimSpace(a.UserID) == "" {
		return errors.New("user id obligatorio")
	}
	if !validValue(a.Type, AppointmentTypes) {
		return errors.New("tipo de cita inválido")
	}
	if _, err := time.Parse("2006-01-02", a.Date); err != nil {
		return errors.New("date debe ser YYYY-MM-DD")
	}
	if _, err := time.Parse("15:04", a.StartTime); err != nil {
		return errors.New("startTime debe ser HH:MM")
	}
	if _, err := time.Parse("15:04", a.EndTime); err != nil {
		return errors.New("endTime debe ser HH:MM")
	}
	if strings.TrimSpace(a.Physician.Name) == "" {
		return errors.New("nombre del médico obligatorio")
	}
	if strings.TrimSpace(a.MedicalCenter.Name) == "" {
		return errors.New("nombre de la sede obligatorio")
	}
	if a.Status != AppointmentStatusCompleted {
		return errors.New("status debe ser COMPLETED")
	}
	if strings.TrimSpace(a.Notes) == "" {
		return errors.New("notes obligatorias")
	}
	if a.CompletedAt.IsZero() {
		return errors.New("completedAt obligatorio")
	}
	return nil
}

// validValue revisa pertenencia a una lista cerrada.
func validValue(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
