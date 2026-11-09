// Package app concentra el cableado compartido (AWS + Mongo + servicio).
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authz"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authzapi"
	doceventbridge "github.com/Mayer-04/kenko-profile-go/internal/documents/eventbridge"
	dochistoryapi "github.com/Mayer-04/kenko-profile-go/internal/documents/historyapi"
	dochistorypdf "github.com/Mayer-04/kenko-profile-go/internal/documents/historypdf"
	dochttpapi "github.com/Mayer-04/kenko-profile-go/internal/documents/httpapi"
	docpdf "github.com/Mayer-04/kenko-profile-go/internal/documents/pdf"
	docstore "github.com/Mayer-04/kenko-profile-go/internal/documents/store"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	platformevents "github.com/Mayer-04/kenko-profile-go/internal/platform/events"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	platformstorage "github.com/Mayer-04/kenko-profile-go/internal/platform/storage"
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

// NewDocumentsHandler cablea el feature de certificados (solo api).
// Exige DOCUMENTS_BUCKET_NAME y JWT_SECRET; worker/relay no lo llaman.
func NewDocumentsHandler(deps *Deps) (*dochttpapi.Handler, error) {
	if deps.Config.DocumentsBucketName == "" {
		return nil, fmt.Errorf("DOCUMENTS_BUCKET_NAME es obligatorio para certificates")
	}
	if deps.Config.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET es obligatorio para certificates")
	}
	mongoStore := docstore.NewMongoStore(deps.Mongo)
	pdfGen := docpdf.NewGenerator()
	s3Storage := platformstorage.NewS3Storage(platformstorage.NewClient(deps.AWS), deps.Config.DocumentsBucketName)
	ebPublisher := platformevents.NewPublisher(NewEventBridge(deps.AWS), deps.Config.EventBusName, "kenko.profile")
	adapter := doceventbridge.NewAdapter(ebPublisher)
	usecase := documents.NewGenerateCertificate(
		mongoStore, mongoStore, pdfGen, s3Storage, adapter,
		documents.SystemClock{}, documents.UUID7Generator{},
		deps.Config.DocumentsBucketName, deps.Log,
	)
	return dochttpapi.NewHandler(usecase, deps.Config.JWTSecret), nil
}

// NewHistoryHandler cablea el feature de historial médico (solo api).
// Exige DOCUMENTS_BUCKET_NAME y JWT_SECRET; worker/relay no lo llaman.
func NewHistoryHandler(deps *Deps) (*dochistoryapi.Handler, error) {
	if deps.Config.DocumentsBucketName == "" {
		return nil, fmt.Errorf("DOCUMENTS_BUCKET_NAME es obligatorio para medical history")
	}
	if deps.Config.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET es obligatorio para medical history")
	}
	mongoStore := docstore.NewMongoStore(deps.Mongo)
	pdfGen := dochistorypdf.NewGenerator()
	s3Storage := platformstorage.NewS3Storage(platformstorage.NewClient(deps.AWS), deps.Config.DocumentsBucketName)
	ebPublisher := platformevents.NewPublisher(NewEventBridge(deps.AWS), deps.Config.EventBusName, "kenko.profile")
	adapter := doceventbridge.NewAdapter(ebPublisher)
	usecase := documents.NewGenerateHistory(
		mongoStore, mongoStore, mongoStore, pdfGen, s3Storage, adapter,
		documents.SystemClock{}, documents.UUID7Generator{},
		deps.Config.DocumentsBucketName, deps.Log,
	)
	return dochistoryapi.NewHandler(usecase, deps.Config.JWTSecret), nil
}

// NewAuthzHandler cablea el feature de autorizaciones (solo api).
// Exige DOCUMENTS_BUCKET_NAME y JWT_SECRET; worker/relay no lo llaman.
func NewAuthzHandler(deps *Deps) (*authzapi.Handler, error) {
	if deps.Config.DocumentsBucketName == "" {
		return nil, fmt.Errorf("DOCUMENTS_BUCKET_NAME es obligatorio para authorizations")
	}
	if deps.Config.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET es obligatorio para authorizations")
	}
	mongoStore := docstore.NewMongoStore(deps.Mongo)
	pdfGen := docpdf.NewGenerator()
	s3Storage := platformstorage.NewS3Storage(platformstorage.NewClient(deps.AWS), deps.Config.DocumentsBucketName)
	ebPublisher := platformevents.NewPublisher(NewEventBridge(deps.AWS), deps.Config.EventBusName, "kenko.profile")
	adapter := doceventbridge.NewAdapter(ebPublisher)
	lister := authz.NewListAuthorizations(mongoStore)
	getter := authz.NewGetAuthorizationPDF(mongoStore, mongoStore, pdfGen)
	notifier := authz.NewNotifyAuthorization(
		mongoStore, mongoStore, pdfGen, s3Storage, mongoStore, adapter,
		documents.SystemClock{}, documents.UUID7Generator{},
		deps.Config.DocumentsBucketName, deps.Log,
	)
	return authzapi.NewHandler(lister, getter, notifier, deps.Config.JWTSecret), nil
}
