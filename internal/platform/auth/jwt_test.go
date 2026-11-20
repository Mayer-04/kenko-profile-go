package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
)

func signJWT(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(payload)
	payloadSeg := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payloadSeg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payloadSeg + "." + sig
}

func validPayload() map[string]any {
	uid, _ := uuid.NewV7()
	now := time.Now()
	_ = now
	return map[string]any{
		"sub":       uid.String(),
		"iss":       ExpectedIssuer,
		"aud":       ExpectedAudience,
		"jti":       "01a1f756-0000-7000-8000-000000000001",
		"iat":       time.Now().Unix(),
		"userId":    uid.String(),
		"firstName": "Maria Fernanda",
		"email":     "maria.gomez@example.com",
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
}

func authShapedPayload() map[string]any {
	uid, _ := uuid.NewV7()
	return map[string]any{
		// Forma exacta de Auth (jwt-signer.ts): sin userId/firstName/email.
		"sub": uid.String(),
		"iss": ExpectedIssuer,
		"aud": ExpectedAudience,
		"jti": "01a1f756-0000-7000-8000-000000000002",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func TestValidateTokenOK(t *testing.T) {
	secret := "test-secret"
	payload := validPayload()
	token := signJWT(t, secret, payload)
	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("válido rechazado: %v", err)
	}
	if claims.UserID != payload["userId"] || claims.Email != payload["email"] {
		t.Fatalf("claims incorrectos: %+v", claims)
	}
}

func TestValidateTokenAuthShapedOK(t *testing.T) {
	// Token con forma exacta de Auth: sub/iss/aud/jti/iat/exp, sin claims legacy.
	secret := "test-secret"
	payload := authShapedPayload()
	token := signJWT(t, secret, payload)
	claims, err := ValidateToken(token, secret)
	if err != nil {
		t.Fatalf("token forma Auth rechazado: %v", err)
	}
	if claims.UserID != payload["sub"] {
		t.Fatalf("sub no usado como userId: %+v", claims)
	}
	if claims.FirstName != "" || claims.Email != "" {
		t.Fatalf("nombre/correo deberían venir vacíos: %+v", claims)
	}
}

func TestValidateTokenTable(t *testing.T) {
	secret := "test-secret"
	valid := validPayload()
	expired := validPayload()
	expired["exp"] = time.Now().Add(-time.Hour).Unix()
	noExp := validPayload()
	delete(noExp, "exp")

	cases := []struct {
		name    string
		token   func() string
		secret  string
		wantErr error
	}{
		{"vacío", func() string { return "" }, secret, ErrMissingToken},
		{"malformado", func() string { return "a.b" }, secret, ErrInvalidToken},
		{"firma mala", func() string { return signJWT(t, "otro", valid) }, secret, ErrInvalidToken},
		{"expirado", func() string { return signJWT(t, secret, expired) }, secret, ErrExpiredToken},
		{"expirado dentro del leeway", func() string {
			p := validPayload()
			p["exp"] = time.Now().Add(-60 * time.Second).Unix()
			return signJWT(t, secret, p)
		}, secret, nil},
		{"sin exp", func() string { return signJWT(t, secret, noExp) }, secret, ErrInvalidToken},
		{"sin secreto", func() string { return signJWT(t, secret, valid) }, "", ErrInvalidToken},
		{"iss distinto", func() string {
			p := validPayload()
			p["iss"] = "otro-issuer"
			return signJWT(t, secret, p)
		}, secret, ErrInvalidToken},
		{"iss legacy kenko-auth aceptado", func() string {
			p := authShapedPayload()
			p["iss"] = "kenko-auth"
			return signJWT(t, secret, p)
		}, secret, nil},
		{"aud distinto", func() string {
			p := validPayload()
			p["aud"] = "otra-audiencia"
			return signJWT(t, secret, p)
		}, secret, ErrInvalidToken},
		{"sin sub ni userId", func() string {
			p := validPayload()
			delete(p, "userId")
			delete(p, "sub")
			return signJWT(t, secret, p)
		}, secret, ErrInvalidToken},
		{"sub no-UUID", func() string {
			p := authShapedPayload()
			p["sub"] = "no-es-uuid"
			return signJWT(t, secret, p)
		}, secret, ErrInvalidToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateToken(tc.token(), tc.secret)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("debería pasar: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("debería fallar")
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	token, err := BearerToken("Bearer abc.def.ghi")
	if err != nil || token != "abc.def.ghi" {
		t.Fatalf("bearer válido rechazado: %v", err)
	}
	if _, err := BearerToken(""); err == nil {
		t.Fatal("vacío debería fallar")
	}
	if _, err := BearerToken("Token abc"); err == nil {
		t.Fatal("esquema distinto debería fallar")
	}
}
