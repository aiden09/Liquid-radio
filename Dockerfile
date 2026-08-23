# syntax=docker/dockerfile:1

# ── Build stage ──────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY web ./web

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/liquid-radio .

# ── Runtime stage ────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 1000 radio

WORKDIR /app

COPY --from=builder /out/liquid-radio /app/liquid-radio
RUN mkdir -p /app/music && chown -R radio:radio /app

USER radio

ENV TZ=UTC \
    PORT=8080 \
    MUSIC_DIR=/app/music

EXPOSE 8080

VOLUME ["/app/music"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/" >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/liquid-radio"]
