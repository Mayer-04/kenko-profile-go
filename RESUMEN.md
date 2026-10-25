# Resumen de Profile

Guía rápida de lo implementado, cómo ejecutarlo y qué expone.

---

## 1. Qué se hizo

**Profile completo: consumidor y productor de eventos con API REST.**

| Proceso | Qué hace |
| --- | --- |
| **API** (`cmd/api`) | `GET` y `PATCH` de perfiles por HTTP. No publica eventos. |
| **Worker** (`cmd/worker`) | Lee la cola SQS, valida el evento `UserRegistered` y en **una sola transacción** guarda: perfil + evento outbox + registro de idempotencia. Solo después borra el mensaje de SQS. |
| **Relay** (`cmd/relay`) | Lee el outbox cada 2 s, publica `ProfileCreated` en EventBridge y marca como publicado solo lo que EventBridge aceptó. Reintenta los fallos. |

**Decisiones aplicadas** (las que confirmaste):

- Idempotencia con la colección `processed_events` (D4-A): un evento duplicado no crea nada y se descarta.
- Outbox con sondeo y reclamo atómico (D5-A); eventos publicados se borran a los 7 días con índice TTL (D6-A).
- MongoDB local convertido en **replica set de un nodo** para tener transacciones (D9-A), sin perder los datos del volumen.
- Worker y Relay como servicios de Docker Compose (D1-A, D3-A), además de poder correr en el host.
- Sin creación de perfil por HTTP (D2-B): los perfiles solo nacen del evento.
- Preparado para `UserEmailChanged` (D8-B): el worker actualizará la copia del email cuando Authentication lo publique y se amplíe la regla.
- `PATCH` acepta solo `phone` y `address`; cualquier otro campo (incluidos `email` y `password`) responde 400.

---

## 2. Cómo ejecutar

### Opción A: desarrollo en el host (la recomendada)

```sh
# 1. MongoDB local con transacciones (solo la primera vez genera secretos y clave).
sh scripts/generate-mongo-secrets.sh
sh scripts/generate-mongo-key.sh

# 2. Levanta MongoDB (replica set). Debe estar en .env con las credenciales.
docker compose up -d

# 3. Tres terminales (o la API con recarga automática usando: go tool air)
go run ./cmd/api
go run ./cmd/worker
go run ./cmd/relay
```

> Desde el host, la URI de Mongo va a `127.0.0.1:27017` con `directConnection=true`
> (ya está así en tu `.env`). Nunca se registra la URI en los logs.

### Opción B: todo en Docker

```sh
docker compose up -d --build
```

Las credenciales de AWS se toman de tu `~/.aws` (montado y copiado por el
entry point del contenedor). Si exportas `AWS_ACCESS_KEY_ID` y
`AWS_SECRET_ACCESS_KEY`, esas tienen prioridad.

### Verificación previa de AWS (solo lectura)

```sh
aws sts get-caller-identity --region us-east-1
aws events describe-event-bus --name kenko-dev-bus --region us-east-1
```

---

## 3. Endpoints

| Método | Ruta | Qué hace |
| --- | --- | --- |
| `GET` | `http://127.0.0.1:8080/healthz` | Comprueba que la API está viva. |
| `GET` | `http://127.0.0.1:8080/profiles/{id}` | Devuelve el perfil por su id (UUID v7). `404` si no existe. |
| `PATCH` | `http://127.0.0.1:8080/profiles/{id}` | Modifica `phone` y/o `address` completo. **Rechaza el resto con 400.** |

Ejemplos:

```sh
# Consultar
curl http://127.0.0.1:8080/profiles/01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012

# Cambiar teléfono y dirección
curl -X PATCH http://127.0.0.1:8080/profiles/01a136f5-1c2d-7e3f-8a4b-5c6d7e8f9012 \
  -H 'Content-Type: application/json' \
  -d '{"phone":"+573009998877","address":{"line":"Calle 100 # 10-20","city":"Medellin"}}'
```

El `email` y la `password` se cambian en **Authentication** (la interfaz lo llama por HTTP); Profile no interviene.

---

## 4. Pruebas

```sh
# Unitarias (sin AWS ni MongoDB)
go test ./...

# Prueba de extremo a extremo (MongoDB arriba + credenciales AWS activas)
sh scripts/e2e-local.sh

# Integración contra MongoDB (transacciones del worker)
export KENKO_TX_TEST_URI="$(grep '^MONGODB_URI=' .env | cut -d= -f2-)"
go test ./internal/profile/storage -run 'TestIntegration(Transactions|Store)' -v
```

El E2E comprueba todo el flujo: publica un `UserRegistered` sintético, verifica
el perfil en Mongo, el `ProfileCreated` en las colas de Appointments y
Documents, el descarte de un duplicado y el `GET`/`PATCH`. **Ya se ejecutó con
éxito** (los mensajes entregados a las colas de otros servicios no se purgan).

---

## 5. Estructura del código

```
cmd/api, cmd/worker, cmd/relay   → los tres procesos (mains finos)
internal/profile                 → dominio: entidad, eventos, puertos y casos de uso
internal/profile/httpapi         → adaptador HTTP (Echo v5)
internal/profile/storage         → adaptador MongoDB (transacciones, outbox, esquema)
internal/worker                 → bucle de consumo SQS y decisión de borrado
internal/relay                  → bucle de publicación del outbox
internal/platform/config         → carga de variables y selección de URI Mongo
internal/platform/logging       → logger único (slog, inyectado)
internal/platform/sqs           → consumidor de SQS
internal/platform/eventbridge   → publicador de eventos
```

Inyección de dependencias manual por constructores; las interfaces se definen
donde se consumen. Go 1.27: `uuid` de la stdlib (v7) y `encoding/json/v2`.

---

## 6. Variables de entorno clave

Ver `.env.example`. Las más importantes:

| Variable | Para qué |
| --- | --- |
| `MONGODB_URI` | URI de Docker (se usa si está bien formada). |
| `MONGO_ATLAS_URI` | URI de Atlas (respaldo). |
| `AWS_ACCOUNT_ID` | Construye la URL de la cola SQS. |
| `SQS_QUEUE_NAME` | Cola de consumo: `kenko-dev-profile-authentication`. |
| `RELAY_POLL_INTERVAL` | Cada cuánto el Relay consulta el outbox (2 s). |
| `OUTBOX_TTL_DAYS` | Días antes de borrar eventos publicados (7). |
| `LOG_LEVEL` | Nivel de logs (`info`). |

---

## 7. Colecciones de MongoDB (`kenko_eps`)

| Colección | Qué guarda |
| --- | --- |
| `profiles` | El perfil. `_id` = userId de Authentication (UUID v7). |
| `outbox_events` | Eventos pendientes de publicar (patrón Outbox). |
| `processed_events` | Eventos ya procesados (idempotencia). |

Se crean solas con validadores e índices al arrancar cualquier proceso.

---

## 8. Pendiente de tu lado (fuera del alcance del agente)

1. **D8-B**: que Authentication publique `UserEmailChanged` y se amplíe el
   `detail-type` de la regla `kenko-dev-auth-to-profile` (Profile ya lo consume
   cuando llegue).
2. Los mensajes sintéticos quedaron en las colas de Appointments y Documents:
   sus consumidores los procesarán o se limpian a mano.
3. Recordar el riesgo de la sección 4.3: si Appointments/Documents usan datos
   del perfil tras la creación, evaluar la opción C de D8 más adelante.
