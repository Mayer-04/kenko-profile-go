package httpapi

import (
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/labstack/echo/v5"
)

// profileResponse es la forma pública del perfil.
type profileResponse struct {
	ID             string `json:"id"`
	DocumentType   string `json:"document_type"`
	DocumentNumber string `json:"document_number"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	BirthDate      string `json:"birth_date"`
	Gender         string `json:"gender"`
	Email          string `json:"email"`
	EmailVerified  bool   `json:"email_verified"`
	Phone          string `json:"phone"`
	AddressLine    string `json:"address_line"`
	City           string `json:"city"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toResponse(p profile.Profile) profileResponse {
	id, _ := platformmongo.BinaryToUUID(p.ID)
	return profileResponse{
		ID:             id.String(),
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
		CreatedAt:      p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// RegisterRoutes monta GET y PATCH sobre /profiles/:id (D2-B: sin POST).
// Toda modificación usa PATCH, nunca PUT ni POST.
func RegisterRoutes(e *echo.Echo, h *Handler) {
	e.GET("/profiles/:id", h.GetByID)
	e.PATCH("/profiles/:id", h.Patch)
	e.GET("/health", func(c *echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})
}
