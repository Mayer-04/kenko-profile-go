// Package app concentra el cableado compartido (AWS + Mongo + servicio).
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Deps son las dependencias cableadas de cada proceso.
type Deps struct {
	Config  config.Config
	Log     *slog.Logger
	Mongo   *platformmongo.Client
	Service *profile.Service
	AWS     aws.Config
}

// Wire carga entorno, conecta Mongo y construye el servicio.
// Los llamadores cierran Mongo con Disconnect al terminar.
func Wire(ctx context.Context) (*Deps, error) {
	config.LoadEnvFile()
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	log := config.NewLogger()
	mongoClient, err := platformmongo.Connect(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		_ = mongoClient.Raw.Disconnect(context.Background())
		return nil, fmt.Errorf("aws config: %w", err)
	}
	svc := profile.NewService(store.NewMongoStore(mongoClient), log)
	return &Deps{Config: cfg, Log: log, Mongo: mongoClient, Service: svc, AWS: awsCfg}, nil
}

// WireOrExit es el cableado compartido de los tres binarios (api, worker,
// relay). Ante un fallo de init escribe "<name>: init failed: <err>" en
// stderr y termina con código 1, igual que hacían los main por separado.
func WireOrExit(ctx context.Context, name string) *Deps {
	deps, err := Wire(ctx)
	if err != nil {
		// Sin logger aún si falla la config; mensaje a stderr.
		_, _ = os.Stderr.WriteString(name + ": init failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	return deps
}

// NewSQS construye el cliente SQS de consumo (10.3).
func NewSQS(cfg aws.Config) *sqs.Client {
	return sqs.NewFromConfig(cfg)
}

// NewEventBridge construye el cliente de publicación (10.3).
func NewEventBridge(cfg aws.Config) *eventbridge.Client {
	return eventbridge.NewFromConfig(cfg)
}
