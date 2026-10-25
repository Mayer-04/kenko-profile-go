package profile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"

	"uuid"
)

// fakeStore implementa profile.Store registrando lo que recibe y permitiendo
// inyectar los fallos que se quieren probar.
type fakeStore struct {
	// Estado guardado.
	createdProfiles []Profile
	createdOutbox   []OutboxEvent
	createdMarkers  []ProcessedEvent
	emailChanges    []EmailChange
	updatedPatches  []Patch
	updatedIDs      []uuid.UUID
	getIDs          []uuid.UUID

	// Fallos a devolver.
	createProfileErr  error
	applyEmailErr     error
	applyEmailApplied bool
	getErr            error
	updateErr         error
}

func (f *fakeStore) CreateProfile(_ context.Context, p Profile, out OutboxEvent, processed ProcessedEvent) error {
	if f.createProfileErr != nil {
		return f.createProfileErr
	}
	f.createdProfiles = append(f.createdProfiles, p)
	f.createdOutbox = append(f.createdOutbox, out)
	f.createdMarkers = append(f.createdMarkers, processed)
	return nil
}

func (f *fakeStore) ApplyEmailChange(_ context.Context, change EmailChange, processed ProcessedEvent) (bool, error) {
	if f.applyEmailErr != nil {
		return false, f.applyEmailErr
	}
	f.emailChanges = append(f.emailChanges, change)
	f.createdMarkers = append(f.createdMarkers, processed)
	return f.applyEmailApplied, nil
}

func (f *fakeStore) GetProfile(_ context.Context, id uuid.UUID) (Profile, error) {
	if f.getErr != nil {
		return Profile{}, f.getErr
	}
	f.getIDs = append(f.getIDs, id)
	return validRegisteredProfile(), nil
}

func (f *fakeStore) UpdateProfile(_ context.Context, id uuid.UUID, patch Patch) (Profile, error) {
	if f.updateErr != nil {
		return Profile{}, f.updateErr
	}
	f.updatedIDs = append(f.updatedIDs, id)
	f.updatedPatches = append(f.updatedPatches, patch)
	p := validRegisteredProfile()
	if patch.Phone != nil {
		p.Phone = *patch.Phone
	}
	if patch.Address != nil {
		p.AddressLine = patch.Address.Line
		p.City = patch.Address.City
	}
	return p, nil
}

func validRegisteredProfile() Profile {
	p, err := NewFromRegistration(validRegistered(), time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	return p
}

func TestCreateFromRegistrationCreaPerfilYOutbox(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)

	outcome, err := service.CreateFromRegistration(t.Context(), marshalDetail(t, validRegistered()))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if outcome.Kind != OutcomeCreated {
		t.Errorf("resultado = %q, se quiere created", outcome.Kind)
	}
	if len(store.createdProfiles) != 1 {
		t.Fatalf("perfiles guardados = %d, se quiere 1", len(store.createdProfiles))
	}

	profile := store.createdProfiles[0]
	if profile.ID.String() != validRegistered().Data.UserID {
		t.Errorf("profileId = %q, se quiere el userId del evento", profile.ID)
	}

	// El outbox guarda el detail completo de ProfileCreated (sección 7.2).
	if len(store.createdOutbox) != 1 {
		t.Fatalf("eventos outbox = %d, se quiere 1", len(store.createdOutbox))
	}
	outbox := store.createdOutbox[0]
	if outbox.EventType != TypeProfileCreated {
		t.Errorf("event_type = %q", outbox.EventType)
	}
	if outbox.CorrelationID != validRegistered().Metadata.CorrelationID {
		t.Errorf("correlation_id = %q, se quiere el heredado", outbox.CorrelationID)
	}

	var created ProfileCreated
	if err := json.Unmarshal([]byte(outbox.Payload), &created); err != nil {
		t.Fatalf("el payload del outbox no es un detail válido: %v", err)
	}
	if created.Metadata.EventID == validRegistered().Metadata.EventID {
		t.Error("el eventId del outbox debe ser nuevo, no el del UserRegistered")
	}
	if created.Data.UserID != validRegistered().Data.UserID {
		t.Errorf("data.userId = %q", created.Data.UserID)
	}

	// El marcador de idempotencia usa el eventId del evento de entrada.
	if len(store.createdMarkers) != 1 {
		t.Fatalf("marcadores = %d, se quiere 1", len(store.createdMarkers))
	}
	if store.createdMarkers[0].EventID.String() != validRegistered().Metadata.EventID {
		t.Errorf("el marcador registra eventId %q", store.createdMarkers[0].EventID)
	}
}

