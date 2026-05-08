# ---------------------------------------------------------------------------
# Stage 1: Build
# ---------------------------------------------------------------------------
FROM golang:1.22-alpine AS builder

# Install build dependencies (gcc needed for CGO used by some GORM drivers).
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependencies separately from source.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -extldflags '-static'" \
    -o /kubepilot-backend ./cmd/server

# ---------------------------------------------------------------------------
# Stage 2: Runtime
# ---------------------------------------------------------------------------
FROM alpine:3.19

# Non-root user for security.
RUN addgroup -S kubepilot && adduser -S -G kubepilot kubepilot

# CA certificates needed for HTTPS calls to registries, Helm repos, etc.
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /kubepilot-backend /app/kubepilot-backend

# Ensure the binary is owned by the non-root user.
RUN chown kubepilot:kubepilot /app/kubepilot-backend

USER kubepilot

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/kubepilot-backend"]
