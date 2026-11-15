# Frontend - Módulo Documents

Documentación para desarrollar el frontend en un repositorio separado.
Microservicio: `kenko-profile-go`. Base local: `http://127.0.0.1:8080`.

## Base común

* Auth: `Authorization: Bearer <JWT>` obligatorio en todos los endpoints.
  Sin token o inválido -> `401 {"error":"unauthorized"}`.
* JWT: HS256, `exp` obligatorio (~15 min), `iss=kenko.auth`, `aud=kenko-api`,
  `sub` o `userId` = UUID del usuario.
  Referencia: `internal/platform/auth/jwt.go:44-121`.
* Frontend no ingresa `userId`: se extrae del token ya logueado.
  `firstName/email` del JWT son opcionales/legacy; el backend los toma
  del perfil en Mongo si vienen vacíos (`internal/documents/usecase.go:51-60`).
* Header opcional: `X-Correlation-Id`. Si se omite, el backend genera uno.
  Úsalo para trazabilidad, no lo pidas al usuario.
* Body: ningún endpoint de documents recibe body. No hay formularios con inputs.
* Estados globales: implementa `loading` en cada acción, `toast/banner`
  para `201/202/200`, y manejo de `401` (redirigir a login), `404`, `502`, `500`.

---

## 1. `POST /certificates` - Certificado de afiliación

Archivo: `internal/documents/httpapi/handler.go:32-67`.

* Qué hace: genera PDF, lo sube a S3, guarda en Mongo y publica
  `CertificateGenerated`. Es la única forma de obtener certificado.
* Datos que necesita (entrada): ninguno del usuario. Solo headers auth.
  No hay body ni query params.
* Datos que muestra (salida `201`):
  ```json
  {"certificateId":"uuid","issuedAt":"RFC3339","affiliationStartDate":"RFC3339"}
  ```
* Validaciones: token válido y perfil existente para `sub`.
  Internamente `Certificate.Validate()` (`internal/documents/certificate.go:29-52`):
  `ID/UserID` UUID no vacíos, fechas no cero en UTC, `fileKey` no vacío.
  Nada de esto lo valida el frontend, solo garantiza formato UUID/fecha al mostrar.
* Si todo bien: `201` + toast "Certificado generado", opcional mostrar
  `certificateId`, fechas formateadas `DD/MM/YYYY HH:mm`, botón descargar
  (el PDF real llega por correo/evento, no en la respuesta).
* Si ocurre un error:
  `401 unauthorized` (token ausente/malformado/expirado/secreto distinto),
  `404 {"error":"profile not found"}` (usuario sin perfil/seed),
  `502 {"error":"event publish failed"}` (guardado ok pero correo no enviado,
  mostrar reintentar), `500 internal error`.
* UI: 1 botón `Generar certificado`, tarjeta de resultado con 3 campos,
  spinner durante `POST`, estado vacío inicial.

## 2. `POST /medical-history` - Historial médico PDF

Archivos: `internal/documents/historyapi/handler.go:32-68`,
`internal/documents/history_usecase.go:48-155`.

* Qué hace: junta perfil + citas atendidas (`CompletedAppointment`),
  genera PDF, sube a S3, publica `MedicalHistoryPdfGenerated`.
* Datos que necesita: ninguna del usuario. Solo auth.
* Datos que muestra (`201`):
  ```json
  {"pdfId":"uuid","generatedAt":"RFC3339","recordCount":3}
  ```
  `recordCount` = número de citas incluidas.
* Validaciones: token + perfil + `len(citas) > 0`.
  Si `0` citas -> `404 {"error":"no medical records"}`.
  Formato cita esperado (`internal/documents/appointments.go:75-110`):
  `date YYYY-MM-DD`, `startTime/endTime HH:MM`, `status=COMPLETED`,
  `physician.name`, `medicalCenter.name` y `notes` obligatorios.
  Solo informativo para mostrar historial.
* Si todo bien: `201` + tarjeta "Historial generado: N registros, fecha X".
* Si ocurre un error: `401`,
  `404 no medical records/profile not found`
  (mostrar vacío ilustrado "Sin citas atendidas, no se puede generar"),
  `502 event publish failed`, `500`.
* UI: botón `Generar historial médico`, badge con `recordCount`,
  fecha formateada. Mismo patrón de loading que certificado.

## 3. `GET /authorizations` - Listar autorizaciones

Archivos: `internal/documents/authzapi/handler.go:94-108`,
`internal/documents/authz/usecases.go:22-35`.

* Qué hace: lista autorizaciones del usuario autenticado, más recientes primero.
  No emite evento. Devuelve `[]` si no hay, nunca `404`.
