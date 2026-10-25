package profile

import (
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"

	"uuid"
)

// validRegistered es el ejemplo de la sección 7.1.
func validRegistered() UserRegistered {
	return UserRegistered{
		Metadata: EventMetadata{
			EventID:       "01a136f6-7882-72d6-afa9-cfb20ef454f6",
			EventVersion:  "1",
			OccurredAt:    time.Date(2026, 10, 13, 20, 15, 30, 0, time.UTC),
			CorrelationID: "01a136f6-7a11-7b22-c3d4-e5f60718293a",
		},
		Data: RegistrationData{
			UserID: "01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012",
			Document: DocIdentity{
				Type:   "CC",
				Number: "1020304050",
			},
			Person: Person{
				FirstName: "Maria Fernanda",
				LastName:  "Gomez Rojas",
				BirthDate: "1995-03-21",
				Gender:    "female",
			},
			Contact: Contact{
				Email:         "maria.gomez@example.com",
				EmailVerified: false,
				Phone:         "+573001234567",
			},
			Address: AddressData{
				Line: "Cra 15 # 93-60 Apto 402",
				City: "Bogota",
			},
		},
	}
}

func marshalDetail(t *testing.T, event UserRegistered) []byte {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("serializando el evento: %v", err)
	}
	return data
}

func TestParseUserRegistered(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*UserRegistered)
		raw       string // si se define, se usa tal cual como cuerpo
		wantValid bool
	}{
		{name: "evento válido de la sección 7.1", mutate: func(*UserRegistered) {}, wantValid: true},
		{
			name:      "eventId no es UUID",
			mutate:    func(e *UserRegistered) { e.Metadata.EventID = "no-es-uuid" },
			wantValid: false,
		},
		{
			name:      "eventVersion distinto de 1",
			mutate:    func(e *UserRegistered) { e.Metadata.EventVersion = "2" },
			wantValid: false,
		},
		{
			name:      "occurredAt ausente",
			mutate:    func(e *UserRegistered) { e.Metadata.OccurredAt = time.Time{} },
			wantValid: false,
		},
		{
			name:      "userId no es UUID",
			mutate:    func(e *UserRegistered) { e.Data.UserID = "abc" },
			wantValid: false,
		},
		{
			name:      "document.type vacío",
			mutate:    func(e *UserRegistered) { e.Data.Document.Type = " " },
			wantValid: false,
		},
		{
			name:      "birthDate con formato incorrecto",
			mutate:    func(e *UserRegistered) { e.Data.Person.BirthDate = "21/03/1995" },
			wantValid: false,
		},
		{
			name:      "birthDate en el futuro",
			mutate:    func(e *UserRegistered) { e.Data.Person.BirthDate = "2100-01-01" },
			wantValid: false,
		},
		{
			name:      "gender fuera del enum",
			mutate:    func(e *UserRegistered) { e.Data.Person.Gender = "n/a" },
			wantValid: false,
		},
		{
			name:      "email inválido",
			mutate:    func(e *UserRegistered) { e.Data.Contact.Email = "maria.gomez" },
			wantValid: false,
		},
		{
			name:      "phone sin formato E.164",
			mutate:    func(e *UserRegistered) { e.Data.Contact.Phone = "3001234567" },
			wantValid: false,
		},
		{
			name:      "address.city vacío",
			mutate:    func(e *UserRegistered) { e.Data.Address.City = "" },
			wantValid: false,
		},
		{
			name:      "JSON mal formado",
			raw:       `{"metadata":`,
			wantValid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := []byte(test.raw)
			if test.raw == "" {
				event := validRegistered()
				test.mutate(&event)
				detail = marshalDetail(t, event)
			}

			_, err := ParseUserRegistered(detail)

			if test.wantValid && err != nil {
				t.Fatalf("se esperaba evento válido, error: %v", err)
			}
			if !test.wantValid {
				if err == nil {
					t.Fatal("se esperaba error de validación")
				}
				if !strings.Contains(err.Error(), ErrInvalidEvent.Error()) {
					t.Errorf("el error %q no envuelve a ErrInvalidEvent", err)
				}
			}
		})
	}
}

func TestParseUserRegisteredAceptaCamposDesconocidos(t *testing.T) {
	detail := marshalDetail(t, validRegistered())
	detail = []byte(strings.Replace(string(detail), `"userId"`, `"campoFuturo":1,"userId"`, 1))

	if _, err := ParseUserRegistered(detail); err != nil {
		t.Fatalf("los campos desconocidos no deben invalidar el evento: %v", err)
	}
}

func TestParseUserEmailChanged(t *testing.T) {
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

	if _, err := ParseUserEmailChanged(mustMarshal(t, valid)); err != nil {
		t.Fatalf("evento válido rechazado: %v", err)
	}

	invalid := valid
	invalid.Data.Email = "sin-formato"
	if _, err := ParseUserEmailChanged(mustMarshal(t, invalid)); err == nil {
		t.Fatal("se esperaba error con email inválido")
	}
}

func mustMarshal[T any](t *testing.T, value T) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("serializando: %v", err)
	}
	return data
}

func TestBuildProfileCreated(t *testing.T) {
	p, err := NewFromRegistration(validRegistered(), time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC))
	if err != nil {
		t.Fatalf("construyendo el perfil: %v", err)
	}

	eventID := uuid.NewV7()
	occurredAt := time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC)
	created := BuildProfileCreated(p, eventID, occurredAt, validRegistered().Metadata.CorrelationID)

	if created.Metadata.EventID != eventID.String() {
		t.Errorf("metadata.eventId = %q, se quiere el nuevo %q", created.Metadata.EventID, eventID.String())
	}
	if created.Metadata.EventID == validRegistered().Metadata.EventID {
		t.Error("el eventId debe ser distinto al del UserRegistered (sección 7.2)")
	}
	if created.Metadata.EventVersion != "1" {
		t.Errorf("metadata.eventVersion = %q, se quiere \"1\"", created.Metadata.EventVersion)
	}
	if !created.Metadata.OccurredAt.Equal(occurredAt) {
		t.Errorf("metadata.occurredAt = %v, se quiere %v", created.Metadata.OccurredAt, occurredAt)
	}
	if created.Metadata.CorrelationID != validRegistered().Metadata.CorrelationID {
		t.Errorf("metadata.correlationId = %q, se quiere el heredado", created.Metadata.CorrelationID)
	}

	// La data replica la estructura del UserRegistered (sección 7.2).
	want := validRegistered().Data
	if created.Data != want {
		t.Errorf("data = %+v, se quiere %+v", created.Data, want)
	}
}

func TestBuildProfileCreatedFormatoDeSerializacion(t *testing.T) {
	p, err := NewFromRegistration(validRegistered(), time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC))
	if err != nil {
		t.Fatalf("construyendo el perfil: %v", err)
	}
	created := BuildProfileCreated(p, uuid.NewV7(), time.Now().UTC(), "01a136f6-7a11-7b22-c3d4-e5f60718293a")

	data, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("serializando ProfileCreated: %v", err)
	}
	for _, key := range []string{
		`"eventId"`, `"eventVersion"`, `"occurredAt"`, `"correlationId"`,
		`"userId"`, `"document"`, `"type"`, `"number"`,
		`"person"`, `"firstName"`, `"lastName"`, `"birthDate"`, `"gender"`,
		`"contact"`, `"email"`, `"emailVerified"`, `"phone"`,
		`"address"`, `"line"`, `"city"`,
	} {
		if !strings.Contains(string(data), key) {
			t.Errorf("el detail serializado no contiene %s: %s", key, data)
		}
	}
}
