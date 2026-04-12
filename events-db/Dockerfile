# ─── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies first (layer re-used unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/api/main.go

# ─── Stage 2: Download golang-migrate ────────────────────────────────────────
FROM alpine:3.19 AS migrate-dl

RUN apk add --no-cache curl tar

RUN curl -L \
    https://github.com/golang-migrate/migrate/releases/download/v4.17.1/migrate.linux-amd64.tar.gz \
    | tar -xz -C /usr/local/bin migrate

# ─── Stage 3: Runtime image ───────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder    /app/server            ./server
COPY --from=migrate-dl /usr/local/bin/migrate  /usr/local/bin/migrate
COPY migrations/                               ./migrations/
COPY docker-entrypoint.sh                      ./docker-entrypoint.sh

RUN chmod +x ./docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["./docker-entrypoint.sh"]