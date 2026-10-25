package profile

import (
	"fmt"
	"slices"
	"strings"
	"time"

	json "encoding/json/v2"

	"uuid"
)

// Sources de EventBridge de este ecosistema (sección 2).
const (
	SourceAuthentication = "kenko.auth"
	SourceProfile        = "kenko.profile"
)

// Detail-types de los eventos que este servicio consume y publica.
const (
	TypeUserRegistered   = "UserRegistered"
	TypeUserEmailChanged = "UserEmailChanged"
	TypeProfileCreated   = "ProfileCreated"
)

// eventVersion es la única versión de contrato soportada (sección 7.1).
const eventVersion = "1"

// validGenders son los géneros aceptados en Person.Gender.
var validGenders = []string{"male", "female", "other"}

// EventMetadata es la metadata común a todos los eventos de Kenko.
type EventMetadata struct {
	EventID       string    `json:"eventId"`
	EventVersion  string    `json:"eventVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	CorrelationID string    `json:"correlationId"`
}

// EventUUID devuelve el identificador del evento ya parseado.
func (m EventMetadata) EventUUID() (uuid.UUID, error) {
	return uuid.Parse(m.EventID)
}

// UserRegistered es el evento que publica Authentication cuando un usuario se
// registra (sección 7.1).
type UserRegistered struct {
	Metadata EventMetadata    `json:"metadata"`
	Data     RegistrationData `json:"data"`
}

// UserID devuelve el identificador del usuario ya parseado.
func (e UserRegistered) UserID() (uuid.UUID, error) {
	return uuid.Parse(e.Data.UserID)
}

// RegistrationData es la parte de datos del evento; ProfileCreated usa esta
// misma estructura (sección 7.2).
type RegistrationData struct {
	UserID   string      `json:"userId"`
	Document DocIdentity `json:"document"`
	Person   Person      `json:"person"`
	Contact  Contact     `json:"contact"`
	Address  AddressData `json:"address"`
}

// DocIdentity es el documento de identidad del usuario.
type DocIdentity struct {
	Type   string `json:"type"`
	Number string `json:"number"`
}

// Person son los datos personales inmutables del perfil.
type Person struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	BirthDate string `json:"birthDate"`
	Gender    string `json:"gender"`
}

// Contact son los datos de contacto.
type Contact struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	Phone         string `json:"phone"`
}

// AddressData es la dirección del usuario.
type AddressData struct {
	Line string `json:"line"`
	City string `json:"city"`
}

// ParseUserRegistered valida y parsea el detail de un UserRegistered.
func ParseUserRegistered(detail []byte) (UserRegistered, error) {
	var event UserRegistered
	if err := json.Unmarshal(detail, &event); err != nil {
		return UserRegistered{}, fmt.Errorf("%w: JSON mal formado: %v", ErrInvalidEvent, err)
	}
	if err := validateUserRegistered(event); err != nil {
		return UserRegistered{}, err
	}
	return event, nil
}

// UserEmailChanged es el evento que publica Authentication al cambiar el
// email del usuario (D8-B). Mantiene la copia del email de Profile al día.
type UserEmailChanged struct {
	Metadata EventMetadata   `json:"metadata"`
	Data     EmailChangeData `json:"data"`
}

// EmailChangeData son los datos del cambio de email.
type EmailChangeData struct {
	UserID        string `json:"userId"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
}

// UserID devuelve el identificador del usuario ya parseado.
func (e UserEmailChanged) UserID() (uuid.UUID, error) {
	return uuid.Parse(e.Data.UserID)
}

// ParseUserEmailChanged valida y parsea el detail de un UserEmailChanged.
func ParseUserEmailChanged(detail []byte) (UserEmailChanged, error) {
	var event UserEmailChanged
	if err := json.Unmarshal(detail, &event); err != nil {
		return UserEmailChanged{}, fmt.Errorf("%w: JSON mal formado: %v", ErrInvalidEvent, err)
	}
	if err := validateUserEmailChanged(event); err != nil {
		return UserEmailChanged{}, err
	}
	return event, nil
}

// ProfileCreated es el evento que este servicio publica hacia Appointments y
// Documents. Su data replica la estructura del UserRegistered (sección 7.2).
type ProfileCreated struct {
	Metadata EventMetadata    `json:"metadata"`
	Data     RegistrationData `json:"data"`
}

