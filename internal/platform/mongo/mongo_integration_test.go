//go:build integration

package mongo

import (
	"context"
	"os"
	"testing"
	"time"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestEnsureIndexesDemoCollections verifica que los índices de la demo existen.
func TestEnsureIndexesDemoCollections(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017/kenko_eps?directConnection=true"
	}
	client, err := mongodriver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("sin mongo local: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("sin mongo local: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	dbName := os.Getenv("MONGO_APP_DB")
	if dbName == "" {
		dbName = "kenko_eps"
	}
	c := &Client{Raw: client, DB: client.Database(dbName)}
	if err := c.EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	// La segunda llamada debe ser idempotente.
	if err := c.EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("EnsureIndexes idempotente: %v", err)
	}
}
