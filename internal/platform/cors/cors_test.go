package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

var devOrigins = []string{
	"http://localhost:3000",
	"http://localhost:3001",
	"http://localhost:5000",
	"http://localhost:8080",
}

func newEchoWithCORS(allowed []string) *echo.Echo {
	e := echo.New()
	e.Pre(Middleware(allowed))
	e.GET("/profiles/:id", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/unauthorized", func(c *echo.Context) error {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})
	e.GET("/boom", func(c *echo.Context) error {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	})
	return e
}

func doRequest(t *testing.T, e *echo.Echo, method, path, origin, reqMethod, reqHeaders string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if reqMethod != "" {
		req.Header.Set("Access-Control-Request-Method", reqMethod)
	}
	if reqHeaders != "" {
		req.Header.Set("Access-Control-Request-Headers", reqHeaders)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestPreflightCadaOrigenPermitido(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	for _, origin := range devOrigins {
		rec := doRequest(t, e, http.MethodOptions, "/profiles/123", origin, "GET", "authorization")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("origen %s: OPTIONS debería ser 204, fue %d", origin, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("origen %s: Allow-Origin = %q, quiero %q", origin, got, origin)
		}
		if vary := rec.Header().Get("Vary"); vary != "Origin" {
			t.Fatalf("origen %s: Vary = %q, quiero %q", origin, vary, "Origin")
		}
		if got := rec.Header().Get("Access-Control-Allow-Methods"); got != AllowMethods {
			t.Fatalf("Allow-Methods = %q, quiero %q", got, AllowMethods)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); got != AllowHeaders {
			t.Fatalf("Allow-Headers = %q, quiero %q", got, AllowHeaders)
		}
		if got := rec.Header().Get("Access-Control-Max-Age"); got != MaxAgeSeconds {
			t.Fatalf("Max-Age = %q, quiero %q", got, MaxAgeSeconds)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Fatalf("no debe enviar Credentials, fue %q", got)
		}
	}
}

func TestOrigenNoPermitidoSinHeaders(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	denied := []string{
		"http://localhost:4000",
		"http://127.0.0.1:3000",
		"https://frontend.example.com",
		"http://localhost:3000.evil.com",
	}
	for _, origin := range denied {
		rec := doRequest(t, e, http.MethodOptions, "/profiles/123", origin, "GET", "authorization")
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("origen %s: no debe enviar Allow-Origin, fue %q", origin, got)
		}
		if got := rec.Header().Get("Vary"); got != "" {
			t.Fatalf("origen %s: no debe enviar Vary, fue %q", origin, got)
		}
		rec2 := doRequest(t, e, http.MethodGet, "/profiles/123", origin, "", "")
		if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("GET origen %s: no debe enviar Allow-Origin, fue %q", origin, got)
		}
	}
}

func TestPeticionRealConOrigenPermitido(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	rec := doRequest(t, e, http.MethodGet, "/profiles/123", "http://localhost:3000", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET debería ser 200, fue %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("Allow-Origin = %q, quiero el origen exacto", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, quiero Origin", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != ExposeHeaders {
		t.Fatalf("Expose = %q, quiero %q", got, ExposeHeaders)
	}
}

func TestError401LlevaCORS(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	rec := doRequest(t, e, http.MethodGet, "/unauthorized", "http://localhost:5000", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("debería ser 401, fue %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5000" {
		t.Fatalf("401 debe llevar Allow-Origin, fue %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("401 debe llevar Vary, fue %q", got)
	}
}

func TestError500LlevaCORS(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	rec := doRequest(t, e, http.MethodGet, "/boom", "http://localhost:8080", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("debería ser 500, fue %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8080" {
		t.Fatalf("500 debe llevar Allow-Origin, fue %q", got)
	}
}

func TestNotFoundLlevaCORSConPre(t *testing.T) {
	// e.Pre garantiza que incluso el 404 del router lleve CORS.
	e := newEchoWithCORS(devOrigins)
	rec := doRequest(t, e, http.MethodGet, "/ruta-que-no-existe", "http://localhost:3001", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("debería ser 404, fue %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3001" {
		t.Fatalf("404 debe llevar Allow-Origin, fue %q", got)
	}
}

func TestOptionsNoExigeToken(t *testing.T) {
	e := echo.New()
	e.Pre(Middleware(devOrigins))
	// Handler protegido que exigiría token; el preflight no debe llegar a él.
	e.GET("/authorizations", func(c *echo.Context) error {
		if c.Request().Header.Get("Authorization") == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		return c.JSON(http.StatusOK, []string{})
	})
	rec := doRequest(t, e, http.MethodOptions, "/authorizations", "http://localhost:3000", "GET", "authorization")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS sin token debería ser 204, fue %d", rec.Code)
	}
}

func TestDeshabilitadoSinHeaders(t *testing.T) {
	e := newEchoWithCORS(nil)
	rec := doRequest(t, e, http.MethodOptions, "/profiles/123", "http://localhost:3000", "GET", "authorization")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS deshabilitado no debe enviar headers, fue %q", got)
	}
	rec2 := doRequest(t, e, http.MethodGet, "/profiles/123", "http://localhost:3000", "", "")
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS deshabilitado no debe enviar headers en GET, fue %q", got)
	}
}

func TestNuncaWildcard(t *testing.T) {
	e := newEchoWithCORS(devOrigins)
	rec := doRequest(t, e, http.MethodGet, "/profiles/123", "http://localhost:3000", "", "")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("nunca debe usar `*`")
	}
}
