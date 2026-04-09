# ── Stage 1: Dependencies cache ─────────────────────────────────────────────
# This stage only installs dependencies to optimize layer caching
FROM golang:1.23-alpine AS deps

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy only go mod files
COPY go.mod go.sum ./

# Download and cache dependencies (expensive step, cached separately)
RUN go mod download

# ── Stage 2: Builder ─────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy cached dependencies
COPY --from=deps /go/pkg/mod /go/pkg/mod

# Copy source code
COPY . .

# Build binary with version/build info from build args
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w \
    -X main.Version=${VERSION} \
    -X main.BuildDate=${BUILD_DATE} \
    -X main.VcsRef=${VCS_REF}" \
    -o generator ./cmd

# ── Stage 3: Runtime ────────────────────────────────────────────────────────
FROM alpine:latest

WORKDIR /app

# Install CA certificates for TLS connections
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 generator && \
    adduser -D -u 1000 -G generator generator

# Create necessary directories
RUN mkdir -p /app/logs /app/events && \
    chown -R generator:generator /app

# Copy binary from builder
COPY --from=builder /build/generator /app/generator
COPY --from=builder /build/config.example.yaml /app/config.example.yaml

# Fix permissions
RUN chown -R generator:generator /app

# Switch to non-root user
USER generator

# OCI labels for container metadata
LABEL org.opencontainers.image.title="EchoMessenger Generator" \
      org.opencontainers.image.description="Security test scenario generator for EchoMessenger audit system" \
      org.opencontainers.image.vendor="EchoMessenger" \
      org.opencontainers.image.url="https://github.com/echo-messenger/echo-messenger" \
      org.opencontainers.image.source="https://github.com/echo-messenger/echo-messenger/tree/main/generator" \
      org.opencontainers.image.documentation="https://github.com/echo-messenger/echo-messenger/tree/main/generator#readme"

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ps aux | grep '[g]enerator' || exit 1

# Expose metrics port
EXPOSE 8080

# Default entrypoint and command
ENTRYPOINT ["/app/generator"]
CMD ["-config", "/app/config.yaml"]

