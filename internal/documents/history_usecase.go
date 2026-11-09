package documents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// GenerateHistory es el caso de uso del historial médico.
// No conoce HTTP, Mongo ni AWS: solo interfaces.
type GenerateHistory struct {
	profiles     ProfileReader
	appointments HistoryAppointmentReader
	repo         HistoryRepository
	pdf          HistoryPDFGenerator
	storage      FileStorage
	publisher    HistoryEventPublisher
	clock        Clock
	ids          IDGenerator
	bucket       string
	log          *slog.Logger
}

// NewGenerateHistory inyecta todas las dependencias por constructor.
func NewGenerateHistory(
	profiles ProfileReader,
	appointments HistoryAppointmentReader,
	repo HistoryRepository,
	pdf HistoryPDFGenerator,
	storage FileStorage,
	publisher HistoryEventPublisher,
	clock Clock,
	ids IDGenerator,
	bucket string,
	log *slog.Logger,
) *GenerateHistory {
	return &GenerateHistory{
		profiles: profiles, appointments: appointments, repo: repo, pdf: pdf,
		storage: storage, publisher: publisher,
		clock: clock, ids: ids, bucket: bucket, log: log,
	}
}

// Execute genera PDF, sube a S3, guarda en Mongo y publica.
// Orden: PDF → S3 → Mongo → evento.
func (u *GenerateHistory) Execute(ctx context.Context, cmd HistoryCommand) (HistoryResult, error) {
	snapshot, err := u.profiles.GetProfile(ctx, strings.TrimSpace(cmd.UserID))
	if err != nil {
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrProfileNotFound, err)
	}

	reader := u.appointments
	appointments, err := reader.ListAppointments(ctx, snapshot.UserID)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if len(appointments) == 0 {
		return HistoryResult{}, fmt.Errorf("%w", ErrNoMedicalRecords)
	}

	now := u.clock.Now().UTC()
	pdfID := strings.TrimSpace(u.ids.NewID())
	eventID := strings.TrimSpace(u.ids.NewID())
	correlationID := strings.TrimSpace(cmd.CorrelationID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(u.ids.NewID())
	}
	fileKey := "medical-history/" + snapshot.UserID + "/" + pdfID + ".pdf"

	exp := MedicalHistoryExport{
		ID:          pdfID,
		UserID:      snapshot.UserID,
		RecordCount: len(appointments),
		FileKey:     fileKey,
		GeneratedAt: now,
	}
	exp.GeneratedAt = exp.GeneratedAt.UTC()
	if err := exp.Validate(); err != nil {
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	pdfBytes, err := u.pdf.Generate(snapshot, appointments, exp)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("%w: genera pdf: %v", ErrStorage, err)
	}

	// Subida a S3. Si falla no se guarda ni se publica nada.
	if err := u.storage.Upload(ctx, fileKey, pdfBytes, "application/pdf"); err != nil {
		u.log.Error("history.upload_failed", "pdfId", pdfID)
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	// Guardado en Mongo. Si falla después de subir queda un archivo
	// huérfano sin consecuencias (se acepta por alcance).
	if err := u.repo.SaveHistoryExport(ctx, exp); err != nil {
		u.log.Error("history.save_failed", "pdfId", pdfID)
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	evt := HistoryEvent{
		Metadata: EventMetadata{
			EventID:       eventID,
			EventVersion:  "1",
			OccurredAt:    now.Format(time.RFC3339),
			CorrelationID: correlationID,
		},
		Data: HistoryData{
			PdfID:       pdfID,
			UserID:      snapshot.UserID,
			Email:       strings.TrimSpace(cmd.Email),
			FirstName:   strings.TrimSpace(cmd.FirstName),
			GeneratedAt: now.Format(time.RFC3339),
			RecordCount: len(appointments),
			File: EventFile{
				Bucket:      u.bucket,
				Key:         fileKey,
				ContentType: "application/pdf",
				FileName:    "historial-medico-" + shortID(pdfID) + ".pdf",
			},
		},
	}

	// Publicación. Si falla queda guardado y se responde 502 sin revertir.
	// TODO: el patrón Outbox cerraría esta ventana de inconsistencia
	// (guardar evento pendiente en la misma transacción y publicarlo con
	// un Relay). Se omite a propósito por alcance académico.
	if err := u.publisher.PublishMedicalHistoryGenerated(ctx, evt); err != nil {
		u.log.Error("history.publish_failed",
			"pdfId", pdfID,
			"email", maskEmail(cmd.Email))
		return HistoryResult{}, fmt.Errorf("%w: %v", ErrEventPublish, err)
	}

	u.log.Info("history.generated",
		"pdfId", pdfID,
		"email", maskEmail(cmd.Email),
		"correlationId", correlationID)

	return HistoryResult{
		PdfID:       pdfID,
		GeneratedAt: now,
		RecordCount: len(appointments),
	}, nil
}
