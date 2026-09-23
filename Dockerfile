FROM node:24-bookworm-slim AS console
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
COPY web/scripts ./scripts
RUN npm ci
COPY web/ ./
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
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 starhub && mkdir /data && chown starhub /data
COPY --from=backend /out/starhub /usr/local/bin/starhub
USER starhub
WORKDIR /data
ENV STARHUB_DB=/data/starhub.db STARHUB_PORT=3000
VOLUME ["/data"]
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/starhub"]
