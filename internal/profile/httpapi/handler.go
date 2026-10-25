// Package httpapi expone la API REST de Profile (sección 9).
package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/profile"
)

// Service es la superficie del dominio que necesita la API.
type Service interface {
	Get(ctx context.Context, id uuid.UUID) (profile.Profile, error)
	Update(ctx context.Context, id uuid.UUID, patch profile.Patch) (profile.Profile, error)
}

// Handler atiende las peticiones HTTP del servicio.
type Handler struct {
	profiles Service
	logger   *slog.Logger
}

// NewHandler crea el handler con sus dependencias inyectadas.
func NewHandler(profiles Service, logger *slog.Logger) *Handler {
	return &Handler{profiles: profiles, logger: logger}
}

// Health responde si el proceso está vivo; lo usa el healthcheck de Compose.
func (h *Handler) Health(c *echo.Context) error {
	return c.NoContent(http.StatusOK)
}

// Get responde con el perfil pedido por id (sección 9).
func (h *Handler) Get(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return badRequest("id inválido: debe ser un UUID")
	}

	p, err := h.profiles.Get(c.Request().Context(), id)
	if errors.Is(err, profile.ErrProfileNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "perfil no encontrado")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, newProfileResponse(p))
}

// Update aplica un PATCH de phone y/o address (sección 4.2). No publica
// eventos (sección 12).
func (h *Handler) Update(c *echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return badRequest("id inválido: debe ser un UUID")
	}

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return badRequest("no se pudo leer el cuerpo de la petición")
	}
	patch, err := parsePatch(body)
	if err != nil {
		return badRequest(err.Error())
	}

	updated, err := h.profiles.Update(c.Request().Context(), id, patch)
	switch {
	case errors.Is(err, profile.ErrProfileNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "perfil no encontrado")
	case errors.Is(err, profile.ErrInvalidPatch):
		return badRequest(err.Error())
	case err != nil:
		h.logger.Error("fallo al actualizar el perfil", "profileId", id, "error", err)
		return err
	}

	h.logger.Info("perfil actualizado", "profileId", id, "campos", patch.FieldNames())
	return c.JSON(http.StatusOK, newProfileResponse(updated))
}

func badRequest(message string) error {
	return echo.NewHTTPError(http.StatusBadRequest, message)
}

// NewErrorHandler responde los errores como JSON y registra los inesperados.
func NewErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		code := http.StatusInternalServerError
		message := "error interno"

		var httpError *echo.HTTPError
		if errors.As(err, &httpError) {
			code = httpError.Code
			if httpError.Message != "" {
				message = httpError.Message
			}
		} else {
			logger.Error("error inesperado en la petición",
				"method", c.Request().Method,
				"path", c.Path(),
				"error", err,
			)
		}
		if err := c.JSON(code, map[string]string{"message": message}); err != nil {
			c.Logger().Error("fallo al escribir la respuesta de error", "error", err)
		}
	}
}
