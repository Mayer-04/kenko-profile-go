package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/labstack/echo/v5"
)

// Handler atiende GET y PATCH de perfiles (sección 9).
type Handler struct {
	svc *profile.Service
}

// NewHandler inyecta el servicio.
func NewHandler(svc *profile.Service) *Handler {
	return &Handler{svc: svc}
}

// GetByID recupera el perfil por su id.
func (h *Handler) GetByID(c *echo.Context) error {
	p, err := h.svc.GetByID(c.Request().Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, profile.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "profile not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	return c.JSON(http.StatusOK, toResponse(p))
}

// Patch modifica phone y address; rechaza cualquier otro campo (9, 4.2).
func (h *Handler) Patch(c *echo.Context) error {
	var raw map[string]any
	dec := json.NewDecoder(c.Request().Body)
	if err := dec.Decode(&raw); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
	}
	in, err := profile.ValidatePatch(raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	p, err := h.svc.Patch(c.Request().Context(), c.Param("id"), in)
	if err != nil {
		switch {
		case errors.Is(err, profile.ErrNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "profile not found"})
		case errors.Is(err, profile.ErrInvalidPatch):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
	return c.JSON(http.StatusOK, toResponse(p))
}