* Datos que necesita: ninguna. Solo auth.
* Datos que muestra (`200` array):
  ```json
  [{
    "authorizationId":"uuid","type":"BLOOD_TEST","typeLabel":"Examen de sangre",
    "status":"IN_PROGRESS","statusLabel":"En proceso",
    "rejectionReason":"MISSING_DOCUMENTATION",
    "rejectionReasonLabel":"Falta de documentación",
    "createdAt":"2026-...Z",
    "appointment":{
      "appointmentId":"uuid","date":"2026-05-10",
      "appointmentType":"MEDICINA_GENERAL","physicianName":"..."
    }
  }]
  ```
  Enums (`internal/documents/authorizations.go:15-116`):
  `type`: `BLOOD_TEST/XRAY/DENTAL_CLEANING`;
  `status`: `IN_PROGRESS/REJECTED/APPROVED`;
  `rejectionReason` solo presente si `status == REJECTED`:
  `MISSING_DOCUMENTATION` (Falta de documentación),
  `NOT_CURRENTLY_AFFILIATED` (La persona no está afiliada actualmente),
  `CREATED_IN_ERROR` (Autorización creada por error).
  Labels ES: `Examen de sangre`, `Radiografía`, `Limpieza dental`;
  `En proceso`, `Rechazada`, `Aprobada`.
* Validaciones: solo token. `userId` trim no vacío.
* Si todo bien: `200`, renderizar tabla/tarjetas.
  Columnas: `typeLabel`, `status` (chip: amarillo En proceso,
  verde Aprobada, rojo Rechazada), `createdAt`,
  `appointment.date + appointmentType + physicianName`,
  `rejectionReasonLabel` condicional, acciones Ver PDF / Notificar.
* Si ocurre un error: `401`, `500`.
  Lista vacía no es error, mostrar empty-state.
* UI: tabla o cards responsive + filtro por `status` (cliente),
  chip de estado, skeleton loader, botón reintentar.

## 4. `GET /authorizations/:id/pdf` - Descargar PDF

Archivo: `internal/documents/authzapi/handler.go:111-129`.

* Qué hace: genera al vuelo el PDF de una autorización propia
  y lo devuelve binario. No guarda ni publica.
* Datos que necesita: `id` en path (trim, obligatorio, UUID).
  Ej: `/authorizations/3fa85f64-.../pdf`.
  Debe pertenecer al `sub` del token, si no -> `404`.
* Datos que muestra (`200`):
  `Content-Type: application/pdf`,
  `Content-Disposition: attachment; filename="autorizacion-<slug>-<shortid>.pdf"`
  donde slug `examen-de-sangre/radiografia/limpieza-dental`
  (`internal/documents/authorizations.go:43-54`). Body binario.
* Validaciones: `userId` y `authorizationId` no vacíos,
  autorización existe y es del usuario, perfil existe.
* Si todo bien: abrir/descargar blob.
  En `fetch` usar `response.blob()`, crear `objectURL`.
* Si ocurre un error: `401`, `404 {"error":"not found"}`
  (id inexistente o de otro usuario, mostrar "No encontrada"), `500`.
  No exponer diferencia entre "no existe" y "no es tuya".
* UI: botón icono `Descargar PDF` por fila, loading por fila,
  visor `iframe` o descarga directa.

## 5. `POST /authorizations/:id/notify` - Enviar por correo

Archivos: `internal/documents/authzapi/handler.go:132-159`,
`internal/documents/authz/notify.go:57-148`.

* Qué hace: genera PDF, sube a `authorizations/<userId>/<authId>.pdf`,
  guarda `fileKey`, publica `AuthorizationStatusChanged` (dispara correo).
  Orden `PDF -> S3 -> Mongo -> evento`.
* Datos que necesita: `id` en path, igual que PDF. Sin body.
* Datos que muestra (`202`):
  ```json
  {"authorizationId":"uuid","status":"IN_PROGRESS"}
  ```
  `status` es el enum crudo (`IN_PROGRESS/REJECTED/APPROVED`).
* Validaciones: mismas que PDF + `email` se toma del perfil si JWT no lo trae.
* Si todo bien: `202` + toast "Enviado al correo de tu perfil".
* Si ocurre un error: `401`, `404 not found`,
  `502 event publish failed` (PDF subido pero correo no enviado,
  ofrecer reintentar), `500`.
* UI: botón `Enviar por correo` con modal de confirmación
  ("Se enviará a tu correo registrado"), spinner,
  deshabilitar mientras envía.

---

## Elementos de la interfaz sugeridos

* `ApiClient` con interceptor que inyecta `Bearer` y `X-Correlation-Id` (uuid).
* Páginas: `Certificados`, `Historial`, `Autorizaciones`.
* Componentes: `GenerateButton`, `ResultCard`, `AuthorizationTable`,
  `StatusChip`, `PdfViewer/DownloadButton`, `NotifyModal`,
  `EmptyState`, `ErrorBanner`, `LoadingSkeleton`.
* Sin formularios de captura (no hay body).
  Solo selector/filtro local por estado y botón de acción.