package documents

import (
	"context"
	"time"
)

// ProfileSnapshot son los datos mínimos del titular para el certificado.
// Viene del adaptador que lee la colección profiles (sin importar ese paquete).
type ProfileSnapshot struct {
	UserID               string
	FirstName            string
	LastName             string
	DocumentType         string
	DocumentNumber       string
	AffiliationStartDate time.Time
}

// GenerateCommand es la entrada del caso de uso (datos del JWT + correlación).
type GenerateCommand struct {
	UserID        string
	FirstName     string
	Email         string
	CorrelationID string
}

// GenerateResult es la respuesta del caso de uso.
type GenerateResult struct {
	CertificateID        string
	IssuedAt             time.Time
	AffiliationStartDate time.Time
}

// CertificateEvent es el detail de CertificateGenerated.
type CertificateEvent struct {
	Metadata EventMetadata `json:"metadata"`
	Data     EventData     `json:"data"`
}

// EventMetadata del sobre estándar.
type EventMetadata struct {
	EventID       string `json:"eventId"`
	EventVersion  string `json:"eventVersion"`
	OccurredAt    string `json:"occurredAt"`
	CorrelationID string `json:"correlationId"`
}

// EventData del certificado.
type EventData struct {
	CertificateID string    `json:"certificateId"`
	UserID        string    `json:"userId"`
	Email         string    `json:"email"`
	FirstName     string    `json:"firstName"`
	IssuedAt      string    `json:"issuedAt"`
	File          EventFile `json:"file"`
}

// EventFile apunta al PDF en S3.
type EventFile struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
	FileName    string `json:"fileName"`
}

// Interfaces pequeñas, del lado de quien las usa.

// ProfileReader lee el perfil del titular.
type ProfileReader interface {
	GetProfile(ctx context.Context, userID string) (ProfileSnapshot, error)
}

// CertificateRepository guarda el registro del certificado.
type CertificateRepository interface {
	Save(ctx context.Context, cert Certificate) error
}

// PDFGenerator genera el PDF en memoria.
type PDFGenerator interface {
	Generate(snapshot ProfileSnapshot, cert Certificate) ([]byte, error)
}

// FileStorage sube el PDF a S3.
type FileStorage interface {
	Upload(ctx context.Context, key string, body []byte, contentType string) error
}

// EventPublisher publica CertificateGenerated.
type EventPublisher interface {
	PublishCertificateGenerated(ctx context.Context, evt CertificateEvent) error
}

// Clock abstrae el tiempo para tests deterministas.
type Clock interface {
	Now() time.Time
}

// IDGenerator abstrae los ids para tests deterministas.
type IDGenerator interface {
	NewID() string
}