func TestCreateFromRegistrationDuplicado(t *testing.T) {
	store := &fakeStore{createProfileErr: ErrDuplicateEvent}
	service := NewService(store)

	outcome, err := service.CreateFromRegistration(t.Context(), marshalDetail(t, validRegistered()))
	if err != nil {
		t.Fatalf("un duplicado no es error: %v", err)
	}
	if outcome.Kind != OutcomeDuplicate {
		t.Errorf("resultado = %q, se quiere duplicate", outcome.Kind)
	}
	if outcome.EventID != validRegistered().Metadata.EventID {
		t.Errorf("eventId del resultado = %q", outcome.EventID)
	}
}

func TestCreateFromRegistrationPerfilExistente(t *testing.T) {
	store := &fakeStore{createProfileErr: ErrProfileAlreadyExists}
	service := NewService(store)

	_, err := service.CreateFromRegistration(t.Context(), marshalDetail(t, validRegistered()))
	if !errors.Is(err, ErrProfileAlreadyExists) {
		t.Fatalf("error = %v, se quiere ErrProfileAlreadyExists", err)
	}
}

func TestCreateFromRegistrationEventoInvalido(t *testing.T) {
	service := NewService(&fakeStore{})

	invalid := validRegistered()
	invalid.Metadata.EventVersion = "9"

	_, err := service.CreateFromRegistration(t.Context(), marshalDetail(t, invalid))
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, se quiere ErrInvalidEvent", err)
	}
	if !strings.Contains(err.Error(), "eventVersion") {
		t.Errorf("el error debería citar el campo: %v", err)
	}
}

func TestApplyEmailChange(t *testing.T) {
	valid := UserEmailChanged{
		Metadata: EventMetadata{
			EventID:       "01a136f6-7882-72d6-afa9-cfb20ef454f6",
			EventVersion:  "1",
			OccurredAt:    time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
			CorrelationID: "01a136f6-7a11-7b22-c3d4-e5f60718293a",
		},
		Data: EmailChangeData{
			UserID:        "01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012",
			Email:         "maria.nueva@example.com",
			EmailVerified: true,
		},
	}
	detail := mustMarshal(t, valid)

	t.Run("aplicado", func(t *testing.T) {
		store := &fakeStore{applyEmailApplied: true}
		outcome, err := NewService(store).ApplyEmailChange(t.Context(), detail)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if outcome.Kind != OutcomeApplied {
			t.Errorf("resultado = %q, se quiere applied", outcome.Kind)
		}
		if len(store.emailChanges) != 1 {
			t.Fatalf("cambios de email = %d", len(store.emailChanges))
		}
		if store.emailChanges[0].Email != valid.Data.Email {
			t.Errorf("email = %q", store.emailChanges[0].Email)
		}
	})

	t.Run("antiguo: se ignora", func(t *testing.T) {
		store := &fakeStore{applyEmailApplied: false}
		outcome, err := NewService(store).ApplyEmailChange(t.Context(), detail)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if outcome.Kind != OutcomeStale {
			t.Errorf("resultado = %q, se quiere stale", outcome.Kind)
		}
	})

	t.Run("duplicado", func(t *testing.T) {
		store := &fakeStore{applyEmailErr: ErrDuplicateEvent}
		outcome, err := NewService(store).ApplyEmailChange(t.Context(), detail)
		if err != nil {
			t.Fatalf("un duplicado no es error: %v", err)
		}
		if outcome.Kind != OutcomeDuplicate {
			t.Errorf("resultado = %q, se quiere duplicate", outcome.Kind)
		}
	})

	t.Run("perfil inexistente", func(t *testing.T) {
		store := &fakeStore{applyEmailErr: ErrProfileNotFound}
		_, err := NewService(store).ApplyEmailChange(t.Context(), detail)
		if !errors.Is(err, ErrProfileNotFound) {
			t.Fatalf("error = %v, se quiere ErrProfileNotFound", err)
		}
	})
}

func TestUpdateValidaAntesDePersistir(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)

	phone := "3001234567" // sin formato E.164
	_, err := service.Update(t.Context(), validRegisteredProfile().ID, Patch{Phone: &phone})
	if !errors.Is(err, ErrInvalidPatch) {
		t.Fatalf("error = %v, se quiere ErrInvalidPatch", err)
	}
	if len(store.updatedPatches) != 0 {
		t.Error("un parche inválido no debe llegar al store")
	}
}

func TestUpdateAplicaElParche(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)

	phone := "+573002223344"
	address := &Address{Line: "Calle 100 # 10-20", City: "Medellin"}
	updated, err := service.Update(t.Context(), validRegisteredProfile().ID, Patch{Phone: &phone, Address: address})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if updated.Phone != phone || updated.AddressLine != address.Line || updated.City != address.City {
		t.Errorf("perfil actualizado = %+v", updated)
	}
}
