# Imagen de Profile: una sola imagen con los tres procesos (api, worker y
# relay); compose elige el comando de cada servicio (D1-A y D3-A).
FROM golang:1.27-alpine AS build

WORKDIR /src

# Primero solo las dependencias, para aprovechar la caché de capas.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 go build -trimpath -o /out/relay ./cmd/relay

FROM alpine:3.22

# ca-certificates: necesario para el TLS de AWS y MongoDB Atlas.
# su-exec: para bajar de root a kenko en el entry point.
RUN apk add --no-cache ca-certificates su-exec \
 && adduser -D -u 10001 kenko

COPY --from=build /out/ /usr/local/bin/
COPY docker/entrypoint.sh /usr/local/bin/entrypoint
RUN chmod +x /usr/local/bin/entrypoint

# Sin USER: el entry point empieza como root (para leer los montajes de solo
# lectura del host) y ejecuta el proceso como kenko.
ENTRYPOINT ["/usr/local/bin/entrypoint"]
