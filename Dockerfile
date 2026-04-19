# syntax=docker/dockerfile:1.7

# Stage A — build the SPA with Bun.
FROM oven/bun:1-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# Stage B — compile the Go backend with the embedded SPA.
FROM golang:1.25-alpine AS server
WORKDIR /src
RUN apk add --no-cache git
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=web /web/dist/ ./internal/webui/files/
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/plantry ./cmd/plantry

# Stage C — minimal runtime.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S plantry && adduser -S -G plantry plantry && \
    mkdir -p /data && chown plantry:plantry /data
COPY --from=server /out/plantry /usr/local/bin/plantry
USER plantry
WORKDIR /data
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:${PLANTRY_PORT:-8080}/api/health || exit 1
ENV PLANTRY_PORT=8080 \
    PLANTRY_DB_PATH=/data/plantry.db \
    PLANTRY_LOG_LEVEL=info
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/plantry"]
