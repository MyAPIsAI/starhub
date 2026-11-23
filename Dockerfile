FROM node:24-bookworm-slim AS console
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
COPY web/scripts ./scripts
RUN npm ci
COPY web/ ./
# The console embeds task plugin examples from the sibling source directory.
COPY plugins/tasks /src/plugins/tasks
RUN npm run typecheck && npm run build

FROM golang:1.26-bookworm AS backend
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
ENV CGO_ENABLED=0 GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console /src/web/dist ./web/dist
RUN go build -trimpath -tags frontend -o /out/starhub .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 starhub && mkdir /data && chown starhub /data
COPY --from=backend /out/starhub /usr/local/bin/starhub
COPY NOTICE THIRD-PARTY-LICENSES.md /licenses/
USER starhub
WORKDIR /data
VOLUME ["/data"]
EXPOSE 3000

# Environment variables (set at runtime, not bake into the image):
#   STARHUB_ENV                — production or development
#   STARHUB_CRYPTO_SECRET      — stable AES-256 key for credential encryption
#   STARHUB_CREDENTIAL_KEYS    — versioned wrapping keys for credential envelopes
#   STARHUB_CREDENTIAL_ACTIVE_KEY — active wrapping key name
#   STARHUB_ROOT_PASSWORD      — initial root password
#   STARHUB_BIND_ADDRESS       — bind to a private or loopback IP
#   STARHUB_TRUSTED_PROXIES    — trusted reverse proxy CIDRs
#   STARHUB_ADMIN_IP_WHITELIST — management access CIDRs
#   STARHUB_DB_DRIVER          — sqlite, mysql, or postgres
#   STARHUB_DB_DSN             — network database connection string
#   STARHUB_REGION_PROFILE     — deployment-selected regional policy
#   STARHUB_BACKUP_DIR         — SQLite snapshot directory
#   STARHUB_BACKUP_INTERVAL    — SQLite backup schedule (e.g. 6h)
#   STARHUB_BACKUP_KEEP        — retained snapshot count (default 7)
#   STARHUB_SETUP_TOKEN        — one-time first-run setup token
#   STARHUB_EMAIL_BRIDGE_URL   — authenticated mail adapter endpoint
#   STARHUB_EMAIL_BRIDGE_TOKEN — mail adapter bearer token
#   STARHUB_SMS_BRIDGE_URL     — authenticated SMS adapter endpoint
#   STARHUB_SMS_BRIDGE_TOKEN   — SMS adapter bearer token
#   RESOURCE_CATALOG_NOTIFY_URL — resource platform base URL
#   NODE_NAME                  — stable gateway node identifier
# See config.example.yml and PRODUCTION.md for full descriptions.
ENTRYPOINT ["/usr/local/bin/starhub"]
