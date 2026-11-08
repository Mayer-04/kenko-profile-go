package documents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// GenerateCertificate es el caso de uso del certificado.
// No conoce HTTP, Mongo ni AWS: solo interfaces.
type GenerateCertificate struct {
	profiles  ProfileReader
	repo      CertificateRepository
	pdf       PDFGenerator
	storage   FileStorage
	publisher EventPublisher
	clock     Clock
	ids       IDGenerator
	bucket    string
	log       *slog.Logger
}

// NewGenerateCertificate inyecta todas las dependencias por constructor.
func NewGenerateCertificate(
	profiles ProfileReader,
	repo CertificateRepository,
	pdf PDFGenerator,
	storage FileStorage,
	publisher EventPublisher,
	clock Clock,
	ids IDGenerator,
	bucket string,
	log *slog.Logger,
) *GenerateCertificate {
	return &GenerateCertificate{
		profiles: profiles, repo: repo, pdf: pdf,
		storage: storage, publisher: publisher,
		clock: clock, ids: ids, bucket: bucket, log: log,
	}
}

// Execute genera PDF, sube a S3, guarda en Mongo y publica.
// Orden: PDF → S3 → Mongo → evento.
func (u *GenerateCertificate) Execute(ctx context.Context, cmd GenerateCommand) (GenerateResult, error) {
	snapshot, err := u.profiles.GetProfile(ctx, strings.TrimSpace(cmd.UserID))
	if err != nil {
		return GenerateResult{}, fmt.Errorf("%w: %v", ErrProfileNotFound, err)
	}

	now := u.clock.Now().UTC()
	certificateID := strings.TrimSpace(u.ids.NewID())
	eventID := strings.TrimSpace(u.ids.NewID())
	correlationID := strings.TrimSpace(cmd.CorrelationID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(u.ids.NewID())
	}
	fileKey := "certificates/" + snapshot.UserID + "/" + certificateID + ".pdf"

	cert := Certificate{
		ID:                   certificateID,
		UserID:               snapshot.UserID,
		AffiliationStartDate: snapshot.AffiliationStartDate.UTC(),
		IssuedAt:             now,
		FileKey:              fileKey,
	}
	cert.EnsureUTC()
	if err := cert.Validate(); err != nil {
		return GenerateResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	pdfBytes, err := u.pdf.Generate(snapshot, cert)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("%w: genera pdf: %v", ErrStorage, err)
	}

	// Subida a S3. Si falla no se guarda ni se publica nada.
	if err := u.storage.Upload(ctx, fileKey, pdfBytes, "application/pdf"); err != nil {
		u.log.Error("certificate.upload_failed", "certificateId", certificateID)
		return GenerateResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	// Guardado en Mongo. Si falla después de subir queda un archivo
	// huérfano sin consecuencias (se acepta por alcance).
	if err := u.repo.Save(ctx, cert); err != nil {
		u.log.Error("certificate.save_failed", "certificateId", certificateID)
		return GenerateResult{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}

	evt := CertificateEvent{
		Metadata: EventMetadata{
			EventID:       eventID,
			EventVersion:  "1",
			OccurredAt:    now.Format(time.RFC3339),
			CorrelationID: correlationID,
		},
		Data: EventData{
			CertificateID: certificateID,
			UserID:        snapshot.UserID,
			Email:         strings.TrimSpace(cmd.Email),
			FirstName:     strings.TrimSpace(cmd.FirstName),
			IssuedAt:      now.Format(time.RFC3339),
			File: EventFile{
				Bucket:      u.bucket,
				Key:         fileKey,
				ContentType: "application/pdf",
				FileName:    "certificado-afiliacion-" + shortID(certificateID) + ".pdf",
			},
		},
	}

	// Publicación. Si falla el certificado queda guardado y se responde
	// 502 sin revertir lo guardado.
	// TODO: el patrón Outbox cerraría esta ventana de inconsistencia
	// (guardar evento pendiente en la misma transacción que el certificado
	// y publicarlo con un Relay). Se omite a propósito por alcance académico.
	if err := u.publisher.PublishCertificateGenerated(ctx, evt); err != nil {
		u.log.Error("certificate.publish_failed",
			"certificateId", certificateID,
			"email", maskEmail(cmd.Email))
		return GenerateResult{}, fmt.Errorf("%w: %v", ErrEventPublish, err)
	}

	u.log.Info("certificate.generated",
		"certificateId", certificateID,
		"email", maskEmail(cmd.Email),
		"correlationId", correlationID)

	return GenerateResult{
		CertificateID:        certificateID,
		IssuedAt:             now,
		AffiliationStartDate: cert.AffiliationStartDate,
	}, nil
}

func shortID(id string) string {
	if i := strings.Index(id, "-"); i > 0 {
		return id[:i]
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// maskEmail enmascara el email en logs (ma***@gmail.com). Nunca PII completa.
func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at+1:]
	if len(local) <= 2 {
		local = local[:1] + "***"
	} else {
		local = local[:2] + "***"
	}
	return local + "@" + domain
}
