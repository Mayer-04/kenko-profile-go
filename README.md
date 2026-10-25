# Kenko Profile

Microservicio de perfil del portal de salud Kenko EPS. Es **consumidor** de los
eventos de Authentication y **productor** hacia Appointments y Documents, con
comunicación por eventos (EventBridge + SQS) y el patrón **Outbox + Relay**.

## Flujo

```
Authentication --PutEvents (UserRegistered)--> EventBridge (kenko-dev-bus)
  --regla kenko-dev-auth-to-profile--> SQS kenko-dev-profile-authentication
  --> Worker --(una transacción)--> MongoDB: profiles + outbox_events + processed_events
  --> Relay --PutEvents (ProfileCreated)--> EventBridge (kenko-dev-bus)
  --reglas de Appointments y Documents--> sus colas

API REST (GET/PATCH) -----> MongoDB (profiles)
```

- **API** (`cmd/api`): consulta y actualización del perfil por HTTP.
- **Worker** (`cmd/worker`): consume SQS, valida `UserRegistered`, guarda el
  perfil y el outbox **en una sola transacción** y solo entonces borra el
  mensaje. Idempotencia por `processed_events` (D4-A).
- **Relay** (`cmd/relay`): reclama eventos pendientes del outbox (sondeo con
  bloqueo y expiración, D5-A), los publica en el bus y marca como publicado
  solo lo que EventBridge aceptó. Los publicados se borran a los 7 días con un
  índice TTL (D6-A).

## Puesta en marcha (desarrollo local)

```sh
# 1. Secretos de MongoDB (solo la primera vez) y clave de réplica.
sh scripts/generate-mongo-secrets.sh
sh scripts/generate-mongo-key.sh

# 2. Copiar .env.example a .env y completar las credenciales.

# 3. Levantar MongoDB como replica set de un nodo (transacciones).
docker compose up -d

# 4. Proceso de recarga de la API (opcional; también go run ./cmd/api).
go tool air
```

El worker y el relay se ejecutan en el host durante el desarrollo:

```sh
go run ./cmd/worker
go run ./cmd/relay
```

### Todo en Docker

Los servicios `api`, `worker` y `relay` están definidos en `compose.yml`
(misma imagen, distinto comando). Las credenciales del AWS CLI (`~/.aws`)
se montan automáticamente; si prefieres, exporta `AWS_ACCESS_KEY_ID` y
`AWS_SECRET_ACCESS_KEY` y esas tienen prioridad:

```sh
docker compose up -d --build
```

Dentro de la red de Docker la URI de Mongo usa el host `mongodb` con
descubrimiento normal de la réplica; desde el host la URI usa
`127.0.0.1` con `directConnection=true`. La selección de URI sigue la
sección 8.3: primero la de Docker si está definida y bien formada, si no la
de Atlas; nunca se registra en los logs.

## Prueba de extremo a extremo

Con MongoDB levantado y las credenciales de AWS activas:

```sh
sh scripts/e2e-local.sh
```

Publica un `UserRegistered` sintético y comprueba todo el flujo: creación del
perfil, `ProfileCreated` en las colas de Appointments y Documents,
descarte del duplicado, `GET`/`PATCH` por HTTP y limpieza de los datos.
Los mensajes entregados a las colas de los otros servicios no se purgan.

## Pruebas de integración de MongoDB

```sh
export KENKO_TX_TEST_URI="$(grep '^MONGODB_URI=' .env | cut -d= -f2-)"
go test ./internal/profile/storage -run 'TestIntegration(Transactions|Store)' -v
```

## API REST

| Método | Ruta | Descripción |
| --- | --- | --- |
| `GET` | `/healthz` | El proceso está vivo. |
| `GET` | `/profiles/{id}` | Recupera el perfil por su id (UUID v7). |
| `PATCH` | `/profiles/{id}` | Modifica **solo** `phone` y/o `address` completo. Rechaza cualquier otro campo con 400 (email y password se cambian en Authentication). No publica eventos. |

## Datos en MongoDB (`kenko_eps`)

| Colección | Contenido |
| --- | --- |
| `profiles` | Perfil con la correspondencia de la sección 7.1 (`_id` = userId de Authentication, UUID BSON subtype 4). Validador `$jsonSchema` e inmutabilidad de documento, nombres, fecha de nacimiento y género. |
| `outbox_events` | Eventos pendientes de publicar; índice TTL sobre `published_at` (7 días) e índice para el reclamo del Relay. |
| `processed_events` | Idempotencia por `event_id` único. |

Las colecciones se crean con validadores e índices al arrancar cualquier
proceso (`EnsureSchema`, idempotente).

## Eventos

- **Consume**: `UserRegistered` (sección 7.1) y `UserEmailChanged` (D8-B,
  preparado: actualiza la copia del email ignorando eventos más antiguos;
  *pendiente por tu parte*: que Authentication lo publique y ampliar el
  `detail-type` de la regla `kenko-dev-auth-to-profile`).
- **Publica**: `ProfileCreated` (sección 7.2) con `source: kenko.profile`,
  `detail-type: ProfileCreated`, eventId nuevo UUID v7 y correlationId
  heredado.

## Variables de entorno

Ver `.env.example`. Las destacadas:

| Variable | Uso |
| --- | --- |
| `MONGODB_URI` / `MONGO_ATLAS_URI` | URIs de Docker (primera opción) y Atlas. |
| `AWS_ACCOUNT_ID` | Para construir la URL de la cola de SQS. |
| `SQS_QUEUE_NAME` | Cola de consumo (`kenko-dev-profile-authentication`). |
| `RELAY_POLL_INTERVAL`, `RELAY_BATCH_SIZE`, `OUTBOX_LOCK_TTL` | Sintonía del Relay. |
| `OUTBOX_TTL_DAYS` | Días de retención de eventos publicados (índice TTL). |
| `LOG_LEVEL` | Nivel del logger (`info` por defecto). |

## Logs

`log/slog` en JSON por stdout, definido en `internal/platform/logging` e
inyectado por dependencias. Sin datos personales, URIs ni payloads; con
`eventId` y `correlationId` cuando existen.

## Infraestructura AWS (ya existe; solo se referencia)

Cuenta `137696817246`, región `us-east-1`, bus `kenko-dev-bus`. Regla de
consumo `kenko-dev-auth-to-profile`; reglas de publicación hacia
Appointments y Documents (filtran `detail-type: ProfileCreated`). Permisos:
`events:PutEvents` sobre el bus y operaciones SQS sobre la cola propia.
Este servicio no crea ni modifica infraestructura AWS.
