# Seed de demostración (perfiles + citas + autorizaciones ficticias)

Crea los 3 perfiles de la demo y, por defecto, 7 perfiles ficticios de relleno
(10 en total), con 6 citas atendidas por usuario en `completed_appointments`
(2 por tipo; 60 en total) y UNA autorización por usuario en `authorizations`
(ciclando BLOOD_TEST/XRAY/DENTAL_CLEANING). Los correos de relleno usan
`@ejemplo.invalid` (dominio inexistente; jamás se les envía correo). No toca
AWS, no publica eventos ni envía correos.

## 1. Obtener los `userId` de Authentication

Cada `userId` del archivo debe ser **el mismo que tiene el usuario en
Authentication** (así el JWT de la demo coincide con el perfil):

1. Registra (o localiza) a los 3 usuarios en Authentication con los correos
   de `seed/demo_users.json`.
2. Copia el `userId` de cada uno (el `sub`/id que devuelve el registro o el
   claim `userId` de su JWT).
3. Reemplaza cada `REEMPLAZAR_CON_USERID_DE_AUTH` por el UUID correspondiente.
   El orden del archivo no importa, pero el correo sí debe ir con su usuario.

El script se detiene sin escribir nada si queda algún marcador, si los
correos no son exactamente los 3 permitidos o si los ids no son UUID válidos.

## 2. Ejecutar (desde la raíz del repo, con el compose arriba)

```bash
go run ./cmd/seed --dry-run            # muestra qué haría, sin escribir
go run ./cmd/seed                      # inserta perfiles + citas + autorizaciones (3 + 7 relleno)
go run ./cmd/seed --extra-profiles 0   # solo los 3 del archivo
go run ./cmd/seed --overwrite-profiles # además actualiza perfiles existentes
go run ./cmd/seed --reset              # borra SOLO citas y autorizaciones de estos usuarios
```

Variables necesarias: `MONGODB_URI` (Docker local) y base (`MONGO_APP_DB`
o `MONGO_APP_DATABASE`, por defecto `kenko_eps`). Se niega a correr contra
Atlas/remoto y si `ENVIRONMENT` es producción. Al arrancar el api, `SEED_DEMO_DATA=true`
siembra lo mismo (`DEMO_USERS_FILE`, `DEMO_EXTRA_PROFILES`, 7 por defecto).
Re-ejecutar es seguro: los ids son determinísticos (`UUIDv5`) y las escrituras
son upsert/`$setOnInsert` (no sobrescribe autorizaciones existentes).

## 3. Comprobar con `mongosh`

```bash
mongosh "mongodb://127.0.0.1:27017/kenko_eps"
db.profiles.countDocuments()                        # 10 (3 demo + 7 relleno)
db.completed_appointments.countDocuments()          # 60
db.authorizations.countDocuments()                  # 10
db.completed_appointments.aggregate([
  { $group: { _id: { userId: "$userId", type: "$type" }, total: { $sum: 1 } } },
  { $sort: { "_id.userId": 1, "_id.type": 1 } }
])
db.completed_appointments.findOne({userId: "<userId>"})
```
