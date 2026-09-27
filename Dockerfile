# ==============================================================================
# NPM Auto-Discovery Multi-Stage Production Dockerfile
# ==============================================================================

# Stage 1: Build binary
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache Go modules dependencies
COPY go.mod ./
# (go.sum if exists)
COPY . .

# Statically compile single binary with stripped symbols for minimal footprint
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o npm-autodiscovery ./cmd/agent

# Stage 2: Minimal runtime image
FROM alpine:3.20

# Install runtime dependencies (certificates for HTTPS, tzdata for timezones)
RUN apk add --no-cache ca-certificates tzdata

# Create application directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/npm-autodiscovery /app/npm-autodiscovery

# Environment defaults
ENV PORT=8080 \
    POLL_INTERVAL=30s \
    DOCKER_SOCKET=/var/run/docker.sock \
    DEFAULT_FORWARD_SCHEME=http \
    DEFAULT_SSL_ENABLED=false \
    FORWARD_HOST_STRATEGY=auto

# Expose web dashboard & API port
EXPOSE 8080

# Health check against internal status endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/status || exit 1

ENTRYPOINT ["/app/npm-autodiscovery"]
