package historyapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/auth"
	"github.com/labstack/echo/v5"
)

// Handler atiende POST /medical-history.
type Handler struct {
	usecase   *documents.GenerateHistory
	jwtSecret string
}

// NewHandler inyecta el caso de uso y el secreto JWT.
func NewHandler(usecase *documents.GenerateHistory, jwtSecret string) *Handler {
	return &Handler{usecase: usecase, jwtSecret: jwtSecret}
}

type createResponse struct {
	PdfID       string `json:"pdfId"`
	GeneratedAt string `json:"generatedAt"`
	RecordCount int    `json:"recordCount"`
}

// Create genera el historial médico del titular del token.
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
	cmd := documents.HistoryCommand{
		UserID:        claims.UserID,
		FirstName:     claims.FirstName,
		Email:         claims.Email,
		CorrelationID: correlationID,
	}
	res, err := h.usecase.Execute(ctx, cmd)
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrProfileNotFound),
			errors.Is(err, documents.ErrNoMedicalRecords):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "no medical records"})
		case errors.Is(err, documents.ErrEventPublish):
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "event publish failed"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
	return c.JSON(http.StatusCreated, createResponse{
		PdfID:       res.PdfID,
		GeneratedAt: res.GeneratedAt.UTC().Format(time.RFC3339),
		RecordCount: res.RecordCount,
	})
}

// RegisterRoutes monta POST /medical-history (sin prefijo, como el resto).
func RegisterRoutes(e *echo.Echo, h *Handler) {
	e.POST("/medical-history", h.Create)
}
