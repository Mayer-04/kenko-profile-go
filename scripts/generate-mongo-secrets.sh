#!/bin/sh
set -eu

SECRET_DIR="./secrets"
ADMIN_FILE="${SECRET_DIR}/mongo_root_password.txt"
APP_FILE="${SECRET_DIR}/mongo_app_password.txt"

# No sobrescribir secretos existentes.
if [ -e "$ADMIN_FILE" ] || [ -e "$APP_FILE" ]; then
    printf '%s\n' "Error: ya existe al menos uno de los archivos secretos." >&2
    printf '%s\n' "No se ha modificado ningún archivo." >&2
    exit 1
fi

# Comprobar dependencia.
if ! command -v openssl >/dev/null 2>&1; then
    printf '%s\n' "Error: instala OpenSSL antes de continuar." >&2
    exit 1
fi

# Crear archivos privados por defecto.
umask 077
mkdir -p "$SECRET_DIR"

# Generar ambas contraseñas.
if ! openssl rand -hex 32 > "$ADMIN_FILE"; then
    rm -f "$ADMIN_FILE" "$APP_FILE"
    printf '%s\n' "Error al generar la contraseña de administrador." >&2
    exit 1
fi

if ! openssl rand -hex 32 > "$APP_FILE"; then
    rm -f "$ADMIN_FILE" "$APP_FILE"
    printf '%s\n' "Error al generar la contraseña de aplicación." >&2
    exit 1
fi

chmod 600 "$ADMIN_FILE" "$APP_FILE"

printf '%s\n' "Secretos de MongoDB creados correctamente."
printf 'Administrador: %s\n' "$ADMIN_FILE"
printf 'Aplicación:    %s\n' "$APP_FILE"
printf '%s\n' "Guárdalos de forma segura y no los subas a Git."
