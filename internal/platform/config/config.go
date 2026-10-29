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
	Port           string
	AWSRegion      string
	EventBusName   string
	ProfileQueueURL string

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
		Port:                getenv("PORT", "8080"),
		AWSRegion:           getenv("AWS_REGION", "us-east-1"),
		EventBusName:        getenv("EVENT_BUS_NAME", "kenko-dev-bus"),
		ProfileQueueURL:     strings.TrimSpace(os.Getenv("PROFILE_QUEUE_URL")),
		MongoDB:             mongoDB,
		RelayPollInterval:   5 * time.Second,
		RelayBatchSize:      10,
		RelayClaimLease:     60 * time.Second,
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
	return cfg, nil
}
