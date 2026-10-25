// Command worker consume la cola SQS de Profile: valida los eventos de
// Authentication y guarda el perfil y el outbox en una transacción (sección 5).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/logging"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/sqs"
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/storage"
	"github.com/Mayer-04/kenko-profile-go/internal/worker"
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
	service := profile.NewService(store)

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWS.Region))
	if err != nil {
		logger.Error("fallo al cargar la configuración de AWS", "error", err)
		return err
	}
	consumer := sqs.NewConsumer(
		awssqs.NewFromConfig(awsCfg),
		cfg.SQS.QueueURL,
		int32(cfg.SQS.MaxMessages),
		int32(cfg.SQS.WaitSeconds),
	)

	w := worker.NewWorker(consumer, service, logger, worker.Config{
		VisibilitySeconds: int32(cfg.SQS.VisibilitySeconds),
		RetryDelay:        5 * time.Second,
	})
	logger.Info("worker iniciado", "queue", cfg.SQS.QueueName)
	return w.Run(ctx)
}
