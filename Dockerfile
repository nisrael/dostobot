# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /dostobot .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

# unzip and tar are needed for archive extraction
RUN apk add --no-cache ca-certificates tzdata unzip tar

# Non-root user
RUN addgroup -S dostobot && adduser -S -G dostobot dostobot

COPY --from=builder /dostobot /usr/local/bin/dostobot

# Default mount points (override via volumes)
RUN mkdir -p /music /downloads /data \
    && chown -R dostobot:dostobot /music /downloads /data

USER dostobot

EXPOSE 8080

ENV PORT=8080 \
    AUTH_USERNAME=admin \
    AUTH_PASSWORD_HASH="" \
    LIBRARY_DIR=/music \
    DOWNLOAD_DIR=/downloads \
    DATA_DIR=/data

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:${PORT}/health || exit 1

ENTRYPOINT ["/usr/local/bin/dostobot"]
