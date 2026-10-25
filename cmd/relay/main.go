// Command relay publica en EventBridge los eventos pendientes del outbox
// (sección 6).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awseb "github.com/aws/aws-sdk-go-v2/service/eventbridge"

	"uuid"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/eventbridge"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/logging"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/storage"
	"github.com/Mayer-04/kenko-profile-go/internal/relay"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mongoClient, err := storage.Connect(ctx, cfg.Mongo, logger)
	if err != nil {
		return err
	}
	defer mongoClient.Disconnect(context.Background())

	store := storage.NewStore(mongoClient, cfg.Mongo.Database)
	if err := store.EnsureSchema(ctx, cfg.Relay.OutboxTTLDays); err != nil {
		return err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWS.Region))
	if err != nil {
		logger.Error("fallo al cargar la configuración de AWS", "error", err)
		return err
	}
	publisher := eventbridge.NewPublisher(awseb.NewFromConfig(awsCfg), cfg.AWS.EventBusName)

	r := relay.NewRelay(store, publisher, logger, relay.Config{
		PollInterval: cfg.Relay.PollInterval,
		BatchSize:    cfg.Relay.BatchSize,
		LockTTL:      cfg.Relay.LockTTL,
		Owner:        uuid.NewV7().String(),
	})
	logger.Info("relay iniciado", "bus", cfg.AWS.EventBusName)
	return r.Run(ctx)
}
