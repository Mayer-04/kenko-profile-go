package authzapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/auth"
	"github.com/labstack/echo/v5"
)

// Handler atiende GET /authorizations, el PDF y la notificación por correo.
type Handler struct {
	lister    *authz.ListAuthorizations
	pdf       *authz.GetAuthorizationPDF
	notify    *authz.NotifyAuthorization
	jwtSecret string
}

// NewHandler inyecta los casos de uso y el secreto JWT.
func NewHandler(lister *authz.ListAuthorizations, pdf *authz.GetAuthorizationPDF, notify *authz.NotifyAuthorization, jwtSecret string) *Handler {
	return &Handler{lister: lister, pdf: pdf, notify: notify, jwtSecret: jwtSecret}
}

type appointmentSummary struct {
	AppointmentID   string `json:"appointmentId"`
	Date            string `json:"date"`
	AppointmentType string `json:"appointmentType"`
	PhysicianName   string `json:"physicianName"`
}

type authorizationResponse struct {
	AuthorizationID      string             `json:"authorizationId"`
	Type                 string             `json:"type"`
	TypeLabel            string             `json:"typeLabel"`
	Status               string             `json:"status"`
	StatusLabel          string             `json:"statusLabel"`
	RejectionReason      string             `json:"rejectionReason,omitempty"`
	RejectionReasonLabel string             `json:"rejectionReasonLabel,omitempty"`
	CreatedAt            string             `json:"createdAt"`
	Appointment          appointmentSummary `json:"appointment"`
}

func toResponse(item authz.AuthorizationWithAppointment) authorizationResponse {
	resp := authorizationResponse{
		AuthorizationID: item.Authorization.ID,
		Type:            string(item.Authorization.Type),
		TypeLabel:       item.Authorization.Type.Label(),
		Status:          string(item.Authorization.Status),
		StatusLabel:     item.Authorization.Status.Label(),
		CreatedAt:       item.Authorization.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Appointment: appointmentSummary{
			AppointmentID:   item.Appointment.ID,
			Date:            item.Appointment.Date,
			AppointmentType: string(item.Appointment.Type),
			PhysicianName:   item.Appointment.Physician.Name,
		},
	}
	if item.Authorization.Status == documents.AuthorizationStatusRejected {
		resp.RejectionReason = string(item.Authorization.RejectionReason)
		resp.RejectionReasonLabel = item.Authorization.RejectionReason.Label()
	}
	return resp
}

// userIDof valida el JWT y devuelve el userId del token.
func (h *Handler) userIDof(c *echo.Context) (string, bool) {
	claims, ok := h.claimsOf(c)
	if !ok {
		return "", false
	}
	return claims.UserID, true
}

// claimsOf valida el JWT y devuelve sus claims.
func (h *Handler) claimsOf(c *echo.Context) (auth.Claims, bool) {
	raw, err := auth.BearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return auth.Claims{}, false
	}
	claims, err := auth.ValidateToken(raw, h.jwtSecret)
	if err != nil {
		return auth.Claims{}, false
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return auth.Claims{}, false
	}
	return claims, true
}

// List devuelve las autorizaciones del usuario autenticado.
func (h *Handler) List(c *echo.Context) error {
	userID, ok := h.userIDof(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	items, err := h.lister.Execute(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	out := make([]authorizationResponse, 0, len(items))
	for _, it := range items {
		out = append(out, toResponse(it))
	}
	return c.JSON(http.StatusOK, out)
}

// PDF genera el PDF de una autorización del usuario.
func (h *Handler) PDF(c *echo.Context) error {
	userID, ok := h.userIDof(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	res, err := h.pdf.Execute(c.Request().Context(), userID, strings.TrimSpace(c.Param("id")))
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrAuthorizationNotFound),
			errors.Is(err, documents.ErrProfileNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
	c.Response().Header().Set("Content-Type", "application/pdf")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+res.FileName+"\"")
	return c.Blob(http.StatusOK, "application/pdf", res.Bytes)
}

// Notify envía la autorización por correo (genera PDF, sube a S3 y publica).
func (h *Handler) Notify(c *echo.Context) error {
	claims, ok := h.claimsOf(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	res, err := h.notify.Execute(c.Request().Context(), authz.NotifyCommand{
		UserID:          claims.UserID,
		FirstName:       claims.FirstName,
		Email:           claims.Email,
		AuthorizationID: strings.TrimSpace(c.Param("id")),
		CorrelationID:   strings.TrimSpace(c.Request().Header.Get("X-Correlation-Id")),
	})
	if err != nil {
		switch {
		case errors.Is(err, documents.ErrAuthorizationNotFound),
			errors.Is(err, documents.ErrProfileNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		case errors.Is(err, documents.ErrEventPublish):
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "event publish failed"})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"authorizationId": res.AuthorizationID,
		"status":          res.Status,
	})
}

// RegisterRoutes monta GET /authorizations, el PDF y la notificación.
func RegisterRoutes(e *echo.Echo, h *Handler) {
	e.GET("/authorizations", h.List)
	e.GET("/authorizations/:id/pdf", h.PDF)
	e.POST("/authorizations/:id/notify", h.Notify)
}
