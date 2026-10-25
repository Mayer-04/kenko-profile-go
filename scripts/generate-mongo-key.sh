#!/bin/sh
set -eu

# Directorio raíz del proyecto, independientemente
# del directorio desde el que se ejecute el script.
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd -P)"
PROJECT_ROOT="$(dirname -- "$SCRIPT_DIR")"

MONGO_DIR="$PROJECT_ROOT/mongo"
KEY_FILE="$MONGO_DIR/replica.key"

# Comprueba que OpenSSL esté disponible.
if ! command -v openssl >/dev/null 2>&1; then
  echo "Error: OpenSSL no está instalado o no está disponible en PATH." >&2
  exit 1
fi

# Comprueba que el directorio mongo exista.
if [ ! -d "$MONGO_DIR" ]; then
  echo "Error: no existe el directorio $MONGO_DIR" >&2
  exit 1
fi

# No sobrescribe una clave existente.
if [ -e "$KEY_FILE" ]; then
  echo "Error: ya existe $KEY_FILE. No se sobrescribirá." >&2
  echo "MongoDB podría estar usando esa clave actualmente." >&2
  exit 1
fi

# Genera una clave aleatoria y restringe sus permisos.
# El subshell limita el umask a este bloque.
(
  umask 077
  openssl rand -base64 756 > "$KEY_FILE"
) || {
  rm -f "$KEY_FILE"
  echo "Error: no se pudo generar la clave." >&2
  exit 1
}
chmod 400 "$KEY_FILE"

echo "Clave generada correctamente: $KEY_FILE"
echo "Permisos configurados: 400"
echo "Recuerda no subir esta clave al repositorio."