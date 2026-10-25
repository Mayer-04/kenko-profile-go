package httpapi

import (
	"errors"
	"fmt"
	"strings"
	"time"

	json "encoding/json/v2"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// ProfileResponse refleja el documento profiles con los nombres de la sección
// 7.1. El id es el UUID del usuario de Authentication.
type ProfileResponse struct {
	ID             string    `json:"id"`
	DocumentType   string    `json:"document_type"`
	DocumentNumber string    `json:"document_number"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	BirthDate      string    `json:"birth_date"`
	Gender         string    `json:"gender"`
	Email          string    `json:"email"`
	EmailVerified  bool      `json:"email_verified"`
	Phone          string    `json:"phone"`
	AddressLine    string    `json:"address_line"`
	City           string    `json:"city"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func newProfileResponse(p profile.Profile) ProfileResponse {
	return ProfileResponse{
		ID:             p.ID.String(),
		DocumentType:   p.DocumentType,
		DocumentNumber: p.DocumentNumber,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		BirthDate:      p.BirthDate.Format("2006-01-02"),
		Gender:         p.Gender,
		Email:          p.Email,
		EmailVerified:  p.EmailVerified,
		Phone:          p.Phone,
		AddressLine:    p.AddressLine,
		City:           p.City,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

// patchRequest solo admite phone y address (sección 9); cualquier otro campo
// —incluidos email y password— se rechaza con 400. email y password se
// cambian en Authentication (sección 4.2).
type patchRequest struct {
	Phone   *string       `json:"phone"`
	Address *patchAddress `json:"address"`
}

type patchAddress struct {
	Line string `json:"line"`
	City string `json:"city"`
}

// parsePatch valida el cuerpo del PATCH contra la lista de campos admitidos y
// lo convierte en un parche del dominio.
func parsePatch(body []byte) (profile.Patch, error) {
	var request patchRequest
	err := json.Unmarshal(body, &request, json.RejectUnknownMembers(true))
	if err != nil {
		return profile.Patch{}, patchParseError(err)
	}
	if request.Phone == nil && request.Address == nil {
		return profile.Patch{}, errors.New("el parche está vacío: incluye phone o address")
	}

	patch := profile.Patch{Phone: request.Phone}
	if request.Address != nil {
		patch.Address = &profile.Address{
			Line: request.Address.Line,
			City: request.Address.City,
		}
	}
	return patch, nil
}

// patchParseError convierte los errores de json/v2 en mensajes claros. El
// JSONPointer indica el campo problemático, tanto si está prohibido como si
// tiene un tipo incorrecto.
func patchParseError(err error) error {
	var semantic *json.SemanticError
	if errors.As(err, &semantic) && semantic.JSONPointer != "" {
		field := strings.TrimPrefix(string(semantic.JSONPointer), "/")
		return fmt.Errorf("campo no admitido o de tipo incorrecto: %s", field)
	}
	return errors.New("cuerpo del PATCH inválido: debe ser un objeto JSON con phone y/o address")
}
