package documents

import (
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

// Errores centinela del feature. El handler HTTP los traduce.
var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrStorage         = errors.New("storage error")
	ErrEventPublish    = errors.New("event publish failed")
)

// Certificate es la foto fija del certificado de afiliación.
// Las fechas se guardan en Mongo como Date en UTC.
type Certificate struct {
	ID                   string
	UserID               string
	AffiliationStartDate time.Time
	IssuedAt             time.Time
	FileKey              string
}

// Validate verifica ids obligatorios y fechas en UTC.
func (c Certificate) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("certificate id obligatorio")
	}
	if _, err := uuid.FromString(strings.TrimSpace(c.ID)); err != nil {
		return errors.New("certificate id inválido")
	}
	if strings.TrimSpace(c.UserID) == "" {
		return errors.New("user id obligatorio")
	}
	if _, err := uuid.FromString(strings.TrimSpace(c.UserID)); err != nil {
		return errors.New("user id inválido")
	}
	if c.AffiliationStartDate.IsZero() {
		return errors.New("affiliation start date obligatoria")
	}
	if c.IssuedAt.IsZero() {
		return errors.New("issued at obligatorio")
	}
	if strings.TrimSpace(c.FileKey) == "" {
		return errors.New("file key obligatoria")
	}
	return nil
}

// EnsureUTC normaliza las fechas a UTC.
func (c *Certificate) EnsureUTC() {
	c.AffiliationStartDate = c.AffiliationStartDate.UTC()
	c.IssuedAt = c.IssuedAt.UTC()
}
