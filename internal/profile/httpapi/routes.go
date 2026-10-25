package httpapi

import "github.com/labstack/echo/v5"

// RegisterRoutes registra los endpoints del servicio (sección 9):
//
//	GET   /healthz        comprueba que el proceso está vivo
//	GET   /profiles/{id}  recupera el perfil por su id
//	PATCH /profiles/{id}  modifica phone y address
func RegisterRoutes(e *echo.Echo, handler *Handler) {
	e.GET("/healthz", handler.Health)
	e.GET("/profiles/:id", handler.Get)
	e.PATCH("/profiles/:id", handler.Update)
}
