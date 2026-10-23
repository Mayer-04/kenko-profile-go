package config

import (
	"testing"
)

func withEnv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv(k, v)
}

func TestURISelectionDockerFirst(t *testing.T) {
	withEnv(t, "MONGODB_URI", "mongodb://user:pass@mongo:27017/db?authSource=db")
	withEnv(t, "MONGO_ATLAS_URI", "mongodb+srv://u:p@host/db?retryWrites=true")
	withEnv(t, "PROFILE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/q")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MongoSource != MongoSourceDocker {
		t.Fatalf("fuente = %q, quiero docker", cfg.MongoSource)
	}
}

func TestURISelectionFallsBackToAtlas(t *testing.T) {
	withEnv(t, "MONGODB_URI", "")
	withEnv(t, "MONGO_ATLAS_URI", "mongodb+srv://u:p@host/db?retryWrites=true")
	withEnv(t, "PROFILE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/q")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MongoSource != MongoSourceAtlas {
		t.Fatalf("fuente = %q, quiero atlas", cfg.MongoSource)
	}
}

func TestURISelectionRejectsMalformed(t *testing.T) {
	withEnv(t, "MONGODB_URI", "postgres://x")
	withEnv(t, "MONGO_ATLAS_URI", "")
	withEnv(t, "PROFILE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/q")
	if _, err := Load(); err == nil {
		t.Fatal("URIs mal formadas deberían fallar")
	}
}
