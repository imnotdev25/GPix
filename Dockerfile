# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Download modules first so they cache independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static binary. All web assets are embedded via go:embed, so the
# runtime image needs nothing but the binary and CA certificates.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gpix .

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 gpix \
 && mkdir -p /data && chown gpix:gpix /data

COPY --from=build /out/gpix /usr/local/bin/gpix

USER gpix
# State (gpix-web.conf, .env, secret.key, gateways.json) lives here — mount it.
WORKDIR /data
VOLUME ["/data"]

# web UI : 8080   S3 : 9000   WebDAV : 8081
EXPOSE 8080 9000 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/login >/dev/null 2>&1 || exit 1

ENTRYPOINT ["gpix"]
# web only by default; pass "-mode all" to also run the Telegram bot.
CMD ["-mode", "web"]
