package authz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
)

// NotifyAuthorization notifica una autorización por correo:
// genera el PDF, lo sube a S3, guarda su clave y publica el evento.
// Orden: PDF → S3 → Mongo → evento.
type NotifyAuthorization struct {
	profiles  documents.ProfileReader
	reader    AuthorizationReader
	pdf       PDFGenerator
	storage   documents.FileStorage
	repo      FileKeySaver
	publisher NotifyEventPublisher
	clock     documents.Clock
	ids       documents.IDGenerator
	bucket    string
	log       *slog.Logger
}

// NewNotifyAuthorization inyecta dependencias por constructor.
func NewNotifyAuthorization(
	profiles documents.ProfileReader,
	reader AuthorizationReader,
	pdf PDFGenerator,
	storage documents.FileStorage,
	repo FileKeySaver,
	publisher NotifyEventPublisher,
	clock documents.Clock,
	ids documents.IDGenerator,
	bucket string,
	log *slog.Logger,
) *NotifyAuthorization {
	return &NotifyAuthorization{
		profiles: profiles, reader: reader, pdf: pdf,
		storage: storage, repo: repo, publisher: publisher,
		clock: clock, ids: ids, bucket: bucket, log: log,
	}
}

// NotifyResult es la respuesta del caso de uso.
type NotifyResult struct {
	AuthorizationID string
	Status          string
}

// Execute obtiene la autorización (solo del propio usuario), el perfil,
// genera el PDF, lo sube a S3, guarda su clave y publica el evento.
func (u *NotifyAuthorization) Execute(ctx context.Context, cmd NotifyCommand) (NotifyResult, error) {
	userID := strings.TrimSpace(cmd.UserID)
	authorizationID := strings.TrimSpace(cmd.AuthorizationID)
	if userID == "" || authorizationID == "" {
		return NotifyResult{}, fmt.Errorf("%w", documents.ErrAuthorizationNotFound)
	}
	item, err := u.reader.GetByID(ctx, userID, authorizationID)
	if err != nil {
		return NotifyResult{}, err
	}
	snapshot, err := u.profiles.GetProfile(ctx, userID)
	if err != nil {
		return NotifyResult{}, fmt.Errorf("%w: %v", documents.ErrProfileNotFound, err)
	}
	data, err := u.pdf.GenerateAuthorization(snapshot, item)
	if err != nil {
		return NotifyResult{}, fmt.Errorf("%w: genera pdf: %v", documents.ErrStorage, err)
	}

	now := u.clock.Now().UTC()
	fileKey := "authorizations/" + snapshot.UserID + "/" + item.Authorization.ID + ".pdf"

	// Subida a S3. Si falla no se guarda ni se publica nada.
	if err := u.storage.Upload(ctx, fileKey, data, "application/pdf"); err != nil {
		u.log.Error("authz.upload_failed", "authorizationId", item.Authorization.ID)
		return NotifyResult{}, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}

	// Guardado de la clave. Si falla después de subir queda un archivo
	// huérfano sin consecuencias (se acepta por alcance).
	if err := u.repo.SetAuthorizationFileKey(ctx, userID, item.Authorization.ID, fileKey); err != nil {
		u.log.Error("authz.save_failed", "authorizationId", item.Authorization.ID)
		return NotifyResult{}, fmt.Errorf("%w: %v", documents.ErrStorage, err)
	}

	eventID := strings.TrimSpace(u.ids.NewID())
	correlationID := strings.TrimSpace(cmd.CorrelationID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(u.ids.NewID())
	}
	evt := NotifyEvent{
		Metadata: documents.EventMetadata{
			EventID:       eventID,
			EventVersion:  "1",
			OccurredAt:    now.Format(time.RFC3339),
			CorrelationID: correlationID,
		},
		Data: NotifyData{
			Email:             strings.TrimSpace(cmd.Email),
			FirstName:         strings.TrimSpace(cmd.FirstName),
			AuthorizationType: item.Authorization.Type.Label(),
			Status:            item.Authorization.Status.Label(),
			ChangedAt:         now.Format(time.RFC3339),
			File: documents.EventFile{
				Bucket:      u.bucket,
				Key:         fileKey,
				ContentType: "application/pdf",
				FileName:    notifyFileName(item.Authorization),
			},
		},
	}

	// Publicación. Si falla queda guardado y se responde 502 sin revertir.
	// TODO: el patrón Outbox cerraría esta ventana de inconsistencia
	// (guardar evento pendiente en la misma transacción y publicarlo con
	// un Relay). Se omite a propósito por alcance académico.
	if err := u.publisher.PublishAuthorizationStatusChanged(ctx, evt); err != nil {
		u.log.Error("authz.publish_failed",
			"authorizationId", item.Authorization.ID,
			"email", maskEmail(cmd.Email))
		return NotifyResult{}, fmt.Errorf("%w: %v", documents.ErrEventPublish, err)
	}

	u.log.Info("authz.notified",
		"authorizationId", item.Authorization.ID,
		"email", maskEmail(cmd.Email),
		"correlationId", correlationID)

	return NotifyResult{
		AuthorizationID: item.Authorization.ID,
		Status:          string(item.Authorization.Status),
	}, nil
}

// NotifyCommand es la entrada del caso de uso (datos del JWT + correlación).
type NotifyCommand struct {
	UserID          string
	FirstName       string
	Email           string
	AuthorizationID string
	CorrelationID   string
}

func notifyFileName(a documents.Authorization) string {
	short := a.ID
	if i := strings.Index(short, "-"); i > 0 {
		short = short[:i]
	} else if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("autorizacion-%s-%s.pdf", a.Type.Slug(), short)
}

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
