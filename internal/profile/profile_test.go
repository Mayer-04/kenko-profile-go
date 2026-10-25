package profile

import (
	"errors"
	"testing"
	"time"
)

func TestNewFromRegistration(t *testing.T) {
	event := validRegistered()
	now := time.Date(2026, 10, 13, 20, 15, 31, 0, time.UTC)

	p, err := NewFromRegistration(event, now)
	if err != nil {
		t.Fatalf("construyendo el perfil: %v", err)
	}

	if p.ID.String() != event.Data.UserID {
		t.Errorf("_id = %q, se quiere el userId %q (coincide con Authentication)", p.ID, event.Data.UserID)
	}
	if p.DocumentType != "CC" || p.DocumentNumber != "1020304050" {
		t.Errorf("documento = %q/%q", p.DocumentType, p.DocumentNumber)
	}
	if p.FirstName != "Maria Fernanda" || p.LastName != "Gomez Rojas" {
		t.Errorf("nombres = %q/%q", p.FirstName, p.LastName)
	}
	wantBirth := time.Date(1995, 3, 21, 0, 0, 0, 0, time.UTC)
	if !p.BirthDate.Equal(wantBirth) {
		t.Errorf("birth_date = %v, se quiere %v", p.BirthDate, wantBirth)
	}
	if p.Gender != "female" {
		t.Errorf("gender = %q", p.Gender)
	}
	if p.Email != "maria.gomez@example.com" || p.EmailVerified {
		t.Errorf("contacto = %q verificado=%v", p.Email, p.EmailVerified)
	}
	if p.Phone != "+573001234567" {
		t.Errorf("phone = %q", p.Phone)
	}
	if p.AddressLine != "Cra 15 # 93-60 Apto 402" || p.City != "Bogota" {
		t.Errorf("dirección = %q/%q", p.AddressLine, p.City)
	}
	if !p.EmailUpdatedAt.Equal(event.Metadata.OccurredAt) {
		t.Errorf("email_updated_at = %v, se quiere el occurredAt del registro", p.EmailUpdatedAt)
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Errorf("marcas de tiempo = %v/%v, se quiere %v", p.CreatedAt, p.UpdatedAt, now)
	}
}

func TestPatchValidate(t *testing.T) {
	validPhone := "+573001234567"
	validAddress := &Address{Line: "Cra 15 # 93-60", City: "Bogota"}

	tests := []struct {
		name    string
		patch   Patch
		wantErr error
	}{
		{
			name:  "solo phone",
			patch: Patch{Phone: &validPhone},
		},
		{
			name:  "solo address",
			patch: Patch{Address: validAddress},
		},
		{
			name:  "phone y address",
			patch: Patch{Phone: &validPhone, Address: validAddress},
		},
		{
			name:    "phone inválido",
			patch:   Patch{Phone: ptrTo("3001234567")},
			wantErr: ErrInvalidPatch,
		},
		{
			name:    "address sin line",
			patch:   Patch{Address: &Address{City: "Bogota"}},
			wantErr: ErrInvalidPatch,
		},
		{
			name:    "address sin city",
			patch:   Patch{Address: &Address{Line: "Cra 15"}},
			wantErr: ErrInvalidPatch,
		},
		{
			name:  "vacío: válido (lo valida la capa HTTP)",
			patch: Patch{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.patch.Validate()

			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("se esperaba parche válido, error: %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, se quiere %v", err, test.wantErr)
			}
		})
	}
}

func TestPatchFieldNames(t *testing.T) {
	phone := "+573001234567"
	address := &Address{Line: "Cra 15", City: "Bogota"}

	tests := []struct {
		name  string
		patch Patch
		want  []string
	}{
		{name: "vacío", patch: Patch{}, want: []string{}},
		{name: "solo phone", patch: Patch{Phone: &phone}, want: []string{"phone"}},
		{name: "phone y address", patch: Patch{Phone: &phone, Address: address}, want: []string{"phone", "address"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.patch.FieldNames()
			if len(got) != len(test.want) {
				t.Fatalf("campos = %v, se quiere %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("campos = %v, se quiere %v", got, test.want)
				}
			}
		})
	}
}

func ptrTo(value string) *string { return &value }
