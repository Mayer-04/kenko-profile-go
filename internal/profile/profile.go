package profile

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"uuid"
)

// Profile es el perfil del usuario de Kenko. La correspondencia con el evento
// UserRegistered y con el documento profiles está en la sección 7.1.
type Profile struct {
	ID             uuid.UUID
	DocumentType   string
	DocumentNumber string
	FirstName      string
	LastName       string
	BirthDate      time.Time
	Gender         string
	Email          string
	EmailVerified  bool
	Phone          string
	AddressLine    string
	City           string
	// EmailUpdatedAt es el instante del último cambio de email aplicado; sirve
	// para ignorar eventos UserEmailChanged más antiguos (D8-B).
	EmailUpdatedAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewFromRegistration construye el perfil a partir de un UserRegistered ya
// validado. El _id es el userId de Authentication (confirmado por el usuario).
func NewFromRegistration(event UserRegistered, now time.Time) (Profile, error) {
	userID, err := event.UserID()
	if err != nil {
		return Profile{}, fmt.Errorf("%w: data.userId no es un UUID", ErrInvalidEvent)
	}
	birthDate, err := time.Parse(birthDateLayout, event.Data.Person.BirthDate)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: data.person.birthDate debe ser YYYY-MM-DD", ErrInvalidEvent)
	}

	return Profile{
		ID:             userID,
		DocumentType:   event.Data.Document.Type,
		DocumentNumber: event.Data.Document.Number,
		FirstName:      event.Data.Person.FirstName,
		LastName:       event.Data.Person.LastName,
		BirthDate:      birthDate,
		Gender:         event.Data.Person.Gender,
		Email:          event.Data.Contact.Email,
		EmailVerified:  event.Data.Contact.EmailVerified,
		Phone:          event.Data.Contact.Phone,
		AddressLine:    event.Data.Address.Line,
		City:           event.Data.Address.City,
		EmailUpdatedAt: event.Metadata.OccurredAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Address es la dirección del usuario. Se modifica como objeto completo.
type Address struct {
	Line string
	City string
}

// Patch son los únicos campos modificables por HTTP (sección 4.2): phone y el
// objeto address completo. El email y la contraseña se cambian en
// Authentication; documento, nombres, fecha de nacimiento y género son
// inmutables.
type Patch struct {
	Phone   *string
	Address *Address
}

// IsEmpty indica si el parche no incluye ningún campo.
func (p Patch) IsEmpty() bool {
	return p.Phone == nil && p.Address == nil
}

// FieldNames devuelve los nombres de los campos incluidos, para registrarlos
// en los logs sin sus valores (sección 11.3).
func (p Patch) FieldNames() []string {
	names := []string{}
	if p.Phone != nil {
		names = append(names, "phone")
	}
	if p.Address != nil {
		names = append(names, "address")
	}
	return names
}

// Validate comprueba los valores del parche.
func (p Patch) Validate() error {
	if p.Phone != nil && !phonePattern.MatchString(*p.Phone) {
		return fmt.Errorf("%w: phone debe estar en formato E.164, por ejemplo +573001234567", ErrInvalidPatch)
	}
	if p.Address != nil {
		if strings.TrimSpace(p.Address.Line) == "" {
			return fmt.Errorf("%w: address.line es obligatorio", ErrInvalidPatch)
		}
		if strings.TrimSpace(p.Address.City) == "" {
			return fmt.Errorf("%w: address.city es obligatorio", ErrInvalidPatch)
		}
	}
	return nil
}

const birthDateLayout = "2006-01-02"

var (
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	phonePattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
)
