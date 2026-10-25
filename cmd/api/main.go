// Command api arranca la API REST de Profile (sección 9).
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	"github.com/Mayer-04/kenko-profile-go/internal/platform/logging"
	"github.com/Mayer-04/kenko-profile-go/internal/profile"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/httpapi"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/storage"
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
	logger.Info("iniciando la API de Profile", "environment", cfg.Environment)

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

	e := echo.New()
	e.Logger = logger
	e.Use(middleware.Recover())
	e.HTTPErrorHandler = httpapi.NewErrorHandler(logger)
	httpapi.RegisterRoutes(e, httpapi.NewHandler(service, logger))

	addr := net.JoinHostPort("", cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("error al iniciar el servidor", "addr", addr, "error", err)
		return err
	}
	server := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	logger.Info("servidor HTTP en ejecución", "addr", listener.Addr().String())

	select {
	case err := <-serverErr:
		logger.Error("error del servidor HTTP", "error", err)
		return err
	case <-ctx.Done():
		logger.Info("apagando el servidor HTTP")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
