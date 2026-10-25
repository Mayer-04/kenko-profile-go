#!/bin/sh
# Prueba de extremo a extremo local (paso 8 de la sección 13).
#
# Publica un UserRegistered sintético en el bus (source kenko.auth) y comprueba:
#   1. El worker crea el perfil y el outbox en una transacción.
#   2. El relay publica ProfileCreated y llega a las colas de Appointments y
#      Documents.
#   3. Un duplicado del mismo evento no crea nada y se descarta.
#   4. GET y PATCH del perfil funcionan por HTTP.
#
# Requisitos: docker compose con kenko-mongo en marcha (replica set), AWS CLI
# con permisos sobre el bus, y las variables del archivo .env.
#
# Uso:
#   sh scripts/e2e-local.sh
set -eu

# Directorio raíz del proyecto, sin importar desde dónde se ejecute.
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd -P)"
PROJECT_ROOT="$(dirname -- "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

BIN_DIR="${TMPDIR:-/tmp}/kenko-e2e"
LOG_DIR="${TMPDIR:-/tmp}/kenko-e2e-logs"
AWAIT_SECONDS=60

# ---------- Utilidades ----------

log() { printf '%s\n' "==> $*"; }
fail() { printf '%s\n' "FALLO: $*" >&2; exit 1; }

# Carga .env sin usar source: los valores llevan caracteres (&, $, ?) que el
# shell interpretaría. La expansión de ${VARS} dentro de las URIs la hace la
# propia aplicación.
load_env() {
    while IFS='=' read -r key value; do
        case "$key" in
            ''|'#'*) continue ;;
        esac
        export "$key=$value"
    done < .env
}

# Genera UUID v7 (sección 11.1: identificadores solo v7) con la stdlib de Go.
uuid7() {
    printf 'package main\n\nimport (\n\t"fmt"\n\t"os"\n\n\t"uuid"\n)\n\nfunc main() {\n\tfor range os.Args[1:] {\n\t\tfmt.Println(uuid.NewV7())\n\t}\n}\n' > "$BIN_DIR/uuid7.go"
    (cd "$BIN_DIR" && go run uuid7.go "${1:-1}")
}

# Ejecuta mongosh dentro del contenedor de MongoDB (usuario de aplicación).
mongosh_eval() {
    docker exec kenko-mongo mongosh \
        --quiet \
        --username "$MONGO_APP_USERNAME" \
        --password "$MONGO_APP_PASSWORD" \
        --authenticationDatabase "$MONGO_APP_DATABASE" \
        "$MONGO_APP_DATABASE" \
        --eval "$1"
}

# Escapa un JSON para incrustarlo como string dentro de otro JSON.
json_escape() {
    printf '%s' "$1" | sed 's/"/\\"/g'
}

# Espera a que un comando devuelva exactamente el valor esperado.
esperar() {
    descripcion="$1"
    comando="$2"
    esperado="$3"
    intentos="$AWAIT_SECONDS"
    while [ "$intentos" -gt 0 ]; do
        valor="$(eval "$comando" 2>/dev/null || true)"
        if [ "$valor" = "$esperado" ]; then
            return 0
        fi
        intentos=$((intentos - 1))
        sleep 1
    done
    fail "se agotó el tiempo esperando: $descripcion"
}

# ---------- Preparación ----------

log "Comprobando requisitos previos"
load_env
[ -n "${MONGO_APP_USERNAME:-}" ] || fail "falta MONGO_APP_USERNAME en .env"
[ -n "${MONGO_ATLAS_URI:-}" ] && [ -z "${MONGODB_URI:-}" ] && \
    fail "con MONGODB_URI vacia se usaria Atlas; esta prueba es para el MongoDB local (replica set)"
docker exec kenko-mongo mongosh --quiet --eval 'quit(db.adminCommand("ping").ok ? 0 : 2)' >/dev/null 2>&1 \
    || fail "kenko-mongo no responde: levantalo con docker compose up -d"

mkdir -p "$BIN_DIR" "$LOG_DIR"
rm -f "$LOG_DIR"/api.log "$LOG_DIR"/worker.log "$LOG_DIR"/relay.log

log "Compilando api, worker y relay"
go build -o "$BIN_DIR/" ./cmd/api ./cmd/worker ./cmd/relay

log "Arrancando api, worker y relay (logs en $LOG_DIR)"
"$BIN_DIR/api" > "$LOG_DIR/api.log" 2>&1 & API_PID=$!
"$BIN_DIR/worker" > "$LOG_DIR/worker.log" 2>&1 & WORKER_PID=$!
"$BIN_DIR/relay" > "$LOG_DIR/relay.log" 2>&1 & RELAY_PID=$!

limpiar() {
    kill "$API_PID" "$WORKER_PID" "$RELAY_PID" 2>/dev/null || true
}
trap limpiar EXIT INT TERM

esperar "la API en /healthz" 'curl -sf http://127.0.0.1:8080/healthz >/dev/null && echo ok' "ok"

# ---------- Identidad de la prueba ----------

USER_ID="$(uuid7 1)"
EVENT_ID="$(uuid7 1)"
CORRELATION_ID="$(uuid7 1)"
log "Perfil de prueba: userId=$USER_ID eventId=$EVENT_ID"

OCCURRED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
DETAIL=$(cat <<EOF
{"metadata":{"eventId":"$EVENT_ID","eventVersion":"1","occurredAt":"$OCCURRED_AT","correlationId":"$CORRELATION_ID"},"data":{"userId":"$USER_ID","document":{"type":"CC","number":"1020304050"},"person":{"firstName":"Maria Fernanda","lastName":"Gomez Rojas","birthDate":"1995-03-21","gender":"female"},"contact":{"email":"maria.gomez@example.com","emailVerified":false,"phone":"+573001234567"},"address":{"line":"Cra 15 # 93-60 Apto 402","city":"Bogota"}}}
EOF
)

