# ============================================================================
# Multi-Stage Dockerfile for Raven Mail Server
# ============================================================================
# This Dockerfile supports building four different images:
#   1. raven-sasl  - SASL authentication service only
#   2. raven-lmtp  - LMTP delivery service only
#   3. raven-imap  - IMAP server only
#   4. raven       - All services combined
# ============================================================================

# ============================================================================
# Stage 1: Build all services
# ============================================================================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build tools for CGO (required for SQLite)
RUN apk add --no-cache git build-base sqlite-dev gcc musl-dev

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source code
COPY . .

# Enable CGO for SQLite support
ENV CGO_ENABLED=1

# Build all services as separate binaries
# Add new service builds here following the same pattern
RUN go build -ldflags="-w -s" -o imap-server ./cmd/server && \
    go build -ldflags="-w -s" -o raven-delivery ./cmd/delivery && \
    go build -ldflags="-w -s" -o raven-sasl ./cmd/sasl

# ============================================================================
# Stage 2: Base runtime image
# ============================================================================
FROM alpine:3.18 AS base

# Install required runtime dependencies
RUN apk add --no-cache sqlite tzdata netcat-openbsd ca-certificates bash \
    && rm -rf /var/cache/apk/*

# Create a non-root user
RUN addgroup -g 1001 -S ravenuser && \
    adduser -u 1001 -S ravenuser -G ravenuser

# ============================================================================
# Stage 3: SASL Authentication Service Image
# ============================================================================
FROM base AS raven-sasl

# Copy only the SASL binary
COPY --from=builder /app/raven-sasl .

# Copy entrypoint script
COPY ./scripts/entrypoint-sasl.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && \
    chown ravenuser:ravenuser /app/entrypoint.sh

# Create required directories
RUN mkdir -p /etc/raven && \
    chown -R ravenuser:ravenuser /app /etc/raven

# Switch to non-root user
USER ravenuser

# Expose SASL TCP port
EXPOSE 12345

# Set environment variables
ENV SERVICE_NAME=sasl

# Health check for SASL service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 12345 || exit 1

# Start SASL service
ENTRYPOINT ["/app/entrypoint.sh"]

# ============================================================================
# Stage 4: LMTP Delivery Service Image
# ============================================================================
FROM base AS raven-lmtp

# Copy only the LMTP binary
COPY --from=builder /app/raven-delivery .

# Copy entrypoint script
COPY ./scripts/entrypoint-lmtp.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && \
    chown ravenuser:ravenuser /app/entrypoint.sh

# Create required directories
RUN mkdir -p /app/data /var/run/raven /etc/raven /var/spool/postfix/private && \
    chown -R ravenuser:ravenuser /app /var/run/raven /etc/raven && \
    chmod 777 /var/spool/postfix/private

# Switch to non-root user
USER ravenuser

# Expose LMTP port
EXPOSE 24

# Set environment variables
ENV DB_PATH=/app/data/databases \
    SERVICE_NAME=lmtp

# Health check for LMTP service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 24 || exit 1

# Start LMTP service
ENTRYPOINT ["/app/entrypoint.sh"]

# ============================================================================
# Stage 5: IMAP Server Image
# ============================================================================
FROM base AS raven-imap

# Copy only the IMAP binary
COPY --from=builder /app/imap-server .

# Copy entrypoint script
COPY ./scripts/entrypoint-imap.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && \
    chown ravenuser:ravenuser /app/entrypoint.sh

# Create required directories
RUN mkdir -p /app/data /var/run/raven /etc/raven && \
    chown -R ravenuser:ravenuser /app /var/run/raven /etc/raven

# Switch to non-root user
USER ravenuser

# Expose IMAP ports
EXPOSE 143 993

# Set environment variables
ENV DB_PATH=/app/data/databases \
    SERVICE_NAME=imap

# Health check for IMAP service
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 143 || exit 1

# Start IMAP service
ENTRYPOINT ["/app/entrypoint.sh"]

# ============================================================================
# Stage 6: Combined Image (All Services)
# ============================================================================
FROM base AS raven

# Copy all binaries from builder
COPY --from=builder /app/imap-server .
COPY --from=builder /app/raven-delivery .
COPY --from=builder /app/raven-sasl .

# Copy combined entrypoint script
COPY ./scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && \
    chown ravenuser:ravenuser /app/entrypoint.sh

# Create directories with proper permissions
RUN mkdir -p /app/data /var/run/raven /etc/raven /var/spool/postfix/private && \
    chown -R ravenuser:ravenuser /app /var/run/raven /etc/raven && \
    chmod 777 /var/spool/postfix/private

# Switch to non-root user
USER ravenuser

# Expose all service ports
# IMAP: 143 (plaintext), 993 (TLS)
# LMTP: 24
# SASL: 12345 (TCP)
EXPOSE 143 993 24 12345

# Set environment variables
ENV DB_PATH=/app/data/databases \
    SERVICE_NAME=all

# Health check - check all services
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 143 && nc -z localhost 24 && nc -z localhost 12345 || exit 1

# Start all services or a specific one based on SERVICE environment variable
# Usage:
#   docker run -e SERVICE=imap ...     # Run only IMAP
#   docker run -e SERVICE=lmtp ...     # Run only LMTP
#   docker run -e SERVICE=sasl ...     # Run only SASL
#   docker run ...                     # Run all services (default)
ENTRYPOINT ["/app/entrypoint.sh"]