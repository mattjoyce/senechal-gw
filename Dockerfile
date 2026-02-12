# Build stage
FROM golang:1.25.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags="-w -s" -o senechal-gw ./cmd/senechal-gw

# Runtime stage
FROM alpine:latest

# Install runtime dependencies (bash for plugins, jq for JSON parsing, python3 for python plugins)
RUN apk add --no-cache ca-certificates tzdata bash jq python3 py3-pip sqlite-libs

# Create app user
RUN addgroup -S senechal && adduser -S senechal -G senechal

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/senechal-gw .

# Copy plugins directory
COPY --chown=senechal:senechal plugins/ ./plugins/

# Copy pipelines directory if it exists
COPY --chown=senechal:senechal pipelines ./pipelines

# Create data directory for state persistence
RUN mkdir -p /app/data && chown -R senechal:senechal /app/data

# Switch to non-root user
USER senechal

# Expose API port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD [ -f /app/senechal.pid ] || exit 1

# Default command
CMD ["./senechal-gw", "start"]
