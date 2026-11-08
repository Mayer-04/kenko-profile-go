package documents

import (
	"errors"
	"strings"
	"time"
)

// Tipos de autorización válidos.
var AuthorizationTypes = []string{"BLOOD_TEST", "XRAY", "COLONOSCOPY"}

// Estados de autorización válidos.
var AuthorizationStatuses = []string{"IN_PROGRESS", "REJECTED", "COMPLETED"}

// Authorization es una autorización del paciente. FileKey apunta al PDF
// más reciente en S3; IssuedAt/UpdatedAt son Date en UTC.
type Authorization struct {
	ID                string    `bson:"_id"`
	UserID            string    `bson:"userId"`
	AuthorizationType string    `bson:"authorizationType"`
	Status            string    `bson:"status"`
	FileKey           string    `bson:"fileKey"`
	IssuedAt          time.Time `bson:"issuedAt"`
	UpdatedAt         time.Time `bson:"updatedAt"`
}

// Validate verifica campos obligatorios y listas cerradas.
func (a Authorization) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("authorization id obligatorio")
	}
	if strings.TrimSpace(a.UserID) == "" {
		return errors.New("user id obligatorio")
	}
	if !validValue(a.AuthorizationType, AuthorizationTypes) {
		return errors.New("tipo de autorización inválido")
	}
	if !validValue(a.Status, AuthorizationStatuses) {
		return errors.New("status de autorización inválido")
	}
	if strings.TrimSpace(a.FileKey) == "" {
		return errors.New("file key obligatoria")
	}
	if a.IssuedAt.IsZero() {
		return errors.New("issuedAt obligatorio")
	}
	if a.UpdatedAt.IsZero() {
		return errors.New("updatedAt obligatorio")
	}
	return nil
}
