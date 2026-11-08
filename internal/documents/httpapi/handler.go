package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/auth"
	"github.com/labstack/echo/v5"
)

// Handler atiende POST /certificates.
type Handler struct {
	usecase   *documents.GenerateCertificate
	jwtSecret string
}

// NewHandler inyecta el caso de uso y el secreto JWT.
func NewHandler(usecase *documents.GenerateCertificate, jwtSecret string) *Handler {
	return &Handler{usecase: usecase, jwtSecret: jwtSecret}
}

type createResponse struct {
	CertificateID        string `json:"certificateId"`
	IssuedAt             string `json:"issuedAt"`
	AffiliationStartDate string `json:"affiliationStartDate"`
}

// Create emite el certificado de afiliación del titular del token.
func (h *Handler) Create(c *echo.Context) error {
	ctx := c.Request().Context()

	raw, err := auth.BearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	claims, err := auth.ValidateToken(raw, h.jwtSecret)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	correlationID := strings.TrimSpace(c.Request().Header.Get("X-Correlation-Id"))
	cmd := documents.GenerateCommand{
		UserID:        claims.UserID,
		FirstName:     claims.FirstName,
		Email:         claims.Email,
		CorrelationID: correlationID,
	}
	res, err := h.usecase.Execute(ctx, cmd)
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrProfileNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "profile not found"})
		case errors.Is(err, documents.ErrEventPublish):
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "event publish failed"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
	return c.JSON(http.StatusCreated, createResponse{
		CertificateID:        res.CertificateID,
		IssuedAt:             res.IssuedAt.UTC().Format(time.RFC3339),
		AffiliationStartDate: res.AffiliationStartDate.UTC().Format(time.RFC3339),
	})
}

// RegisterRoutes monta POST /certificates (sin prefijo, como el resto del repo).
func RegisterRoutes(e *echo.Echo, h *Handler) {
	e.POST("/certificates", h.Create)
}
