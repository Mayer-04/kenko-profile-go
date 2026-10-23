// Package events define los contratos de eventos de 7.1/7.2 y la
// validación de entrada del worker (5.3), incluida eventVersion.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// Envelope es el evento tal como lo entrega EventBridge al worker
// (minúsculas: source, detail-type, detail).
type Envelope struct {
	Source     string          `json:"source" validate:"required"`
	DetailType string          `json:"detail-type" validate:"required"`
	Detail     json.RawMessage `json:"detail" validate:"required"`
}

// Metadata del detail (7.1).
type Metadata struct {
	EventID       string `json:"eventId" validate:"required,uuid"`
	EventVersion  string `json:"eventVersion" validate:"required,eq=1"`
	OccurredAt    string `json:"occurredAt" validate:"required"`
	CorrelationID string `json:"correlationId" validate:"required,uuid"`
}

// UserData es el data de UserRegistered / ProfileCreated (7.1).
type UserData struct {
	UserID   string `json:"userId" validate:"required,uuid"`
	Document struct {
		Type   string `json:"type" validate:"required"`
		Number string `json:"number" validate:"required"`
	} `json:"document" validate:"required"`
	Person struct {
		FirstName string `json:"firstName" validate:"required"`
		LastName  string `json:"lastName" validate:"required"`
		BirthDate string `json:"birthDate" validate:"required"`
		Gender    string `json:"gender" validate:"required"`
	} `json:"person" validate:"required"`
	Contact struct {
		Email         string `json:"email" validate:"required,email"`
		EmailVerified bool   `json:"emailVerified"`
		Phone         string `json:"phone" validate:"required"`
	} `json:"contact" validate:"required"`
	Address struct {
		Line string `json:"line" validate:"required"`
		City string `json:"city" validate:"required"`
	} `json:"address" validate:"required"`
}

// UserRegistered es el detail consumido (7.1).
type UserRegistered struct {
	Metadata Metadata `json:"metadata" validate:"required"`
	Data     UserData `json:"data" validate:"required"`
}

// EmailChangedData es el data de UserEmailChanged (D8-B).
type EmailChangedData struct {
	UserID        string `json:"userId" validate:"required,uuid"`
	Email         string `json:"email" validate:"required,email"`
	EmailVerified bool   `json:"emailVerified"`
	OccurredAt    string `json:"occurredAt" validate:"required"`
}

// UserEmailChanged es el evento de Authentication al cambiar el email.
type UserEmailChanged struct {
	Metadata Metadata         `json:"metadata" validate:"required"`
	Data     EmailChangedData `json:"data" validate:"required"`
}

// ParseEnvelope valida el sobre y devuelve el tipo y el detail crudo.
func ParseEnvelope(body []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Envelope{}, fmt.Errorf("envelope: %w", err)
	}
	if err := validate.Struct(env); err != nil {
		return Envelope{}, fmt.Errorf("envelope: %w", err)
	}
	if env.Source != "kenko.auth" {
		return Envelope{}, fmt.Errorf("envelope: source %q no esperado", env.Source)
	}
	return env, nil
}

// ParseUserRegistered valida el detail contra el contrato 7.1.
func ParseUserRegistered(detail json.RawMessage) (UserRegistered, error) {
	var evt UserRegistered
	if err := json.Unmarshal(detail, &evt); err != nil {
		return UserRegistered{}, fmt.Errorf("userRegistered: %w", err)
	}
	if err := validate.Struct(evt); err != nil {
		return UserRegistered{}, fmt.Errorf("userRegistered: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, evt.Metadata.OccurredAt); err != nil {
		return UserRegistered{}, fmt.Errorf("userRegistered: occurredAt inválido: %w", err)
	}
	if _, err := time.Parse("2006-01-02", evt.Data.Person.BirthDate); err != nil {
		return UserRegistered{}, fmt.Errorf("userRegistered: birthDate inválida: %w", err)
	}
	return evt, nil
}

// ParseUserEmailChanged valida el evento de cambio de email (D8-B).
func ParseUserEmailChanged(detail json.RawMessage) (UserEmailChanged, error) {
	var evt UserEmailChanged
	if err := json.Unmarshal(detail, &evt); err != nil {
		return UserEmailChanged{}, fmt.Errorf("userEmailChanged: %w", err)
	}
	if err := validate.Struct(evt); err != nil {
		return UserEmailChanged{}, fmt.Errorf("userEmailChanged: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, evt.Metadata.OccurredAt); err != nil {
		return UserEmailChanged{}, fmt.Errorf("userEmailChanged: occurredAt inválido: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, evt.Data.OccurredAt); err != nil {
		return UserEmailChanged{}, fmt.Errorf("userEmailChanged: data.occurredAt inválido: %w", err)
	}
	return evt, nil
}
