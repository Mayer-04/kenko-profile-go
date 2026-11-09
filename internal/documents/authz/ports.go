package authz

import (
	"context"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
)

// AuthorizationWithAppointment es una autorización junto con su cita.
type AuthorizationWithAppointment struct {
	Authorization documents.Authorization
	Appointment   documents.CompletedAppointment
}

// AuthorizationReader lista y obtiene autorizaciones del usuario con su cita.
type AuthorizationReader interface {
	ListByUser(ctx context.Context, userID string) ([]AuthorizationWithAppointment, error)
	GetByID(ctx context.Context, userID, authorizationID string) (AuthorizationWithAppointment, error)
}

// PDFGenerator genera el PDF de la autorización en memoria.
type PDFGenerator interface {
	GenerateAuthorization(snapshot documents.ProfileSnapshot, item AuthorizationWithAppointment) ([]byte, error)
}

// NotifyEvent es el detail de AuthorizationStatusChanged.
type NotifyEvent struct {
	Metadata documents.EventMetadata `json:"metadata"`
	Data     NotifyData              `json:"data"`
}

// NotifyData del evento de autorización.
type NotifyData struct {
	Email             string              `json:"email"`
	FirstName         string              `json:"firstName"`
	AuthorizationType string              `json:"authorizationType"`
	Status            string              `json:"status"`
	ChangedAt         string              `json:"changedAt"`
	File              documents.EventFile `json:"file"`
}

// NotifyEventPublisher publica AuthorizationStatusChanged.
type NotifyEventPublisher interface {
	PublishAuthorizationStatusChanged(ctx context.Context, evt NotifyEvent) error
}

// FileKeySaver guarda la clave S3 del PDF en la autorización.
type FileKeySaver interface {
	SetAuthorizationFileKey(ctx context.Context, userID, authorizationID, fileKey string) error
}
