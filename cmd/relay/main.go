package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mayer-04/kenko-profile-go/internal/app"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/relay"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := app.Wire(ctx)
	if err != nil {
		_, _ = os.Stderr.WriteString("relay: init failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer deps.Mongo.Raw.Disconnect(context.Background()) //nolint:errcheck

	r := relay.New(
		deps.Service.StoreForRelay(),
		app.NewEventBridge(deps.AWS),
		deps.Config.EventBusName,
		deps.Log,
		deps.Config.RelayPollInterval,
		deps.Config.RelayBatchSize,
		deps.Config.RelayClaimLease,
	)
	if err := r.Run(ctx); err != nil && err != context.Canceled {
		deps.Log.Error("relay.stopped_error")
		os.Exit(1)
	}
}
