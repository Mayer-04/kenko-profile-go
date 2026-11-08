# Seed de demostración (perfiles + citas ficticias)

Crea los 3 perfiles de la demo y 4–6 citas atendidas por usuario en
`completed_appointments`. No toca AWS, no publica eventos ni envía correos.

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
go run ./cmd/seed                      # inserta perfiles + citas
go run ./cmd/seed --overwrite-profiles # además actualiza perfiles existentes
go run ./cmd/seed --reset              # borra SOLO las citas de estos usuarios
```

Variables necesarias: `MONGODB_URI` (Docker local) y base (`MONGO_APP_DB`
o `MONGO_APP_DATABASE`, por defecto `kenko_eps`). Se niega a correr contra
Atlas/remoto. Re-ejecutar es seguro: los ids de cita son determinísticos y
las escrituras son upsert.

## 3. Comprobar con `mongosh`

```bash
mongosh "mongodb://127.0.0.1:27017/kenko_eps"
db.profiles.countDocuments()                        # 3
db.completed_appointments.countDocuments()          # 12-18
db.completed_appointments.findOne({userId: "<userId>"})
```
