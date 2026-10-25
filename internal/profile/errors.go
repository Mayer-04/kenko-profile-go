// Package profile contiene el dominio del microservicio: la entidad Profile,
// los contratos de eventos, los puertos de persistencia y los casos de uso.
package profile

import "errors"

// Errores del dominio. Los adaptadores los envuelven con contexto.
var (
	// ErrInvalidEvent es un evento de entrada que no cumple el contrato de la
	// sección 7.1. El mensaje no se elimina de SQS y acaba en la DLQ.
	ErrInvalidEvent = errors.New("evento inválido")

	// ErrDuplicateEvent es un evento de entrada ya procesado (idempotencia,
	// sección 5.3). Se descarta sin crear nada.
	ErrDuplicateEvent = errors.New("evento ya procesado")

	// ErrProfileAlreadyExists indica que ya existe un perfil para ese usuario
	// creado por otro evento distinto. El mensaje no se elimina.
	ErrProfileAlreadyExists = errors.New("ya existe un perfil para ese usuario")

	// ErrProfileNotFound indica que no existe un perfil con ese id.
	ErrProfileNotFound = errors.New("perfil no encontrado")

	// ErrInvalidPatch es un PATCH que no cumple las reglas de la sección 4.2.
	ErrInvalidPatch = errors.New("cambios no válidos")
)
