package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/app"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/httpapi"
	"github.com/labstack/echo/v5"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := app.Wire(ctx)
	if err != nil {
		// Sin logger aún si falla la config; mensaje a stderr.
		_, _ = os.Stderr.WriteString("api: init failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer deps.Mongo.Raw.Disconnect(context.Background()) //nolint:errcheck

	e := echo.New()
	httpapi.RegisterRoutes(e, httpapi.NewHandler(deps.Service))

	srv := &http.Server{Addr: ":" + deps.Config.Port, Handler: e}
	errCh := make(chan error, 1)
	go func() {
		deps.Log.Info("server.running", "port", deps.Config.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			deps.Log.Error("server.start_failed")
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		_ = err
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
