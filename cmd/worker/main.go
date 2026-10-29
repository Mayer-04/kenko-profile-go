package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mayer-04/kenko-profile-go/internal/app"
	"github.com/Mayer-04/kenko-profile-go/internal/profile/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := app.WireOrExit(ctx, "worker")
	defer deps.Mongo.Raw.Disconnect(context.Background()) //nolint:errcheck

	w := worker.New(
		app.NewSQS(deps.AWS),
		deps.Config.ProfileQueueURL,
		deps.Service,
		deps.Log,
		deps.Config.WorkerWaitTimeSeconds,
		deps.Config.WorkerMaxMessages,
	)
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		deps.Log.Error("worker.stopped_error")
		os.Exit(1)
	}
}
