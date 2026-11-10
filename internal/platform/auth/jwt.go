package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
)

// Errores de autenticación. El handler los traduce a 401.
var (
	ErrMissingToken = errors.New("missing token")
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

// Claims mínimos que emite Authentication.
// Formato legacy: userId/firstName/email. Formato Auth actual:
// sub (=userId), sid, iss, aud, jti, iat, exp (sin firstName/email;
// esos se toman del snapshot del perfil en Mongo).
type Claims struct {
	UserID    string
	FirstName string
	Email     string
}

// Issuer y audiencia que emite el servicio de authorization.
const (
	ExpectedIssuer   = "kenko.auth"
	ExpectedAudience = "kenko-api"
)

// ValidateToken valida un JWT HS256 con exp obligatorio.
// Acepta dos formatos:
//   - legacy: userId, firstName, email
//   - Auth actual: sub (=userId), iss=kenko.auth, aud=kenko-api
// En formato Auth, firstName/email vienen vacíos y el caller debe
// tomarlos del snapshot del perfil. No registra nada (sin PII en logs).
func ValidateToken(tokenString, secret string) (Claims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return Claims{}, ErrMissingToken
	}
	if strings.TrimSpace(secret) == "" {
		return Claims{}, ErrInvalidToken
	}
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	headerSeg, payloadSeg, sigSeg := parts[0], parts[1], parts[2]

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(headerSeg)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if header.Alg != "HS256" {
		return Claims{}, ErrInvalidToken
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(headerSeg + "." + payloadSeg))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sigSeg)) {
		return Claims{}, ErrInvalidToken
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadSeg)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var raw map[string]any
	dec := json.NewDecoder(strings.NewReader(string(payloadJSON)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return Claims{}, ErrInvalidToken
	}
	exp, ok := rawExp(raw)
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	if time.Now().Unix() >= exp {
		return Claims{}, ErrExpiredToken
	}
	userID := strings.TrimSpace(rawString(raw, "userId"))
	if userID == "" {
		userID = strings.TrimSpace(rawString(raw, "sub"))
	}
	firstName := strings.TrimSpace(rawString(raw, "firstName"))
	email := strings.TrimSpace(rawString(raw, "email"))
	if iss := strings.TrimSpace(rawString(raw, "iss")); iss != "" && iss != ExpectedIssuer {
		return Claims{}, ErrInvalidToken
	}
	if audRaw, present := raw["aud"]; present && audRaw != nil {
		if !validAudience(audRaw) {
			return Claims{}, ErrInvalidToken
		}
	}
	if userID == "" {
		return Claims{}, ErrInvalidToken
	}
	if _, err := uuid.FromString(userID); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if email != "" && !strings.Contains(email, "@") {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: userID, FirstName: firstName, Email: email}, nil
}

func rawString(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func validAudience(v any) bool {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n) == ExpectedAudience
	case []any:
		for _, e := range n {
			if s, _ := e.(string); strings.TrimSpace(s) == ExpectedAudience {
				return true
			}
		}
		return false
	case []string:
		for _, s := range n {
			if strings.TrimSpace(s) == ExpectedAudience {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func rawExp(raw map[string]any) (int64, bool) {
	v, ok := raw["exp"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

// BearerToken extrae el token de "Authorization: Bearer <token>".
func BearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrMissingToken
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "bearer") {
		return "", ErrInvalidToken
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", ErrMissingToken
	}
	return token, nil
}
