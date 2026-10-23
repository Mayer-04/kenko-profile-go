# Kenko Profile

---

## Air

Iniciar Air

```bash
go tool air
```

---

```sh
chmod +x scripts/generate-mongo-secret.sh
./scripts/generate-mongo-secret.sh
```

```sh
sh ./scripts/generate-mongo-secret.sh
```

---

```sh
docker compose ps
```

```sh
docker compose up -d
```

```sh
docker compose logs -f mongodb
```

```sh
docker compose logs --tail=100 mongodb
```

```sh
docker container exec -it kenko-mongo mongosh \
  --username kenko_admin \
  --authenticationDatabase admin \
  --password
```

```sh
db.getSiblingDB("kenko_eps").getUser("mayer")

db.getSiblingDB("kenko_eps").getUsers()
```
