# Stage 1: build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build tools for CGO (required for SQLite)
RUN apk add --no-cache git build-base sqlite-dev gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source code
COPY . .

# Enable CGO
ENV CGO_ENABLED=1

# Build all services
RUN go build -o imap-server ./cmd/server && \
    go build -o raven-delivery ./cmd/delivery && \
    go build -o raven-sasl ./cmd/sasl

# Stage 2: runtime
FROM alpine:3.18

WORKDIR /app

# Install required runtime dependencies
RUN apk add --no-cache sqlite tzdata netcat-openbsd ca-certificates bash \
    && rm -rf /var/cache/apk/*

# Create a non-root user
RUN addgroup -g 1001 -S ravenuser && \
    adduser -u 1001 -S ravenuser -G ravenuser

# Copy all binaries from builder
COPY --from=builder /app/imap-server .
COPY --from=builder /app/raven-delivery .
COPY --from=builder /app/raven-sasl .

# Create directories with proper permissions
RUN mkdir -p /app/data /var/run/raven /etc/raven /var/spool/postfix/private && \
    chown -R ravenuser:ravenuser /app /var/run/raven /etc/raven && \
    chmod 777 /var/spool/postfix/private


COPY ./scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && \
    chown ravenuser:ravenuser /app/entrypoint.sh

# Switch to non-root user
USER ravenuser

# Expose ports for services
# IMAP: 143 (plaintext), 993 (TLS)
# LMTP: 24
# SASL: 12345 (TCP)
EXPOSE 143 993 24 12345

# Set environment variables - use directory path for DBManager
ENV DB_PATH=/app/data/databases

# Health check - check IMAP, LMTP, and SASL services
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 143 && nc -z localhost 24 && nc -z localhost 12345 || exit 1

# Start all services
ENTRYPOINT ["/app/entrypoint.sh"]