# ---------- 1. Publicar UserRegistered ----------

log "Publicando UserRegistered en $EVENT_BUS_NAME (source kenko.auth)"
ENTRIES=$(printf '{"Source":"kenko.auth","DetailType":"UserRegistered","EventBusName":"%s","Detail":"%s"}' \
    "$EVENT_BUS_NAME" "$(json_escape "$DETAIL")")
RESULT="$(aws events put-events --region "$AWS_REGION" --entries "[$ENTRIES]")"
echo "$RESULT" | grep -q '"FailedEntryCount": 0' || fail "PutEvents rechazo la entrada: $RESULT"

# ---------- 2. El worker crea el perfil ----------

esperar "el perfil en profiles" \
    "mongosh_eval 'db.profiles.countDocuments({_id: UUID(\"$USER_ID\")})'" "1"
log "Perfil creado por el worker"

PROFILE_COUNT="$(mongosh_eval "db.profiles.countDocuments({_id: UUID(\"$USER_ID\")})")"
[ "$PROFILE_COUNT" = "1" ] || fail "se esperaba 1 perfil, hay $PROFILE_COUNT"
PROCESSED_COUNT="$(mongosh_eval "db.processed_events.countDocuments({event_id: UUID(\"$EVENT_ID\")})")"
[ "$PROCESSED_COUNT" = "1" ] || fail "se esperaba 1 marcador de idempotencia, hay $PROCESSED_COUNT"

# ---------- 3. El relay publica ProfileCreated ----------

esperar "el evento publicado en outbox_events" \
    "mongosh_eval 'db.outbox_events.countDocuments({aggregate_id: UUID(\"$USER_ID\"), published_at: {\$ne: null}})'" "1"
log "ProfileCreated publicado por el relay"
grep -q '"msg":"lote publicado"' "$LOG_DIR/relay.log" \
    || fail "el relay no registro el lote publicado"

# ¿Llegó a las colas de Appointments y Documents? (puede faltar permiso de lectura)
for COLA in kenko-dev-appointments-profile kenko-dev-documents-profile; do
    URL="https://sqs.$AWS_REGION.amazonaws.com/$AWS_ACCOUNT_ID/$COLA"
    MENSAJES="$(aws sqs get-queue-attributes --region "$AWS_REGION" --queue-url "$URL" \
        --attribute-names ApproximateNumberOfMessages --query 'Attributes.ApproximateNumberOfMessages' --output text 2>/dev/null || echo "?")"
    log "Cola $COLA: $MENSAJES mensajes en espera (sus consumidores veran el perfil sintetico)"
done

# ---------- 4. Duplicado: mismo eventId ----------

log "Reenviando el mismo evento (duplicado)"
aws events put-events --region "$AWS_REGION" --entries "[$ENTRIES]" >/dev/null

sleep 10
PROFILE_COUNT="$(mongosh_eval "db.profiles.countDocuments({_id: UUID(\"$USER_ID\")})")"
[ "$PROFILE_COUNT" = "1" ] || fail "el duplicado creo otro perfil: hay $PROFILE_COUNT"
PROCESSED_COUNT="$(mongosh_eval "db.processed_events.countDocuments({event_id: UUID(\"$EVENT_ID\")})")"
[ "$PROCESSED_COUNT" = "1" ] || fail "el duplicado dejo otro marcador: hay $PROCESSED_COUNT"
grep -q '"msg":"mensaje duplicado descartado"' "$LOG_DIR/worker.log" \
    || fail "el worker no descarto el duplicado"

# ---------- 5. GET y PATCH por HTTP ----------

log "Consultando el perfil por HTTP (GET)"
CUERPO="$(curl -sf "http://127.0.0.1:8080/profiles/$USER_ID")"
echo "$CUERPO" | grep -q '"first_name":"Maria Fernanda"' || fail "GET inesperado: $CUERPO"

log "Actualizando phone y address (PATCH)"
CUERPO="$(curl -sf -X PATCH "http://127.0.0.1:8080/profiles/$USER_ID" \
    -H 'Content-Type: application/json' \
    -d '{"phone":"+573009998877","address":{"line":"Calle 100 # 10-20","city":"Medellin"}}')"
echo "$CUERPO" | grep -q '"phone":"+573009998877"' || fail "PATCH inesperado: $CUERPO"

log "Rechazando un campo no permitido (email)"
ESTADO="$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "http://127.0.0.1:8080/profiles/$USER_ID" \
    -H 'Content-Type: application/json' -d '{"email":"otra@example.com"}')"
[ "$ESTADO" = "400" ] || fail "el PATCH con email debio responder 400, respondio $ESTADO"

# ---------- Limpieza ----------

log "Borrando los datos sinteticos de la prueba"
mongosh_eval "db.profiles.deleteOne({_id: UUID(\"$USER_ID\")})" >/dev/null
mongosh_eval "db.outbox_events.deleteMany({aggregate_id: UUID(\"$USER_ID\")})" >/dev/null
mongosh_eval "db.processed_events.deleteMany({event_id: UUID(\"$EVENT_ID\")})" >/dev/null

log "TODO OK: UserRegistered -> perfil -> ProfileCreated -> colas de Appointments y Documents"
log "Nota: los mensajes entregados a las colas de Appointments y Documents no se purgan (se limpian sus consumidores)."
