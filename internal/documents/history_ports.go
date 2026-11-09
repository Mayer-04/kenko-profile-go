package documents

import (
	"context"
	"time"
)

// HistoryCommand es la entrada del caso de uso (datos del JWT + correlación).
type HistoryCommand struct {
	UserID        string
	FirstName     string
	Email         string
	CorrelationID string
}

// HistoryResult es la respuesta del caso de uso.
type HistoryResult struct {
	PdfID       string
	GeneratedAt time.Time
	RecordCount int
}

// HistoryEvent es el detail de MedicalHistoryPdfGenerated.
type HistoryEvent struct {
	Metadata EventMetadata `json:"metadata"`
	Data     HistoryData   `json:"data"`
}

// HistoryData del historial.
type HistoryData struct {
	PdfID       string    `json:"pdfId"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	FirstName   string    `json:"firstName"`
	GeneratedAt string    `json:"generatedAt"`
	RecordCount int       `json:"recordCount"`
	File        EventFile `json:"file"`
}

// HistoryAppointmentReader lista las citas atendidas del titular.
type HistoryAppointmentReader interface {
	ListAppointments(ctx context.Context, userID string) ([]CompletedAppointment, error)
}

// HistoryRepository guarda el registro de la exportación.
type HistoryRepository interface {
	SaveHistoryExport(ctx context.Context, exp MedicalHistoryExport) error
}

// HistoryPDFGenerator genera el PDF del historial en memoria.
type HistoryPDFGenerator interface {
	Generate(snapshot ProfileSnapshot, appointments []CompletedAppointment, exp MedicalHistoryExport) ([]byte, error)
}

// HistoryEventPublisher publica MedicalHistoryPdfGenerated.
type HistoryEventPublisher interface {
	PublishMedicalHistoryGenerated(ctx context.Context, evt HistoryEvent) error
}
