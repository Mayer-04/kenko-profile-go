package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/app"
	"github.com/Mayer-04/kenko-profile-go/internal/documents/authzapi"
	dochistoryapi "github.com/Mayer-04/kenko-profile-go/internal/documents/historyapi"
	dochttpapi "github.com/Mayer-04/kenko-profile-go/internal/documents/httpapi"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/httpapi"
	"github.com/Mayer-04/kenko-profile-go/internal/seed"
	"github.com/labstack/echo/v5"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := app.WireOrExit(ctx, "api")
	defer deps.Mongo.Raw.Disconnect(context.Background()) //nolint:errcheck

	docHandler, err := app.NewDocumentsHandler(deps)
	if err != nil {
		deps.Log.Error("documents.wire_failed")
		_, _ = os.Stderr.WriteString("api: init failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	historyHandler, err := app.NewHistoryHandler(deps)
	if err != nil {
		deps.Log.Error("history.wire_failed")
		_, _ = os.Stderr.WriteString("api: init failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	authzHandler, err := app.NewAuthzHandler(deps)
	if err != nil {
		deps.Log.Error("authz.wire_failed")
		_, _ = os.Stderr.WriteString("api: init failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	seedDemoData(ctx, deps)

	e := echo.New()
	httpapi.RegisterRoutes(e, httpapi.NewHandler(deps.Service))
	dochttpapi.RegisterRoutes(e, docHandler)
	dochistoryapi.RegisterRoutes(e, historyHandler)
	authzapi.RegisterRoutes(e, authzHandler)

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

// seedDemoData siembra citas y autorizaciones solo si SEED_DEMO_DATA=true.
// Se niega en producción; si el archivo no es válido o la siembra falla,
// registra un aviso y deja que el servidor arranque igual.
func seedDemoData(ctx context.Context, deps *app.Deps) {
	if !deps.Config.SeedDemoData {
		return
	}
	if env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT"))); env == "production" || env == "prod" {
		deps.Log.Warn("seed.omitido_produccion")
		return
	}
	file := strings.TrimSpace(deps.Config.DemoUsersFile)
	if file == "" {
		file = "seed/demo_users.json"
	}
	users, err := seed.LoadUsers(file)
	if err != nil {
		deps.Log.Warn("seed.archivo_invalido", "error", err.Error())
		return
	}
	summary, err := seed.Run(ctx, deps.Mongo.DB, users, seed.Options{ExtraProfiles: deps.Config.DemoExtraProfiles}, time.Now())
	if err != nil {
		deps.Log.Warn("seed.fallo_siembra", "error", err.Error())
		return
	}
	deps.Log.Info("seed.completado",
		"citas", summary.AppointmentsSet,
		"autorizaciones", summary.AuthorizationsSet,
		"perfiles_relleno", summary.ExtraProfiles,
	)
}
