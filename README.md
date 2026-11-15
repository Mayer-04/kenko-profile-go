# Kenko Profile

Microservicio Profile de Kenko EPS: consume `UserRegistered` de Authentication por SQS,
guarda el perfil en MongoDB con patrón Outbox + Relay y publica `ProfileCreated` en EventBridge.

## Servicios (`compose.yml`)

| Servicio | Comando | Función |
| --- | --- | --- |
| `mongo` | replica set `kenkoRs` (1 nodo, con keyFile) | MongoDB local con transacciones |
| `mongo-init` | `rs.initiate` idempotente | Inicia el replica set |
| `api` | `/app/api` | `GET /profiles/{id}`, `PATCH /profiles/{id}`, `GET /health`, documents: `POST /certificates`, `POST /medical-history`, `GET /authorizations`, `GET /authorizations/:id/pdf`, `POST /authorizations/:id/notify` |
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

## Endpoints documents (requieren `Authorization: Bearer <JWT Auth>`)

JWT Auth actual: `sub` (=userId), `iss=kenko.auth`, `aud=kenko-api`, `exp` 15 min.
`JWT_SECRET` debe ser el de authorization. `email`/`firstName` se toman del perfil en Mongo.

```bash
# Certificado -> evento CertificateGenerated (Source: kenko.profile)
curl -X POST http://127.0.0.1:8080/certificates \
 -H "Authorization: Bearer $TOKEN"
# 201 {"certificateId":"...","issuedAt":"...","affiliationStartDate":"..."}

# Historial -> evento MedicalHistoryPdfGenerated
curl -X POST http://127.0.0.1:8080/medical-history \
 -H "Authorization: Bearer $TOKEN"
# 201 {"pdfId":"...","generatedAt":"...","recordCount":N}
# 404 {"error":"no medical records"} si no tiene citas

# Listar autorizaciones (no emite evento)
curl http://127.0.0.1:8080/authorizations \
 -H "Authorization: Bearer $TOKEN"
# 200 [{authorizationId, type, typeLabel, status, statusLabel, appointment:{...}}]

# PDF autorización (no emite evento)
curl http://127.0.0.1:8080/authorizations/<id>/pdf \
 -H "Authorization: Bearer $TOKEN" --output auth.pdf
# 200 application/pdf

# Notificar autorización -> evento AuthorizationStatusChanged
curl -X POST http://127.0.0.1:8080/authorizations/<id>/notify \
 -H "Authorization: Bearer $TOKEN"
# 202 {"authorizationId":"...","status":"..."}
```

Errores comunes: `401` token inválido/expirado o secreto distinto al de Auth;
`404` el `sub` no tiene perfil (debe llegar por SQS `UserRegistered` o estar en `seed/demo_users.json`).

## CORS

Solo CORS, sin cambios de negocio. Middleware propio en `internal/platform/cors/cors.go`,
registrado con `e.Pre` en `cmd/api/main.go:50` para que corra antes que la
autenticación JWT (que vive en cada handler) e incluso en 404 del router.

- Variable: `CORS_ALLOWED_ORIGINS` (orígenes completos separados por comas).
  Ejemplo dev en `.env.example`: `http://localhost:3000,http://localhost:3001,http://localhost:5000,http://localhost:8080`.
- Si no está definida y el entorno NO es producción: se usan esos 4 por defecto.
  Si no está definida y el entorno ES producción (`production`/`prod` en `APP_ENV` o `ENVIRONMENT`): no se envían headers (todo denegado).
- Comparación exacta (esquema+host+puerto, sin barra final, sin `*`).
  Al desplegar el frontend solo se cambia el valor de la variable a su URL real.
- Origen permitido: `Access-Control-Allow-Origin: <origen>` + `Vary: Origin` +
  `Access-Control-Expose-Headers: Content-Disposition, X-Correlation-Id`.
- Preflight `OPTIONS`: `204` con `Allow-Methods: GET, POST, PATCH, OPTIONS`,
  `Allow-Headers: Authorization, Content-Type, Accept, X-Correlation-Id`,
  `Max-Age: 600`, sin exigir token. Sin credenciales.

## Notas

- `MONGODB_URI` (Docker) tiene prioridad si está bien formada; si no, se usa `MONGO_ATLAS_URI` (ver `internal/platform/config/config.go`). Las URIs nunca se registran en logs.
- Los contenedores `api/worker/relay` leen credenciales AWS de `~/.aws` (solo lectura) y corren con tu `UID:GID` para poder leerlas.
- El `PATCH` solo acepta `phone` y `address` completo (`{line, city}`); rechaza `email`, `password` y cualquier otro campo. No publica eventos.
- Idempotencia por `processed_events.eventId` (D4-A) + unicidad de `_id = userId`.
- Limpieza del outbox con TTL de 30 días sobre `published_at` (D6-A).
- El worker consume `UserRegistered` y `EmailChanged` de Authentication por la misma cola; el cambio de email actualiza la proyección por `userId` (hecho consumado, sin publicar eventos).

## Logs

```bash
docker compose logs -f api worker relay
docker compose logs --tail=50 worker
docker compose ps
curl http://127.0.0.1:8080/health
```