package config

import (
	"github.com/joho/godotenv"
)

// LoadEnvFile carga .env solo para desarrollo local. En contenedores las
// variables ya vienen del entorno; si no hay fichero no es un error.
func LoadEnvFile() {
	_ = godotenv.Load()
}
