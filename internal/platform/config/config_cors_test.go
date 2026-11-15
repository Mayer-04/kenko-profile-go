package config

import (
	"os"
	"testing"
)

func clearEnv(k string) error {
	return os.Unsetenv(k)
}

func mustLoadCORS(t *testing.T) Config {
	t.Helper()
	t.Setenv("MONGODB_URI", "mongodb://user:pass@mongo:27017/db?authSource=db")
	t.Setenv("PROFILE_QUEUE_URL", "https://sqs.us-east-1.amazonaws.com/1/q")
	t.Setenv("EVENT_BUS_NAME", "kenko-dev-bus")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestCORSDefaultsEnDesarrollo(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("APP_ENV", "")
	// t.Setenv no permite unset; restauramos manualmente al final.
	if err := clearEnv("CORS_ALLOWED_ORIGINS"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoadCORS(t)
	if len(cfg.CORSAllowedOrigins) != 4 {
		t.Fatalf("dev debería aplicar 4 por defecto, fue %v", cfg.CORSAllowedOrigins)
	}
}

func TestCORSNoDefinidaEnProduccionSinHeaders(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("APP_ENV", "")
	if err := clearEnv("CORS_ALLOWED_ORIGINS"); err != nil {
		t.Fatal(err)
	}
	cfg := mustLoadCORS(t)
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatalf("prod sin variable no debe enviar headers, fue %v", cfg.CORSAllowedOrigins)
	}
}

func TestCORSVariableExplicita(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.kenko.com, https://admin.kenko.com/")
	t.Setenv("ENVIRONMENT", "production")
	cfg := mustLoadCORS(t)
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("debería usar la variable explícita, fue %v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[1] != "https://admin.kenko.com" {
		t.Fatalf("debe recortar barra final, fue %q", cfg.CORSAllowedOrigins[1])
	}
}

func TestParseCORSAllowedOrigins(t *testing.T) {
	got := ParseCORSAllowedOrigins(" http://localhost:3000 ,http://localhost:3000/, ,http://localhost:3001 ")
	if len(got) != 2 || got[0] != "http://localhost:3000" || got[1] != "http://localhost:3001" {
		t.Fatalf("parse con dedupe/trim falló: %v", got)
	}
	if IsProductionEnv("prod") != true || IsProductionEnv("development") != false {
		t.Fatal("IsProductionEnv falló")
	}
}
