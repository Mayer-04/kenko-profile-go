package profile

import "errors"

// Errores de dominio del servicio de perfiles.
var (
	ErrNotFound       = errors.New("profile not found")
	ErrInvalidPatch   = errors.New("patch contains immutable or unknown fields")
	ErrInvalidPayload = errors.New("invalid event payload")
	ErrDuplicate      = errors.New("duplicate event")
)
