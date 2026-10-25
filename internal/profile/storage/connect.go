package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
)

// Connect crea el cliente de MongoDB con la URI ya seleccionada (sección 8.3)
// y comprueba la conexión. La URI nunca se registra: contiene credenciales
// (sección 11.3); solo se indica si fue la de Docker o la de Atlas.
func Connect(ctx context.Context, cfg config.Mongo, logger *slog.Logger) (*mongo.Client, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		logger.Error("fallo al crear el cliente de MongoDB", "origen", string(cfg.Source), "error", err)
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		logger.Error("fallo de conexión a MongoDB", "origen", string(cfg.Source), "error", err)
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("conectando a MongoDB (origen %s): %w", cfg.Source, err)
	}

	logger.Info("conexión a MongoDB establecida", "origen", string(cfg.Source), "baseDatos", cfg.Database)
	return client, nil
}
