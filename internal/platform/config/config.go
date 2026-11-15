package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// MongoSource indica qué URI se seleccionó según la regla de 8.3.
type MongoSource string

const (
	MongoSourceDocker MongoSource = "docker"
	MongoSourceAtlas  MongoSource = "atlas"
)

// Config concentra toda la configuración por variables de entorno.
type Config struct {
	Port            string
	AWSRegion       string
	EventBusName    string
	ProfileQueueURL string
	Environment     string

	// CORSAllowedOrigins son los orígenes exactos permitidos (sin `*`).
	// Vacío = CORS deshabilitado (no se envían headers).
	CORSAllowedOrigins []string

	// Documents (certificado de afiliación).
	// Opcionales en Load para no romper worker/relay ni tests existentes;
	// el API de certificates los exige a nivel de arranque de ese binario.
	DocumentsBucketName string
	JWTSecret           string

	// Demo: siembra al arrancar el api (solo desarrollo local).
	SeedDemoData      bool
	DemoUsersFile     string
	DemoExtraProfiles int

	MongoURI    string
	MongoSource MongoSource
	MongoDB     string

	// Relay
	RelayPollInterval time.Duration
	RelayBatchSize    int
	RelayClaimLease   time.Duration

	// Worker
	WorkerWaitTimeSeconds int32
	WorkerMaxMessages     int32
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// DefaultCORSAllowedOrigins son los 4 orígenes de desarrollo del frontend.
var DefaultCORSAllowedOrigins = []string{
	"http://localhost:3000",
	"http://localhost:3001",
	"http://localhost:5000",
	"http://localhost:8080",
}

// currentEnvironment lee APP_ENV con fallback a ENVIRONMENT.
// APP_ENV tiene prioridad para compatibilidad con el brief;
// el repo históricamente usa ENVIRONMENT (ver cmd/api/main.go y .env).
func currentEnvironment() string {
	if v := strings.TrimSpace(os.Getenv("APP_ENV")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("ENVIRONMENT"))
}

// IsProductionEnv reporta si el entorno es producción.
func IsProductionEnv(env string) bool {
	e := strings.ToLower(strings.TrimSpace(env))
	return e == "production" || e == "prod"
}

// normalizeCORSOrigin recorta espacios y barras finales.
// La comparación sigue siendo exacta en esquema, host y puerto.
func normalizeCORSOrigin(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimRight(s, "/")
}

// ParseCORSAllowedOrigins parsea CORS_ALLOWED_ORIGINS (coma-separado).
func ParseCORSAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		n := normalizeCORSOrigin(p)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// resolveCORSAllowedOrigins aplica la regla:
// 1) si CORS_ALLOWED_ORIGINS está definida (tras parsear no vacía o vacía
// explícita) se usa tal cual; 2) si no está definida y el entorno es
// producción, lista vacía (sin headers); 3) si no, los 4 por defecto.
func resolveCORSAllowedOrigins(env string) []string {
	if raw, defined := os.LookupEnv("CORS_ALLOWED_ORIGINS"); defined {
		if parsed := ParseCORSAllowedOrigins(raw); len(parsed) > 0 {
			return parsed
		}
		// Definida pero vacía o solo comas/espacios: denegar todo.
		// Distingue de "no definida". Si solo trae "/" se normaliza a ""
		// y también deniega.
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			// Variable presente pero vacía: cae a la regla por entorno
			// (dev = defaults, prod = vacío) para no romper `VAR=` en local.
		} else {
			return nil
		}
	}
	if IsProductionEnv(env) {
		return nil
	}
	out := make([]string, len(DefaultCORSAllowedOrigins))
	copy(out, DefaultCORSAllowedOrigins)
	return out
}

// demoExtraProfiles lee DEMO_EXTRA_PROFILES (7 por defecto, 0 o más).
func demoExtraProfiles() int {
	v := strings.TrimSpace(os.Getenv("DEMO_EXTRA_PROFILES"))
	if v == "" {
		return 7
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 7
	}
	return n
}

// wellFormedMongoURI implementa el criterio de «disponible» de 8.3:
// no vacía y bien formada (empieza con mongodb:// o mongodb+srv://).
func wellFormedMongoURI(uri string) bool {
	u := strings.TrimSpace(uri)
	return strings.HasPrefix(u, "mongodb://") || strings.HasPrefix(u, "mongodb+srv://")
}

// Load lee el entorno y aplica el orden de conexión de 8.3:
// 1) MONGODB_URI (Docker) si disponible; 2) si no, MONGO_ATLAS_URI.
func Load() (Config, error) {
	// Compat: compose/.env usan MONGO_APP_DATABASE, código histórico usa MONGO_APP_DB.
	mongoDB := getenv("MONGO_APP_DB", "")
	if mongoDB == "" {
		mongoDB = getenv("MONGO_APP_DATABASE", "kenko_eps")
	}
	cfg := Config{
		Port:                  getenv("PORT", "8080"),
		AWSRegion:             getenv("AWS_REGION", "us-east-1"),
		EventBusName:          getenv("EVENT_BUS_NAME", "kenko-dev-bus"),
		ProfileQueueURL:       strings.TrimSpace(os.Getenv("PROFILE_QUEUE_URL")),
		Environment:           currentEnvironment(),
		DocumentsBucketName:   strings.TrimSpace(os.Getenv("DOCUMENTS_BUCKET_NAME")),
		JWTSecret:             strings.TrimSpace(os.Getenv("JWT_SECRET")),
		SeedDemoData:          strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_DEMO_DATA")), "true"),
		DemoUsersFile:         getenv("DEMO_USERS_FILE", "seed/demo_users.json"),
		DemoExtraProfiles:     demoExtraProfiles(),
		MongoDB:               mongoDB,
		RelayPollInterval:     5 * time.Second,
		RelayBatchSize:        10,
		RelayClaimLease:       60 * time.Second,
		WorkerWaitTimeSeconds: 20,
		WorkerMaxMessages:     10,
	}

	if v := strings.TrimSpace(os.Getenv("RELAY_POLL_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.RelayPollInterval = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("RELAY_BATCH_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10 {
			cfg.RelayBatchSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RELAY_CLAIM_LEASE")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.RelayClaimLease = d
		}
	}

	dockerURI := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	atlasURI := strings.TrimSpace(os.Getenv("MONGO_ATLAS_URI"))

	switch {
	case wellFormedMongoURI(dockerURI):
		cfg.MongoURI = dockerURI
		cfg.MongoSource = MongoSourceDocker
	case wellFormedMongoURI(atlasURI):
		cfg.MongoURI = atlasURI
		cfg.MongoSource = MongoSourceAtlas
	default:
		return Config{}, fmt.Errorf("ninguna URI de MongoDB disponible: MONGODB_URI vacía o mal formada y MONGO_ATLAS_URI ausente o mal formada")
	}

	if cfg.ProfileQueueURL == "" {
		return Config{}, fmt.Errorf("PROFILE_QUEUE_URL es obligatoria")
	}
	if cfg.EventBusName == "" {
		return Config{}, fmt.Errorf("EVENT_BUS_NAME es obligatorio")
	}
	cfg.CORSAllowedOrigins = resolveCORSAllowedOrigins(cfg.Environment)
	return cfg, nil
}
