# syntax=docker/dockerfile:1

# ---- web UI ----
# Builds the React SPA (ui/) into pkg/web/dist, which the Go binary embeds.
# Runs on the build machine's architecture; the output is plain JS/CSS.
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /src/ui
COPY ui/package*.json ./
RUN if [ -f package-lock.json ]; then npm ci --no-audit --no-fund; \
    else npm install --no-audit --no-fund; fi
COPY ui/ ./
RUN npm run build

# ---- Go build ----
# Also runs natively and cross-compiles for the target platform (the server is
# pure Go, CGO-free), so multi-arch builds need no emulation here.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

# Download modules first so they cache independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static binary. All web assets are embedded via go:embed, so the
# runtime image needs nothing but the binary and CA certificates.
# -tags nodynamic: the HEIC decoder otherwise loads libheif through purego,
# which links against glibc even with CGO_ENABLED=0, and Alpine (musl) can't
# start that binary ("exec /usr/local/bin/gpix: no such file or directory").
# With the tag it uses its embedded WASM decoder. The check below stops the
# build if the binary ever ends up dynamically linked again.
RUN apk add --no-cache file
COPY . .
COPY --from=ui /src/pkg/web/dist ./pkg/web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -tags nodynamic -trimpath -ldflags="-s -w" -o /out/gpix . \
 && file /out/gpix \
 && file /out/gpix | grep -q "statically linked"

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 gpix \
 && mkdir -p /data && chown gpix:gpix /data

COPY --from=build /out/gpix /usr/local/bin/gpix

USER gpix
# State (.env, secret.key, gateways.json, the *.db files) lives here — mount it.
WORKDIR /data
VOLUME ["/data"]

# web UI : 8080   S3 : 9000   WebDAV : 8081
EXPOSE 8080 9000 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/login >/dev/null 2>&1 || exit 1

ENTRYPOINT ["gpix"]
# Web only by default. The Telegram bot stays off unless TG_BOT_ENABLED=true and
# the mode is "all" or "bot".
CMD ["-mode", "web"]
