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

// Contrato alineado con Auth/Appointments: HS256, iss=kenko.auth,
// aud=kenko-api, secreto compartido, sub UUID v7, exp con leeway de 120s.
//
// Compatibilidad: se siguen aceptando los claims legacy userId, firstName
// y email. Si userId falta, se usa sub como identificador; si firstName o
// email faltan, cada caso de uso los resuelve desde el perfil en Mongo.
const (
	ExpectedIssuer   = "kenko.auth"
	ExpectedAudience = "kenko-api"

	// expLeeway tolera desfases de reloj entre servicios (120s como Auth).
	expLeewaySeconds = 120
)

// Claims mínimos para operar. UserID siempre viene resuelto (claim userId
// o sub); FirstName y Email pueden venir vacíos y se completan desde Mongo.
type Claims struct {
	UserID    string
	FirstName string
	Email     string
}

// ValidateToken valida un JWT HS256 del contrato Auth: firma, iss, aud,
// exp (con leeway) y sub/userId UUID v7. No registra nada (sin PII en logs).
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
	dec := json.NewDecoder(strings.NewReader(string(payloadJSON)))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return Claims{}, ErrInvalidToken
	}

	if !validIssuer(raw) {
		return Claims{}, ErrInvalidToken
	}
	if !validAudience(raw) {
		return Claims{}, ErrInvalidToken
	}
	exp, ok := extractExp(raw)
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	if time.Now().Unix() > exp+expLeewaySeconds {
		return Claims{}, ErrExpiredToken
	}

	userID := firstNonEmpty(stringClaim(raw, "userId"), stringClaim(raw, "sub"))
	if !isUUIDv7(userID) {
		return Claims{}, ErrInvalidToken
	}
	email := strings.TrimSpace(stringClaim(raw, "email"))
	if email != "" && !strings.Contains(email, "@") {
		return Claims{}, ErrInvalidToken
	}
	return Claims{
		UserID:    strings.TrimSpace(userID),
		FirstName: strings.TrimSpace(stringClaim(raw, "firstName")),
		Email:     email,
	}, nil
}

// validIssuer acepta kenko.auth (contrato acordado con Appointments/frontend).
// También acepta kenko-auth porque es el default del código de Auth
// (env.ts) y no está confirmado cuál de los dos tiene desplegado;
// la confianza real la da la firma HMAC + aud + sub, no el iss.
func validIssuer(raw map[string]any) bool {
	iss, ok := raw["iss"].(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(iss) {
	case ExpectedIssuer, "kenko-auth":
		return true
	}
	return false
}

// validAudience acepta aud=kenko-api como string o dentro de un arreglo.
func validAudience(raw map[string]any) bool {
	switch aud := raw["aud"].(type) {
	case string:
		return strings.TrimSpace(aud) == ExpectedAudience
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && strings.TrimSpace(s) == ExpectedAudience {
				return true
			}
		}
	}
	return false
}

// stringClaim lee un claim string sin fallar si viene de otro tipo.
func stringClaim(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// isUUIDv7 exige UUID parseable de versión 7 (contrato de Auth).
func isUUIDv7(s string) bool {
	id, err := uuid.FromString(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return id.Version() == 7
}

func extractExp(raw map[string]any) (int64, bool) {
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
