package documents

import "errors"

// Errores del historial. El handler los traduce a HTTP.
var (
	// ErrNoMedicalRecords indica que el usuario no tiene citas atendidas.
	ErrNoMedicalRecords = errors.New("no medical records")
)
