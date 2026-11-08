package documents

import (
	"errors"
	"strings"
	"time"
)

// MedicalHistoryExport son los metadatos de un PDF del historial médico.
// GeneratedAt es Date en UTC.
type MedicalHistoryExport struct {
	ID          string    `bson:"_id"`
	UserID      string    `bson:"userId"`
	RecordCount int       `bson:"recordCount"`
	FileKey     string    `bson:"fileKey"`
	GeneratedAt time.Time `bson:"generatedAt"`
}

// Validate verifica campos obligatorios.
func (e MedicalHistoryExport) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("pdf id obligatorio")
	}
	if strings.TrimSpace(e.UserID) == "" {
		return errors.New("user id obligatorio")
	}
	if e.RecordCount < 0 {
		return errors.New("recordCount no puede ser negativo")
	}
	if strings.TrimSpace(e.FileKey) == "" {
		return errors.New("file key obligatoria")
	}
	if e.GeneratedAt.IsZero() {
		return errors.New("generatedAt obligatorio")
	}
	return nil
}
