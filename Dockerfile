# Stage 1: build
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build tools for CGO (required for SQLite)
RUN apk add --no-cache git build-base sqlite-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source code
COPY . .

# Enable CGO for go-sqlite3
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64

# Build the application from the cmd/server entry point
RUN go build -a -o imap-server ./cmd/server

# Stage 2: runtime
FROM alpine:3.18

WORKDIR /app

# Install required runtime dependencies
RUN apk add --no-cache sqlite tzdata netcat-openbsd \
    && rm -rf /var/cache/apk/*

# Create a non-root user
RUN addgroup -g 1001 -S imapuser && \
    adduser -u 1001 -S imapuser -G imapuser

# Copy the binary from builder
COPY --from=builder /app/imap-server .

# Create directory for database with proper permissions
RUN mkdir -p /app/data && chown -R imapuser:imapuser /app

# Switch to non-root user
USER imapuser

# Expose IMAP port
EXPOSE 143 993

# Set environment variables
ENV DB_FILE=/app/data/mails.db

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD nc -z localhost 143 || exit 1

# Start the server
ENTRYPOINT ["./imap-server"]