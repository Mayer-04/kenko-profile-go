package documents

import (
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

// AuthorizationType es el tipo de autorización médica.
type AuthorizationType string

// Tipos de autorización válidos.
const (
	AuthorizationTypeBloodTest      AuthorizationType = "BLOOD_TEST"
	AuthorizationTypeXRay           AuthorizationType = "XRAY"
	AuthorizationTypeDentalCleaning AuthorizationType = "DENTAL_CLEANING"
)

// AuthorizationTypes son los tres tipos válidos.
var AuthorizationTypes = []AuthorizationType{
	AuthorizationTypeBloodTest,
	AuthorizationTypeXRay,
	AuthorizationTypeDentalCleaning,
}

// Label devuelve el texto en español del tipo.
func (t AuthorizationType) Label() string {
	switch t {
	case AuthorizationTypeBloodTest:
		return "Examen de sangre"
	case AuthorizationTypeXRay:
		return "Radiografía"
	case AuthorizationTypeDentalCleaning:
		return "Limpieza dental"
	default:
		return ""
	}
}

// Slug devuelve el identificador URL/archivo en español y sin tildes.
func (t AuthorizationType) Slug() string {
	switch t {
	case AuthorizationTypeBloodTest:
		return "examen-de-sangre"
	case AuthorizationTypeXRay:
		return "radiografia"
	case AuthorizationTypeDentalCleaning:
		return "limpieza-dental"
	default:
		return "autorizacion"
	}
}

// AuthorizationStatus es el estado de la autorización.
type AuthorizationStatus string

// Estados válidos.
const (
	AuthorizationStatusInProgress AuthorizationStatus = "IN_PROGRESS"
	AuthorizationStatusRejected   AuthorizationStatus = "REJECTED"
	AuthorizationStatusApproved   AuthorizationStatus = "APPROVED"
)

// AuthorizationStatuses son los tres estados válidos.
var AuthorizationStatuses = []AuthorizationStatus{
	AuthorizationStatusInProgress,
	AuthorizationStatusRejected,
	AuthorizationStatusApproved,
}

// Label devuelve el texto en español del estado.
func (s AuthorizationStatus) Label() string {
	switch s {
	case AuthorizationStatusInProgress:
		return "En proceso"
	case AuthorizationStatusRejected:
		return "Rechazada"
	case AuthorizationStatusApproved:
		return "Aprobada"
	default:
		return ""
	}
}

// RejectionReason es el motivo de rechazo (solo si REJECTED).
type RejectionReason string

// Motivos válidos.
const (
	RejectionReasonMissingDocumentation   RejectionReason = "MISSING_DOCUMENTATION"
	RejectionReasonNotCurrentlyAffiliated RejectionReason = "NOT_CURRENTLY_AFFILIATED"
	RejectionReasonCreatedInError         RejectionReason = "CREATED_IN_ERROR"
)

// RejectionReasons son los tres motivos válidos.
var RejectionReasons = []RejectionReason{
	RejectionReasonMissingDocumentation,
	RejectionReasonNotCurrentlyAffiliated,
	RejectionReasonCreatedInError,
}

// Label devuelve el texto en español del motivo.
func (r RejectionReason) Label() string {
	switch r {
	case RejectionReasonMissingDocumentation:
		return "Falta de documentación"
	case RejectionReasonNotCurrentlyAffiliated:
		return "La persona no está afiliada actualmente"
	case RejectionReasonCreatedInError:
		return "Autorización creada por error"
	default:
		return ""
	}
}

// Errores propios del dominio de autorizaciones.
var (
	ErrAuthorizationNotFound = errors.New("authorization not found")
	ErrIncompatibleTypes     = errors.New("authorization type not compatible with appointment type")
)

// Authorization es una autorización ligada a una cita atendida.
// No guarda el médico: se toma de la cita (physician.name).
// FileKey se llena al notificar por correo (clave del PDF en S3).
type Authorization struct {
	ID              string              `bson:"_id"`
	UserID          string              `bson:"userId"`
	AppointmentID   string              `bson:"appointmentId"`
	Type            AuthorizationType   `bson:"type"`
	Status          AuthorizationStatus `bson:"status"`
	RejectionReason RejectionReason     `bson:"rejectionReason,omitempty"`
	FileKey         string              `bson:"fileKey,omitempty"`
	CreatedAt       time.Time           `bson:"createdAt"`
	UpdatedAt       time.Time           `bson:"updatedAt"`
}

// Validate verifica campos, listas cerradas y reglas propias
// (motivo solo en rechazadas, fechas coherentes).
func (a Authorization) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("authorization id obligatorio")
	}
	if _, err := uuid.FromString(strings.TrimSpace(a.ID)); err != nil {
		return errors.New("authorization id inválido")
	}
	if strings.TrimSpace(a.UserID) == "" {
		return errors.New("user id obligatorio")
	}
	if _, err := uuid.FromString(strings.TrimSpace(a.UserID)); err != nil {
		return errors.New("user id inválido")
	}
	if strings.TrimSpace(a.AppointmentID) == "" {
		return errors.New("appointment id obligatorio")
	}
	if _, err := uuid.FromString(strings.TrimSpace(a.AppointmentID)); err != nil {
		return errors.New("appointment id inválido")
	}
	if !validAuthorizationType(a.Type) {
		return errors.New("tipo de autorización inválido")
	}
	if !validAuthorizationStatus(a.Status) {
		return errors.New("status de autorización inválido")
	}
	if a.Status == AuthorizationStatusRejected {
		if !validRejectionReason(a.RejectionReason) {
			return errors.New("rejectionReason obligatorio y válido si REJECTED")
		}
	} else if strings.TrimSpace(string(a.RejectionReason)) != "" {
		return errors.New("rejectionReason solo existe si REJECTED")
	}
	if a.CreatedAt.IsZero() {
		return errors.New("createdAt obligatorio")
	}
	if a.UpdatedAt.IsZero() {
		return errors.New("updatedAt obligatorio")
	}
	if a.UpdatedAt.Before(a.CreatedAt) {
		return errors.New("updatedAt no puede ser anterior a createdAt")
	}
	return nil
}