// BuildProfileCreated construye el detail de ProfileCreated a partir del
// perfil guardado. El eventId es nuevo (nunca el del UserRegistered), el
// occurredAt es el instante de creación del perfil y el correlationId es el
// del evento que originó el flujo (sección 7.2).
func BuildProfileCreated(p Profile, eventID uuid.UUID, occurredAt time.Time, correlationID string) ProfileCreated {
	return ProfileCreated{
		Metadata: EventMetadata{
			EventID:       eventID.String(),
			EventVersion:  eventVersion,
			OccurredAt:    occurredAt,
			CorrelationID: correlationID,
		},
		Data: RegistrationData{
			UserID: p.ID.String(),
			Document: DocIdentity{
				Type:   p.DocumentType,
				Number: p.DocumentNumber,
			},
			Person: Person{
				FirstName: p.FirstName,
				LastName:  p.LastName,
				BirthDate: p.BirthDate.Format(birthDateLayout),
				Gender:    p.Gender,
			},
			Contact: Contact{
				Email:         p.Email,
				EmailVerified: p.EmailVerified,
				Phone:         p.Phone,
			},
			Address: AddressData{
				Line: p.AddressLine,
				City: p.City,
			},
		},
	}
}

func validateUserRegistered(event UserRegistered) error {
	if err := validateMetadata(event.Metadata); err != nil {
		return err
	}

	d := event.Data
	if _, err := uuid.Parse(d.UserID); err != nil {
		return fmt.Errorf("%w: data.userId no es un UUID", ErrInvalidEvent)
	}
	if err := requireFields(map[string]string{
		"data.document.type":    d.Document.Type,
		"data.document.number":  d.Document.Number,
		"data.person.firstName": d.Person.FirstName,
		"data.person.lastName":  d.Person.LastName,
		"data.address.line":     d.Address.Line,
		"data.address.city":     d.Address.City,
	}); err != nil {
		return err
	}

	birthDate, err := time.Parse(birthDateLayout, d.Person.BirthDate)
	if err != nil {
		return fmt.Errorf("%w: data.person.birthDate debe tener formato YYYY-MM-DD", ErrInvalidEvent)
	}
	if !birthDate.Before(time.Now()) {
		return fmt.Errorf("%w: data.person.birthDate debe estar en el pasado", ErrInvalidEvent)
	}
	if !slices.Contains(validGenders, d.Person.Gender) {
		return fmt.Errorf("%w: data.person.gender debe ser male, female u other", ErrInvalidEvent)
	}
	if !emailPattern.MatchString(d.Contact.Email) {
		return fmt.Errorf("%w: data.contact.email no es válido", ErrInvalidEvent)
	}
	if !phonePattern.MatchString(d.Contact.Phone) {
		return fmt.Errorf("%w: data.contact.phone debe estar en formato E.164", ErrInvalidEvent)
	}
	return nil
}

func validateUserEmailChanged(event UserEmailChanged) error {
	if err := validateMetadata(event.Metadata); err != nil {
		return err
	}
	if _, err := uuid.Parse(event.Data.UserID); err != nil {
		return fmt.Errorf("%w: data.userId no es un UUID", ErrInvalidEvent)
	}
	if !emailPattern.MatchString(event.Data.Email) {
		return fmt.Errorf("%w: data.email no es válido", ErrInvalidEvent)
	}
	return nil
}

func validateMetadata(m EventMetadata) error {
	if _, err := uuid.Parse(m.EventID); err != nil {
		return fmt.Errorf("%w: metadata.eventId no es un UUID", ErrInvalidEvent)
	}
	if m.EventVersion != eventVersion {
		return fmt.Errorf("%w: metadata.eventVersion debe ser %q", ErrInvalidEvent, eventVersion)
	}
	if m.OccurredAt.IsZero() {
		return fmt.Errorf("%w: metadata.occurredAt es obligatorio", ErrInvalidEvent)
	}
	if _, err := uuid.Parse(m.CorrelationID); err != nil {
		return fmt.Errorf("%w: metadata.correlationId no es un UUID", ErrInvalidEvent)
	}
	return nil
}

// requireFields comprueba que ningún campo obligatorio esté vacío.
func requireFields(fields map[string]string) error {
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: %s es obligatorio", ErrInvalidEvent, name)
		}
	}
	return nil
}
