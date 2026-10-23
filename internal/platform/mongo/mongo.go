// Package mongo gestiona la conexión a MongoDB y la creación de
// colecciones/índices de Profile (sección 8).
package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Collections centraliza los nombres de colección.
const (
	ProfilesCollection   = "profiles"
	OutboxCollection     = "outbox_events"
	ProcessedCollection  = "processed_events"
)

// Client envuelve el cliente y la base de datos de la aplicación.
type Client struct {
	Raw *mongodriver.Client
	DB  *mongodriver.Database
}

// Connect abre la conexión, verifica con ping y asegura índices.
// Registra solo la fuente (docker/atlas), nunca la URI (8.3).
func Connect(ctx context.Context, cfg config.Config, log *slog.Logger) (*Client, error) {
	client, err := mongodriver.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Error("mongo.connect_failed", "mongo_source", string(cfg.MongoSource))
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		log.Error("mongo.ping_failed", "mongo_source", string(cfg.MongoSource))
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	c := &Client{Raw: client, DB: client.Database(cfg.MongoDB)}
	if err := c.EnsureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	log.Info("mongo.connected", "mongo_source", string(cfg.MongoSource), "db", cfg.MongoDB)
	return c, nil
}

// EnsureIndexes crea validadores e índices de 8.2. Es idempotente.
func (c *Client) EnsureIndexes(ctx context.Context) error {
	// processed_events.event_id único (idempotencia D4-A).
	if _, err := c.DB.Collection(ProcessedCollection).Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "event_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_event_id"),
	}); err != nil {
		return fmt.Errorf("index processed_events: %w", err)
	}
	// outbox: pendientes por publicar + reclamo (D5-A) y TTL de limpieza (D6-A).
	outbox := c.DB.Collection(OutboxCollection)
	if _, err := outbox.Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "published_at", Value: 1}, {Key: "claimed_until", Value: 1}},
		Options: options.Index().SetName("pending_claim"),
	}); err != nil {
		return fmt.Errorf("index outbox pending: %w", err)
	}
	if _, err := outbox.Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "aggregate_id", Value: 1}},
		Options: options.Index().SetName("by_aggregate"),
	}); err != nil {
		return fmt.Errorf("index outbox aggregate: %w", err)
	}
	// event_id único: un outbox por evento publicado.
	if _, err := outbox.Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "event_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_event_id"),
	}); err != nil {
		return fmt.Errorf("index outbox event_id: %w", err)
	}
	// TTL 30 días sobre published_at: solo borra publicados (D6-A).
	// Los pendientes tienen published_at nulo y el TTL los ignora.
	if _, err := outbox.Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "published_at", Value: 1}},
		Options: options.Index().SetName("ttl_published").SetExpireAfterSeconds(30 * 24 * 3600),
	}); err != nil {
		return fmt.Errorf("index outbox ttl: %w", err)
	}
	// profiles: unicidad de documento para detectar duplicados lógicos.
	if _, err := c.DB.Collection(ProfilesCollection).Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys:    bson.D{{Key: "document_number", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_document_number"),
	}); err != nil {
		return fmt.Errorf("index profiles document: %w", err)
	}
	return nil
}

// UUIDToBinary convierte un UUID v7 a UUID BSON estándar (subtipo 0x04).
func UUIDToBinary(id uuid.UUID) bson.Binary {
	return bson.Binary{Subtype: 0x04, Data: id.Bytes()}
}

// BinaryToUUID convierte un UUID BSON a uuid.UUID.
func BinaryToUUID(b bson.Binary) (uuid.UUID, error) {
	return uuid.FromBytes(b.Data)
}

// MustUUID7 genera un UUID v7 o falla (11.1: solo UUID v7).
func MustUUID7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(fmt.Sprintf("uuid v7: %v", err))
	}
	return id
}
