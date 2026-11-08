// Command seed inserta perfiles y citas ficticias para la demostración.
// Uso: go run ./cmd/seed [--file seed/demo_users.json] [--dry-run] [--reset] [--overwrite-profiles]
// Nunca escribe en Atlas/remoto: solo MongoDB local (Docker).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Mayer-04/kenko-profile-go/internal/platform/config"
	platformmongo "github.com/Mayer-04/kenko-profile-go/internal/platform/mongo"
	"github.com/Mayer-04/kenko-profile-go/internal/seed"
)

func main() {
	file := flag.String("file", "seed/demo_users.json", "ruta al archivo de usuarios")
	dryRun := flag.Bool("dry-run", false, "muestra qué haría sin escribir")
	reset := flag.Bool("reset", false, "borra solo completed_appointments de los usuarios y termina")
	overwrite := flag.Bool("overwrite-profiles", false, "actualiza perfiles existentes")
	flag.Parse()

	if err := run(*file, *dryRun, *reset, *overwrite); err != nil {
		_, _ = os.Stderr.WriteString("seed: " + err.Error() + "\n")
		os.Exit(1)
	}
}

func run(file string, dryRun, reset, overwrite bool) error {
	config.LoadEnvFile()
	logger := config.NewLogger()

	dockerURI := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	atlasURI := strings.TrimSpace(os.Getenv("MONGO_ATLAS_URI"))
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	if env == "production" || env == "prod" {
		return fmt.Errorf("rehusado: ENVIRONMENT es producción; el seed solo corre contra MongoDB local")
	}
	if isAtlas(atlasURI) && !isDocker(dockerURI) {
		return fmt.Errorf("rehusado: la URI resuelta es remota (Atlas); el seed solo corre contra MongoDB local")
	}
	uri := dockerURI
	if !isDocker(uri) {
		return fmt.Errorf("MONGODB_URI vacía o mal formada (debe empezar con mongodb://)")
	}
	dbName := strings.TrimSpace(os.Getenv("MONGO_APP_DB"))
	if dbName == "" {
		dbName = strings.TrimSpace(os.Getenv("MONGO_APP_DATABASE"))
	}
	if dbName == "" {
		dbName = "kenko_eps"
	}
	cfg := config.Config{MongoURI: uri, MongoSource: config.MongoSourceDocker, MongoDB: dbName}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := platformmongo.Connect(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer client.Raw.Disconnect(context.Background()) //nolint:errcheck

	users, err := seed.LoadUsers(file)
	if err != nil {
		return err
	}
	summary, err := seed.Run(ctx, client.DB, users, seed.Options{
		DryRun:            dryRun,
		Reset:             reset,
		OverwriteProfiles: overwrite,
	}, time.Now())
	if err != nil {
		return err
	}
	if reset {
		logger.Info("seed.reset_completado", slog.Int("citas_borradas", summary.AppointmentsGone), slog.Bool("dry_run", dryRun))
		return nil
	}
	logger.Info("seed.completado",
		slog.Int("perfiles_insertados", summary.ProfilesInserted),
		slog.Int("perfiles_actualizados", summary.ProfilesUpdated),
		slog.Int("perfiles_omitidos", summary.ProfilesSkipped),
		slog.Int("citas", summary.AppointmentsSet),
		slog.Bool("dry_run", dryRun),
	)
	return nil
}

// isDocker verifica el prefijo mongodb:// (local).
func isDocker(uri string) bool {
	return strings.HasPrefix(strings.TrimSpace(uri), "mongodb://")
}

// isAtlas verifica el prefijo mongodb+srv:// (remoto).
func isAtlas(uri string) bool {
	return strings.HasPrefix(strings.TrimSpace(uri), "mongodb+srv://")
}
