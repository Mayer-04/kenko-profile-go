package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// fakeService implementa httpapi.Service.
type fakeService struct {
	profiles  map[string]profile.Profile
	updateErr error
	updates   []string
}

func (f *fakeService) Get(_ context.Context, id uuid.UUID) (profile.Profile, error) {
	p, ok := f.profiles[id.String()]
	if !ok {
		return profile.Profile{}, profile.ErrProfileNotFound
	}
	return p, nil
}

func (f *fakeService) Update(_ context.Context, id uuid.UUID, patch profile.Patch) (profile.Profile, error) {
	// Igual que el servicio real: el parche se valida antes de persistir.
	if err := patch.Validate(); err != nil {
		return profile.Profile{}, err
	}
	if f.updateErr != nil {
		return profile.Profile{}, f.updateErr
	}
	p, ok := f.profiles[id.String()]
	if !ok {
		return profile.Profile{}, profile.ErrProfileNotFound
	}
	if patch.Phone != nil {
		p.Phone = *patch.Phone
		f.updates = append(f.updates, "phone")
	}
	if patch.Address != nil {
		p.AddressLine = patch.Address.Line
		p.City = patch.Address.City
		f.updates = append(f.updates, "address")
	}
	f.profiles[id.String()] = p
	return p, nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeService) {
	t.Helper()
	service := &fakeService{profiles: map[string]profile.Profile{}}
	e := echo.New()
	e.Logger = slog.New(slog.DiscardHandler)
	e.HTTPErrorHandler = NewErrorHandler(slog.New(slog.DiscardHandler))
	RegisterRoutes(e, NewHandler(service, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(e)
	t.Cleanup(server.Close)
	return server, service
}

func testProfile(t *testing.T) profile.Profile {
	t.Helper()
	return profile.Profile{
		ID:             uuid.MustParse("01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012"),
		DocumentType:   "CC",
		DocumentNumber: "1020304050",
		FirstName:      "Maria Fernanda",
		LastName:       "Gomez Rojas",
		Phone:          "+573001234567",
		AddressLine:    "Cra 15 # 93-60",
		City:           "Bogota",
		Email:          "maria.gomez@example.com",
	}
}

func TestGetProfile(t *testing.T) {
	server, service := newTestServer(t)
	p := testProfile(t)
	service.profiles[p.ID.String()] = p

	response, err := http.Get(server.URL + "/profiles/" + p.ID.String())
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("estado = %d, se quiere 200", response.StatusCode)
	}
	body := readBody(t, response)
	for _, field := range []string{`"id"`, `"document_type"`, `"phone"`, `"address_line"`, `"city"`, `"birth_date"`} {
		if !strings.Contains(body, field) {
			t.Errorf("la respuesta no contiene %s: %s", field, body)
		}
	}
}

func TestGetPerfilInexistente(t *testing.T) {
	server, _ := newTestServer(t)

	response, err := http.Get(server.URL + "/profiles/" + uuid.NewV7().String())
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("estado = %d, se quiere 404", response.StatusCode)
	}
}

func TestGetIdInvalido(t *testing.T) {
	server, _ := newTestServer(t)

	response, err := http.Get(server.URL + "/profiles/no-es-un-uuid")
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("estado = %d, se quiere 400", response.StatusCode)
	}
}

func TestPatchProfile(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantFields []string // campos que deben quedar actualizados
	}{
		{
			name:       "phone válido",
			body:       `{"phone":"+573002223344"}`,
			wantCode:   http.StatusOK,
			wantFields: []string{"phone"},
		},
		{
			name:       "address completo",
			body:       `{"address":{"line":"Calle 100 # 10-20","city":"Medellin"}}`,
			wantCode:   http.StatusOK,
			wantFields: []string{"address"},
		},
		{
			name:       "phone y address",
			body:       `{"phone":"+573002223344","address":{"line":"Calle 100","city":"Medellin"}}`,
			wantCode:   http.StatusOK,
			wantFields: []string{"phone", "address"},
		},
		{
			name:     "email: rechazado (se cambia en Authentication)",
			body:     `{"email":"nueva@example.com"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "password: rechazado",
			body:     `{"password":"secreto"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "campo inmutable first_name: rechazado",
			body:     `{"first_name":"Otro"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "campo desconocido: rechazado",
			body:     `{"foo":1}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "address incompleto: rechazado",
			body:     `{"address":{"line":"Calle 100"}}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "address con campo desconocido: rechazado",
			body:     `{"address":{"line":"Calle 100","city":"Medellin","zona":"norte"}}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "phone con formato inválido: rechazado",
			body:     `{"phone":"3001234567"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "parche vacío: rechazado",
			body:     `{}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "cuerpo mal formado: rechazado",
			body:     `{"phone":`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, service := newTestServer(t)
			p := testProfile(t)
			service.profiles[p.ID.String()] = p

			request, err := http.NewRequest(http.MethodPatch, server.URL+"/profiles/"+p.ID.String(), strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("creando la petición: %v", err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("error en la petición: %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != test.wantCode {
				t.Fatalf("estado = %d, se quiere %d; cuerpo: %s", response.StatusCode, test.wantCode, readBody(t, response))
			}

			if test.wantFields != nil {
				if len(service.updates) != len(test.wantFields) {
					t.Fatalf("campos actualizados = %v, se quiere %v", service.updates, test.wantFields)
				}
			}
		})
	}
}

func TestPatchPerfilInexistente(t *testing.T) {
	server, _ := newTestServer(t)
	id := uuid.NewV7().String()

	request, err := http.NewRequest(http.MethodPatch, server.URL+"/profiles/"+id, strings.NewReader(`{"phone":"+573002223344"}`))
	if err != nil {
		t.Fatalf("creando la petición: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("estado = %d, se quiere 404", response.StatusCode)
	}
}

func TestPatchErrorDePersistencia(t *testing.T) {
	server, service := newTestServer(t)
	p := testProfile(t)
	service.profiles[p.ID.String()] = p
	service.updateErr = errors.New("réplica sin primaria")

	request, err := http.NewRequest(http.MethodPatch, server.URL+"/profiles/"+p.ID.String(), strings.NewReader(`{"phone":"+573002223344"}`))
	if err != nil {
		t.Fatalf("creando la petición: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("estado = %d, se quiere 500", response.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	server, _ := newTestServer(t)

	response, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("error en la petición: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("estado = %d, se quiere 200", response.StatusCode)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("leyendo el cuerpo: %v", err)
	}
	return string(data)
}