// ValidateForAppointment verifica que la cita existe, es del mismo usuario
// y que el tipo de autorización es compatible con el tipo de cita.
func (a Authorization) ValidateForAppointment(appt CompletedAppointment) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(appt.ID) == "" {
		return errors.New("la cita debe existir")
	}
	if strings.TrimSpace(a.AppointmentID) != strings.TrimSpace(appt.ID) {
		return errors.New("appointmentId no coincide con la cita")
	}
	if strings.TrimSpace(a.UserID) != strings.TrimSpace(appt.UserID) {
		return errors.New("userId de la autorización debe ser el de su cita")
	}
	if !CompatibleAuthorization(appt.Type, a.Type) {
		return ErrIncompatibleTypes
	}
	return nil
}

// CompatibleAuthorization indica si el tipo de cita admite el tipo de autorización:
// MEDICINA_GENERAL admite BLOOD_TEST y XRAY; ODONTOLOGIA solo DENTAL_CLEANING;
// PSICOLOGIA no admite ninguna.
func CompatibleAuthorization(appointmentType AppointmentType, authorizationType AuthorizationType) bool {
	switch appointmentType {
	case AppointmentTypeMedicinaGeneral:
		return authorizationType == AuthorizationTypeBloodTest ||
			authorizationType == AuthorizationTypeXRay
	case AppointmentTypeOdontologia:
		return authorizationType == AuthorizationTypeDentalCleaning
	default:
		return false
	}
}

func validAuthorizationType(v AuthorizationType) bool {
	for _, a := range AuthorizationTypes {
		if v == a {
			return true
		}
	}
	return false
}

func validAuthorizationStatus(v AuthorizationStatus) bool {
	for _, a := range AuthorizationStatuses {
		if v == a {
			return true
		}
	}
	return false
}

func validRejectionReason(v RejectionReason) bool {
	for _, a := range RejectionReasons {
		if v == a {
			return true
		}
	}
	return false
}
