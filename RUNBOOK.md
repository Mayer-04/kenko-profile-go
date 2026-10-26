# Kenko Profile — Guía de ejecución

## 1. Qué hace

Microservicio Profile (EPS Kenko):

* Guarda la proyección del usuario (`_id = userId` de Auth) en MongoDB.
* Expone API HTTP: `GET /profiles/{id}`, `PATCH /profiles/{id}`, `GET /health`.
* Consume eventos de **Authentication** por SQS y actualiza la proyección.
* Publica `ProfileCreated` por Outbox + Relay. El cambio de email **no** publica nada.

## 2. Ponerlo en marcha

Requisitos: Docker, Go 1.27, credenciales AWS en `~/.aws` (solo lectura) o por entorno.

```bash
cp .env.example .env   # completar MONGO_* y revisar PORT, AWS_REGION, EVENT_BUS_NAME, PROFILE_QUEUE_URL
docker compose up -d --build   # mongo + api + worker + relay
curl http://127.0.0.1:8080/health
# esperado: {"status":"ok"}
```

Útil:

```bash
docker compose logs -f api worker relay
docker compose ps
docker compose down
go tool air          # solo API local con recarga (puerto 8080)
go test ./...       # tests
```

Config clave (`.env.example`, `internal/platform/config/config.go`):

* `PORT=8080`, `AWS_REGION=us-east-1`, `EVENT_BUS_NAME=kenko-dev-bus`
* `PROFILE_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/137696817246/kenko-dev-profile-authentication`
* Mongo: `MONGODB_URI` (Docker, prioritaria si empieza con `mongodb://`) o `MONGO_ATLAS_URI`. Nunca se loguea la URI.
* Relay opcional: `RELAY_POLL_INTERVAL=5s`, `RELAY_BATCH_SIZE=10`, `RELAY_CLAIM_LEASE=1m`.
* Contenedores corren con `UID:GID` y montan `~/.aws:/aws:ro`.

## 3. Endpoints (sin auth, solo `Content-Type: application/json`)

### `GET /health`

```bash
curl -i http://127.0.0.1:8080/health
# 200 {"status":"ok"}
```

### `GET /profiles/{id}`

* `id`: UUID del usuario en Auth (UUID v7). UUID inválido o inexistente → `404`.
* Respuesta `200`:

```json
{
  "id": "<userId>",
  "document_type": "CC",
  "document_number": "1020304050",
  "first_name": "Maria Fernanda",
  "last_name": "Gomez Rojas",
  "birth_date": "1995-03-21",
  "gender": "female",
  "email": "next@example.com",
  "email_verified": false,
  "phone": "+573001234567",
  "address_line": "Cra 15 # 93-60",
  "city": "Bogota",
  "created_at": "2026-10-26T10:00:00Z",
  "updated_at": "2026-10-26T10:00:00Z"
}
```

```bash
curl http://127.0.0.1:8080/profiles/<userId>
curl -i http://127.0.0.1:8080/profiles/no-uuid   # 404 {"error":"profile not found"}
```

### `PATCH /profiles/{id}`

Solo `phone` y/o `address` completo. Rechaza `email`, `password` u otros → `400`. No publica eventos.

Body permitido:

```json
{
  "phone": "+573009998877",
  "address": {"line": "Calle 10 # 5-20", "city": "Medellin"}
}
```

```bash
curl -X PATCH http://127.0.0.1:8080/profiles/<userId> \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+573009998877","address":{"line":"Calle 10 # 5-20","city":"Medellin"}}'

# solo phone:
curl -X PATCH http://127.0.0.1:8080/profiles/<userId> \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+573009998877"}'

# rechazado (email inmutable):
curl -X PATCH http://127.0.0.1:8080/profiles/<userId> \
  -H 'Content-Type: application/json' \
  -d '{"email":"x@example.com"}'
# 400 {"error":"patch contains immutable or unknown fields: ..."}
```

## 4. Eventos

Bus: `kenko-dev-bus`. Regla propia: `kenko-dev-auth-to-profile` (`ENABLED`, `source: [kenko.auth]`, `detail-type: [UserRegistered, EmailChanged]`) → SQS `kenko-dev-profile-authentication` (+ DLQ `...-dlq`, `maxReceiveCount: 3`).

Formato de entrega (no el de `PutEvents`):

```json
{"source": "kenko.auth", "detail-type": "UserRegistered|EmailChanged", "detail": {"metadata": {...}, "data": {...}}}
```

`metadata`: `eventId` (uuid), `eventVersion: "1"`, `occurredAt` (RFC3339), `correlationId` (uuid).

### Consume (de Authentication)

* `UserRegistered` → crea perfil + outbox `ProfileCreated` en una transacción. Idempotencia por `processed_events.eventId` + `_id=userId`.
  `data`: `userId`, `document{type,number}`, `person{firstName,lastName,birthDate YYYY-MM-DD,gender}`, `contact{email,emailVerified,phone}`, `address{line,city}`.
* `EmailChanged` (hecho consumado, sin confirmación previa) → actualiza `email` por `userId`.
  `data`: `userId`, `newEmail` (requerido), `oldEmail` (opcional, solo verifica), `firstName` (informativo).
  Reglas: recencia por `metadata.occurredAt` (bus estándar, sin orden), duplicados por `eventId` no reaplican, `oldEmail` distinto = no actualiza, `userId` inexistente = terminal (registra `processed`, borra SQS, log `email_user_not_found`), `email_verified=false` siempre. Logs solo IDs.

Inválido o `detail-type` desconocido → no se borra, tras 3 intentos va a DLQ.

### Produce (a Appointments y Documents)

Relay (`internal/profile/relay/relay.go`) lee `outbox_events` y hace `PutEvents`:

* `Source: kenko.profile`, `DetailType: ProfileCreated`, `EventBusName: kenko-dev-bus`, `Detail: {metadata, data}` del perfil.
* Reglas existentes: `kenko-dev-profile-to-appointments` → cola `appointments-profile`, `kenko-dev-profile-to-documents` → cola `documents-profile`.
* Marca `published_at` solo lo aceptado por EventBridge; lo fallido suma `attempts` + `last_error`. TTL 30 días sobre `published_at`.
