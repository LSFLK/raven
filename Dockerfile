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
RUN go build -a -o imap-server ./cmd/server
RUN go build -a -o raven-delivery ./cmd/delivery
RUN go build -a -o raven-sasl ./cmd/sasl

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

# Copy configuration file
COPY config/raven.yaml /etc/raven/raven.yaml

# Create directories with proper permissions
RUN mkdir -p /app/data /var/run/raven /etc/raven /var/run/sasl && \
    chown -R ravenuser:ravenuser /app /var/run/raven /etc/raven /var/run/sasl

# Create startup script
RUN echo '#!/bin/sh' > /app/start.sh && \
    echo 'echo "Starting Raven services..."' >> /app/start.sh && \
    echo 'echo "Starting SASL authentication service..."' >> /app/start.sh && \
    echo './raven-sasl -socket /var/run/sasl/auth.sock -config /etc/raven/raven.yaml &' >> /app/start.sh && \
    echo 'SASL_PID=$!' >> /app/start.sh && \
    echo 'echo "SASL service started with PID: $SASL_PID"' >> /app/start.sh && \
    echo 'sleep 1' >> /app/start.sh && \
    echo 'echo "Starting IMAP server..."' >> /app/start.sh && \
    echo './imap-server &' >> /app/start.sh && \
    echo 'IMAP_PID=$!' >> /app/start.sh && \
    echo 'echo "IMAP server started with PID: $IMAP_PID"' >> /app/start.sh && \
    echo 'sleep 1' >> /app/start.sh && \
    echo 'echo "Starting Delivery service (LMTP)..."' >> /app/start.sh && \
    echo './raven-delivery &' >> /app/start.sh && \
    echo 'DELIVERY_PID=$!' >> /app/start.sh && \
    echo 'echo "Delivery service started with PID: $DELIVERY_PID"' >> /app/start.sh && \
    echo 'echo ""' >> /app/start.sh && \
    echo 'echo "==================================="' >> /app/start.sh && \
    echo 'echo "All Raven services started:"' >> /app/start.sh && \
    echo 'echo "  SASL Auth: PID $SASL_PID"' >> /app/start.sh && \
    echo 'echo "  IMAP:      PID $IMAP_PID"' >> /app/start.sh && \
    echo 'echo "  LMTP:      PID $DELIVERY_PID"' >> /app/start.sh && \
    echo 'echo "==================================="' >> /app/start.sh && \
    echo 'wait' >> /app/start.sh && \
    chmod +x /app/start.sh && \
    chown ravenuser:ravenuser /app/start.sh

# Switch to non-root user
USER ravenuser

# Expose ports for services
# IMAP: 143 (plaintext), 993 (TLS)
# LMTP: 24
# SASL: Uses Unix socket (no port needed)
EXPOSE 143 993 24

# Set environment variables
ENV DB_FILE=/app/data/mails.db

# Health check - check IMAP and LMTP services (SASL uses Unix socket)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 143 && nc -z localhost 24 || exit 1

# Start all services
ENTRYPOINT ["/app/start.sh"]