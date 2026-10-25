package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	json "encoding/json/v2"

	"uuid"
)

// Service implementa los casos de uso del microservicio (sección 4).
type Service struct {
	store Store
	now   func() time.Time
}

// NewService crea el servicio con su puerto de persistencia.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// OutcomeKind resume qué hizo el servicio con un evento de entrada. El worker
// lo usa para decidir si elimina el mensaje de SQS (sección 5.2).
type OutcomeKind string

const (
	OutcomeCreated   OutcomeKind = "created"   // perfil creado
	OutcomeDuplicate OutcomeKind = "duplicate" // evento ya procesado, se descarta
	OutcomeApplied   OutcomeKind = "applied"   // email del perfil actualizado
	OutcomeStale     OutcomeKind = "stale"     // email ignorado por antigüedad
)

// Outcome describe el resultado de procesar un evento, con los identificadores
// que los logs necesitan (sección 11.3).
type Outcome struct {
	Kind          OutcomeKind
	ProfileID     string
	EventID       string
	CorrelationID string
}

// CreateFromRegistration procesa el detail de un UserRegistered: valida el
// evento y, en una sola transacción, crea el perfil y guarda el
// ProfileCreated pendiente en el outbox (sección 4.1).
func (s *Service) CreateFromRegistration(ctx context.Context, detail []byte) (Outcome, error) {
	event, err := ParseUserRegistered(detail)
	if err != nil {
		return Outcome{}, err
	}

	p, err := NewFromRegistration(event, s.now().UTC())
	if err != nil {
		return Outcome{}, err
	}
	outbox, err := s.newOutboxEvent(p, event.Metadata.CorrelationID)
	if err != nil {
		return Outcome{}, err
	}

	eventID, err := event.Metadata.EventUUID()
	if err != nil {
		return Outcome{}, err
	}
	processed := ProcessedEvent{
		ID:          uuid.NewV7(),
		EventID:     eventID,
		ProcessedAt: s.now().UTC(),
	}

	if err := s.store.CreateProfile(ctx, p, outbox, processed); err != nil {
		if errors.Is(err, ErrDuplicateEvent) {
			return newOutcome(OutcomeDuplicate, "", event.Metadata), nil
		}
		return Outcome{}, err
	}
	return newOutcome(OutcomeCreated, p.ID.String(), event.Metadata), nil
}

// ApplyEmailChange procesa el detail de un UserEmailChanged (D8-B): actualiza
// la copia del email del perfil, ignorando eventos más antiguos que el último
// aplicado. No publica eventos.
func (s *Service) ApplyEmailChange(ctx context.Context, detail []byte) (Outcome, error) {
	event, err := ParseUserEmailChanged(detail)
	if err != nil {
		return Outcome{}, err
	}

	userID, err := event.UserID()
	if err != nil {
		return Outcome{}, err
	}
	eventID, err := event.Metadata.EventUUID()
	if err != nil {
		return Outcome{}, err
	}

	change := EmailChange{
		UserID:        userID,
		Email:         event.Data.Email,
		EmailVerified: event.Data.EmailVerified,
		OccurredAt:    event.Metadata.OccurredAt,
	}
	processed := ProcessedEvent{
		ID:          uuid.NewV7(),
		EventID:     eventID,
		ProcessedAt: s.now().UTC(),
	}

	applied, err := s.store.ApplyEmailChange(ctx, change, processed)
	if err != nil {
		if errors.Is(err, ErrDuplicateEvent) {
			return newOutcome(OutcomeDuplicate, "", event.Metadata), nil
		}
		return Outcome{}, err
	}
	if applied {
		return newOutcome(OutcomeApplied, userID.String(), event.Metadata), nil
	}
	return newOutcome(OutcomeStale, userID.String(), event.Metadata), nil
}

// Get devuelve el perfil por su id (sección 9).
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Profile, error) {
	return s.store.GetProfile(ctx, id)
}

// Update aplica un PATCH de phone y/o address y devuelve el perfil
// actualizado. Nunca publica eventos (sección 12).
func (s *Service) Update(ctx context.Context, id uuid.UUID, patch Patch) (Profile, error) {
	if err := patch.Validate(); err != nil {
		return Profile{}, err
	}
	return s.store.UpdateProfile(ctx, id, patch)
}

// newOutboxEvent construye el evento outbox con el detail completo de
// ProfileCreated (sección 7.2): eventId nuevo, occurredAt de la creación del
// perfil y correlationId heredado del UserRegistered.
func (s *Service) newOutboxEvent(p Profile, correlationID string) (OutboxEvent, error) {
	occurredAt := s.now().UTC()
	payload, err := json.Marshal(BuildProfileCreated(p, uuid.NewV7(), occurredAt, correlationID))
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("serializando ProfileCreated: %w", err)
	}
	return OutboxEvent{
		ID:            uuid.NewV7(),
		EventType:     TypeProfileCreated,
		AggregateID:   p.ID,
		CorrelationID: correlationID,
		Payload:       string(payload),
		CreatedAt:     occurredAt,
	}, nil
}

// newOutcome rellena el resultado con los identificadores del evento de entrada.
func newOutcome(kind OutcomeKind, profileID string, metadata EventMetadata) Outcome {
	return Outcome{
		Kind:          kind,
		ProfileID:     profileID,
		EventID:       metadata.EventID,
		CorrelationID: metadata.CorrelationID,
	}
}
