# Kenko Profile

Microservicio Profile de Kenko EPS: consume `UserRegistered` de Authentication por SQS,
guarda el perfil en MongoDB con patrón Outbox + Relay y publica `ProfileCreated` en EventBridge.

## Servicios (`compose.yml`)

| Servicio | Comando | Función |
| --- | --- | --- |
| `mongo` | replica set `kenkoRs` (1 nodo, con keyFile) | MongoDB local con transacciones |
| `mongo-init` | `rs.initiate` idempotente | Inicia el replica set |
| `api` | `/app/api` | `GET /profiles/{id}`, `PATCH /profiles/{id}`, `GET /health` |
| `worker` | `/app/worker` | Consume SQS `kenko-dev-profile-authentication` |
| `relay` | `/app/relay` | Publica el outbox en `kenko-dev-bus` |

## Desarrollo local

```bash
cp .env.example .env   # completar credenciales (nunca se commitea)
docker compose up -d --build   # mongo + api + worker + relay
curl http://127.0.0.1:8080/health
```

Solo el API con recarga:

```bash
go tool air
```

## Notas

- `MONGODB_URI` (Docker) tiene prioridad si está bien formada; si no, se usa `MONGO_ATLAS_URI` (ver `internal/platform/config/config.go`). Las URIs nunca se registran en logs.
- Los contenedores `api/worker/relay` leen credenciales AWS de `~/.aws` (solo lectura) y corren con tu `UID:GID` para poder leerlas.
- El `PATCH` solo acepta `phone` y `address` completo (`{line, city}`); rechaza `email`, `password` y cualquier otro campo. No publica eventos.
- Idempotencia por `processed_events.eventId` (D4-A) + unicidad de `_id = userId`.
- Limpieza del outbox con TTL de 30 días sobre `published_at` (D6-A).
- D8-B: el worker también acepta `UserEmailChanged` (requiere que Authentication lo publique y que se amplíe el `detail-type` de la regla `kenko-dev-auth-to-profile`; cambio fuera de este repo).
