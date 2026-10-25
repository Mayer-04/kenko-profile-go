// Package config carga la configuración del servicio desde variables de
// entorno (archivo .env en desarrollo, sección 8.3).
package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// MongoSource indica cuál de las dos URIs de MongoDB se usó (sección 8.3).
type MongoSource string

const (
	MongoDocker MongoSource = "docker"
	MongoAtlas  MongoSource = "atlas"
)

// Config reúne toda la configuración del servicio, agrupada por proceso.
type Config struct {
	Environment string
	Port        string
	LogLevel    string

	AWS   AWS
	Mongo Mongo
	SQS   SQS
	Relay Relay
}

// AWS tiene los datos del bus de eventos y la cuenta de despliegue.
type AWS struct {
	Region       string
	AccountID    string
	EventBusName string
}

// Mongo tiene la URI seleccionada y la base de datos del servicio.
type Mongo struct {
	URI      string
	Source   MongoSource
	Database string
}

// SQS tiene los parámetros del consumo de la cola de Profile (sección 5).
type SQS struct {
	QueueName         string
	QueueURL          string
	MaxMessages       int
	WaitSeconds       int
	VisibilitySeconds int
}

// Relay tiene los parámetros del publicador del outbox (sección 6).
type Relay struct {
	PollInterval  time.Duration
	BatchSize     int
	LockTTL       time.Duration
	OutboxTTLDays int
}

// Load lee la configuración del entorno con valores por defecto razonables
// para el entorno de desarrollo de Kenko.
func Load() (Config, error) {
	// El .env es opcional: en producción las variables llegan del entorno.
	_ = godotenv.Load()

	c := Config{
		Environment: cmp.Or(os.Getenv("ENVIRONMENT"), "development"),
		Port:        cmp.Or(os.Getenv("PORT"), "8080"),
		LogLevel:    cmp.Or(os.Getenv("LOG_LEVEL"), "info"),
		AWS: AWS{
			Region:       cmp.Or(os.Getenv("AWS_REGION"), "us-east-1"),
			AccountID:    strings.TrimSpace(os.Getenv("AWS_ACCOUNT_ID")),
			EventBusName: cmp.Or(os.Getenv("EVENT_BUS_NAME"), "kenko-dev-bus"),
		},
	}

	dockerURI := os.ExpandEnv(os.Getenv("MONGODB_URI"))
	atlasURI := os.ExpandEnv(os.Getenv("MONGO_ATLAS_URI"))
	uri, source, err := selectMongoURI(dockerURI, atlasURI)
	if err != nil {
		return Config{}, err
	}
	c.Mongo = Mongo{
		URI:      uri,
		Source:   source,
		Database: cmp.Or(os.Getenv("MONGO_APP_DATABASE"), "kenko_eps"),
	}

	if err := c.loadSQS(); err != nil {
		return Config{}, err
	}
	if err := c.loadRelay(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) loadSQS() error {
	c.SQS.QueueName = cmp.Or(os.Getenv("SQS_QUEUE_NAME"), "kenko-dev-profile-authentication")
	if c.AWS.AccountID == "" {
		return errors.New("falta AWS_ACCOUNT_ID, necesaria para construir la URL de la cola de SQS")
	}
	// La URL de una cola clásica es determinista a partir de región, cuenta y nombre.
	c.SQS.QueueURL = fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s", c.AWS.Region, c.AWS.AccountID, c.SQS.QueueName)

	maxMessages, err := envInt("SQS_MAX_MESSAGES", 10)
	if err != nil {
		return err
	}
	waitSeconds, err := envInt("SQS_WAIT_SECONDS", 20)
	if err != nil {
		return err
	}
	visibilitySeconds, err := envInt("SQS_VISIBILITY_SECONDS", 30)
	if err != nil {
		return err
	}

	c.SQS.MaxMessages = clamp(maxMessages, 1, 10)
	c.SQS.WaitSeconds = clamp(waitSeconds, 1, 20)
	c.SQS.VisibilitySeconds = clamp(visibilitySeconds, 10, 600)
	return nil
}

func (c *Config) loadRelay() error {
	pollInterval, err := envDuration("RELAY_POLL_INTERVAL", 2*time.Second)
	if err != nil {
		return err
	}
	batchSize, err := envInt("RELAY_BATCH_SIZE", 10)
	if err != nil {
		return err
	}
	lockTTL, err := envDuration("OUTBOX_LOCK_TTL", time.Minute)
	if err != nil {
		return err
	}
	outboxTTLDays, err := envInt("OUTBOX_TTL_DAYS", 7)
	if err != nil {
		return err
	}

	c.Relay.PollInterval = max(pollInterval, 250*time.Millisecond)
	c.Relay.BatchSize = clamp(batchSize, 1, 10)
	c.Relay.LockTTL = max(lockTTL, time.Second)
	c.Relay.OutboxTTLDays = max(outboxTTLDays, 1)
	return nil
}

// selectMongoURI aplica el orden de conexión de la sección 8.3: la URI de
// Docker es la primera opción si está definida y bien formada; si no, se usa
// la de Atlas.
func selectMongoURI(docker, atlas string) (string, MongoSource, error) {
	if isMongoURI(docker) {
		return docker, MongoDocker, nil
	}
	if isMongoURI(atlas) {
		return atlas, MongoAtlas, nil
	}
	return "", "", errors.New("no hay una URI de MongoDB válida: define MONGODB_URI (Docker) o MONGO_ATLAS_URI (Atlas)")
}

// isMongoURI comprueba que la URI no esté vacía y empiece por mongodb:// o
// mongodb+srv://.
func isMongoURI(uri string) bool {
	return strings.HasPrefix(uri, "mongodb://") || strings.HasPrefix(uri, "mongodb+srv://")
}

func envInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s debe ser un número entero: %q", name, raw)
	}
	return value, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s debe ser una duración válida (por ejemplo 2s): %q", name, raw)
	}
	return value, nil
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
