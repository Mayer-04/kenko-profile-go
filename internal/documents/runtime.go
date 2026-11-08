package documents

import (
	"time"

	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
)

// SystemClock es el Clock de producción.
type SystemClock struct{}

// Now devuelve la hora actual en UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// UUID7Generator genera UUID v7 con el generador del repo.
type UUID7Generator struct{}

// NewID genera un UUID v7 en texto.
func (UUID7Generator) NewID() string { return platformmongo.MustUUID7().String() }
