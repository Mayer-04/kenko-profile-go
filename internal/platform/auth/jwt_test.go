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
	return map[string]any{
		"userId":    uid.String(),
		"firstName": "Maria Fernanda",
		"email":     "maria.gomez@example.com",
		"exp":       time.Now().Add(time.Hour).Unix(),
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
		{"sin exp", func() string { return signJWT(t, secret, noExp) }, secret, ErrInvalidToken},
		{"sin secreto", func() string { return signJWT(t, secret, valid) }, "", ErrInvalidToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateToken(tc.token(), tc.secret)
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
