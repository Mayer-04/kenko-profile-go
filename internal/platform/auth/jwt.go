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
type Claims struct {
	UserID    string
	FirstName string
	Email     string
}

// ValidateToken valida un JWT HS256 con exp obligatorio y claims
// userId, firstName y email. No registra nada (sin PII en logs).
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
	var payload struct {
		UserID    string `json:"userId"`
		FirstName string `json:"firstName"`
		Email     string `json:"email"`
		Exp       *int64 `json:"exp"`
	}
	dec := json.NewDecoder(strings.NewReader(string(payloadJSON)))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return Claims{}, ErrInvalidToken
	}
	rawBytes, _ := json.Marshal(raw)
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		// exp puede venir como float; reintento manual.
		if !parseClaimsLoose(raw, &payload) {
			return Claims{}, ErrInvalidToken
		}
	}
	// exp puede venir como float64 si el primer parse lo truncó.
	if payload.Exp == nil {
		if !extractExp(raw, &payload) {
			return Claims{}, ErrInvalidToken
		}
	}
	if payload.Exp == nil {
		return Claims{}, ErrInvalidToken
	}
	if time.Now().Unix() >= *payload.Exp {
		return Claims{}, ErrExpiredToken
	}
	userID := strings.TrimSpace(payload.UserID)
	firstName := strings.TrimSpace(payload.FirstName)
	email := strings.TrimSpace(payload.Email)
	if userID == "" || firstName == "" || email == "" {
		return Claims{}, ErrInvalidToken
	}
	if _, err := uuid.FromString(userID); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if !strings.Contains(email, "@") {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: userID, FirstName: firstName, Email: email}, nil
}

func parseClaimsLoose(raw map[string]any, out *struct {
	UserID    string `json:"userId"`
	FirstName string `json:"firstName"`
	Email     string `json:"email"`
	Exp       *int64 `json:"exp"`
},
) bool {
	if v, ok := raw["userId"].(string); ok {
		out.UserID = v
	}
	if v, ok := raw["firstName"].(string); ok {
		out.FirstName = v
	}
	if v, ok := raw["email"].(string); ok {
		out.Email = v
	}
	return extractExp(raw, out)
}

func extractExp(raw map[string]any, out *struct {
	UserID    string `json:"userId"`
	FirstName string `json:"firstName"`
	Email     string `json:"email"`
	Exp       *int64 `json:"exp"`
},
) bool {
	v, ok := raw["exp"]
	if !ok || v == nil {
		return false
	}
	var exp int64
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return false
		}
		exp = i
	case float64:
		exp = int64(n)
	case int64:
		exp = n
	case int:
		exp = int64(n)
	default:
		return false
	}
	out.Exp = &exp
	return true
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
