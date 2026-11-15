// Package cors implementa el middleware CORS del microservicio (solo CORS,
// sin lógica de negocio). Debe registrarse ANTES que cualquier autenticación
// (e.Pre) para que los preflight OPTIONS respondan 204 sin token y para que
// los errores 401/404/500 también lleven los headers.
package cors

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

const (
	// AllowMethods son los únicos métodos del servicio más OPTIONS.
	AllowMethods = "GET, POST, PATCH, OPTIONS"
	// AllowHeaders son los headers permitidos en preflight.
	AllowHeaders = "Authorization, Content-Type, Accept, X-Correlation-Id"
	// ExposeHeaders se expone para leer el PDF y la correlación.
	ExposeHeaders = "Content-Disposition, X-Correlation-Id"
	// MaxAgeSeconds es el cache del preflight.
	MaxAgeSeconds = "600"
)

// normalize recorta espacios y barras finales para comparar exacto
// esquema+host+puerto sin barra final.
func normalize(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// Middleware construye un middleware CORS sin credenciales y sin `*`.
//   - allowed: lista exacta de orígenes (ej. http://localhost:3000).
//     Vacía/nil = CORS deshabilitado (no se envía ningún header).
//   - Origen permitido: refleja ESE origen en Access-Control-Allow-Origin
//     y agrega Vary: Origin.
//   - Origen no permitido o sin Origin: sin headers, pasa a next.
//   - OPTIONS permitido: 204 con Allow-Methods/Headers y Max-Age sin llamar next
//     (no exige token).
//   - No OPTIONS permitido: fija Allow-Origin/Vary/Expose antes de next para
//     que también los lleven los 401/404/500 del handler.
func Middleware(allowed []string) echo.MiddlewareFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if n := normalize(o); n != "" {
			set[n] = struct{}{}
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			origin := normalize(c.Request().Header.Get("Origin"))
			if origin == "" {
				return next(c)
			}
			if _, ok := set[origin]; !ok {
				return next(c)
			}
			h := c.Response().Header()
			h.Set("Access-Control-Allow-Origin", origin)
			appendVary(h)
			h.Set("Access-Control-Expose-Headers", ExposeHeaders)
			if c.Request().Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", AllowMethods)
				h.Set("Access-Control-Allow-Headers", AllowHeaders)
				h.Set("Access-Control-Max-Age", MaxAgeSeconds)
				return c.NoContent(http.StatusNoContent)
			}
			return next(c)
		}
	}
}

func appendVary(h http.Header) {
	vary := strings.TrimSpace(h.Get("Vary"))
	if vary == "" {
		h.Set("Vary", "Origin")
		return
	}
	for _, p := range strings.Split(vary, ",") {
		if strings.EqualFold(strings.TrimSpace(p), "Origin") {
			return
		}
	}
	h.Set("Vary", vary+", Origin")
}
