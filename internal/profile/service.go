package profile

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/profile/events"
	"github.com/gofrs/uuid/v5"
)

// Service concentra la lógica de negocio de perfiles.
type Service struct {
	store Store
	log   *slog.Logger
}

// NewService inyecta dependencias (store + logger centralizado).
func NewService(store Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

// StoreForRelay expone el store para el Relay (mismo proceso o separado).
func (s *Service) StoreForRelay() Store {
	return s.store
}

// RegisterFromEvent procesa un UserRegistered: valida, escribe en
// transacción y clasifica el resultado (creado / duplicado).
func (s *Service) RegisterFromEvent(ctx context.Context, evt events.UserRegistered) (profileID string, duplicated bool, err error) {
	now := time.Now().UTC()
	profileID, duplicated, err = s.store.CreateFromRegistration(ctx, evt, now)
	if err != nil {
		s.log.Error("profile.create_failed",
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
		return "", false, err
	}
	if duplicated {
		s.log.Info("profile.duplicate_skipped",
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID,
			"profileId", profileID)
		return profileID, true, nil
	}
	s.log.Info("profile.created",
		"profileId", profileID,
		"eventId", evt.Metadata.EventID,
		"correlationId", evt.Metadata.CorrelationID)
	return profileID, false, nil
}

// ApplyEmailChanged procesa EmailChanged (contrato v1, hecho consumado).
// Solo registra IDs, nunca emails (11.3). Todos los casos salvo error de
// infra son terminales: el worker borra el mensaje (no reintenta/DLQ).
func (s *Service) ApplyEmailChanged(ctx context.Context, evt events.EmailChanged) error {
	updated, duplicated, notFound, err := s.store.ApplyEmailChanged(ctx, evt)
	if err != nil {
		s.log.Error("profile.email_change_failed",
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
		return err
	}
	switch {
	case duplicated:
		s.log.Info("profile.duplicate_skipped",
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
	case notFound:
		s.log.Info("profile.email_user_not_found",
			"profileId", evt.Data.UserID,
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
	case updated:
		s.log.Info("profile.email_updated",
			"profileId", evt.Data.UserID,
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
	default:
		s.log.Info("profile.email_stale_ignored",
			"profileId", evt.Data.UserID,
			"eventId", evt.Metadata.EventID,
			"correlationId", evt.Metadata.CorrelationID)
	}
	return nil
}

// GetByID recupera el perfil por su id.
func (s *Service) GetByID(ctx context.Context, id string) (Profile, error) {
	if _, err := uuid.FromString(strings.TrimSpace(id)); err != nil {
		return Profile{}, ErrNotFound
	}
	return s.store.FindByID(ctx, id)
}

// ValidatePatch rechaza cualquier campo fuera de phone/address (4.2, 9),
// incluidos email y password. address debe venir completo.
func ValidatePatch(raw map[string]any) (PatchInput, error) {
	var in PatchInput
	for k := range raw {
		switch k {
		case "phone", "address":
		default:
			return PatchInput{}, fmt.Errorf("%w: %q", ErrInvalidPatch, k)
		}
	}
	if v, ok := raw["phone"]; ok {
		if v == nil {
			return PatchInput{}, fmt.Errorf("%w: phone nulo", ErrInvalidPatch)
		}
		phone, ok := v.(string)
		if !ok || strings.TrimSpace(phone) == "" {
			return PatchInput{}, fmt.Errorf("%w: phone inválido", ErrInvalidPatch)
		}
		phone = strings.TrimSpace(phone)
		in.Phone = &phone
	}
	if v, ok := raw["address"]; ok {
		if v == nil {
			return PatchInput{}, fmt.Errorf("%w: address nula", ErrInvalidPatch)
		}
		m, ok := v.(map[string]any)
		if !ok {
			return PatchInput{}, fmt.Errorf("%w: address debe ser objeto", ErrInvalidPatch)
		}
		lineRaw, lok := m["line"]
		cityRaw, cok := m["city"]
		if !lok || !cok || len(m) != 2 {
			return PatchInput{}, fmt.Errorf("%w: address debe traer line y city completos", ErrInvalidPatch)
		}
		line, ok1 := lineRaw.(string)
		city, ok2 := cityRaw.(string)
		if !ok1 || !ok2 || strings.TrimSpace(line) == "" || strings.TrimSpace(city) == "" {
			return PatchInput{}, fmt.Errorf("%w: address inválida", ErrInvalidPatch)
		}
		line = strings.TrimSpace(line)
		city = strings.TrimSpace(city)
		in.AddressLine = &line
		in.City = &city
		in.HasAddress = true
	}
	if in.Phone == nil && !in.HasAddress {
		return PatchInput{}, fmt.Errorf("%w: nada que actualizar", ErrInvalidPatch)
	}
	return in, nil
}

// Patch aplica la actualización (sin publicar eventos) y registra
// solo el id y los nombres de campos (11.3).
func (s *Service) Patch(ctx context.Context, id string, in PatchInput) (Profile, error) {
	if _, err := uuid.FromString(strings.TrimSpace(id)); err != nil {
		return Profile{}, ErrNotFound
	}
	p, err := s.store.Patch(ctx, id, in, time.Now().UTC())
	if err != nil {
		s.log.Error("profile.patch_failed", "profileId", id)
		return Profile{}, err
	}
	fields := []string{}
	if in.Phone != nil {
		fields = append(fields, "phone")
	}
	if in.HasAddress {
		fields = append(fields, "address")
	}
	s.log.Info("profile.updated", "profileId", id, "fields", strings.Join(fields, ","))
	return p, nil
}
