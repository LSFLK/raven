#!/bin/bash
set -e

# Raven Combined Services Entrypoint
# Supports running all services or a single service based on SERVICE environment variable
# 
# Usage:
#   SERVICE=all    - Run all services (default)
#   SERVICE=imap   - Run only IMAP server
#   SERVICE=lmtp   - Run only LMTP delivery service
#   SERVICE=sasl   - Run only SASL authentication service

SERVICE=${SERVICE:-all}

# Function to start SASL service
start_sasl() {
    echo "Starting SASL authentication service..."
    exec ./raven-sasl -tcp :12345 -config /etc/raven/raven.yaml
}

# Function to start IMAP service
start_imap() {
    echo "Starting IMAP server..."
    exec ./imap-server -db ${DB_PATH:-/app/data/databases}
}

# Function to start LMTP service
start_lmtp() {
    echo "Starting Delivery service (LMTP)..."
    exec ./raven-delivery -db ${DB_PATH:-/app/data/databases}
}

# Function to start all services
start_all() {
    echo "Starting Raven services..."

    echo "Starting SASL authentication service..."
    ./raven-sasl -tcp :12345 -config /etc/raven/raven.yaml &
    SASL_PID=$!
    echo "SASL service started with PID: $SASL_PID (TCP :12345)"
    sleep 1

    echo "Starting IMAP server..."
    ./imap-server -db ${DB_PATH:-/app/data/databases} &
    IMAP_PID=$!
    echo "IMAP server started with PID: $IMAP_PID"
    sleep 1

    echo "Starting Delivery service (LMTP)..."
    ./raven-delivery -db ${DB_PATH:-/app/data/databases} &
    DELIVERY_PID=$!
    echo "Delivery service started with PID: $DELIVERY_PID"

    echo ""
    echo "==================================="
    echo "All Raven services started:"
    echo "  SASL Auth: PID $SASL_PID (TCP :12345)"
    echo "  IMAP:      PID $IMAP_PID"
    echo "  LMTP:      PID $DELIVERY_PID"
    echo "  DB Path:   ${DB_PATH:-/app/data/databases}"
    echo "==================================="

    wait
}

# Route to appropriate service based on SERVICE variable
case "$SERVICE" in
    sasl)
        start_sasl
        ;;
    imap)
        start_imap
        ;;
    lmtp)
        start_lmtp
        ;;
    all)
        start_all
        ;;
    *)
        echo "Error: Unknown SERVICE value: $SERVICE"
        echo "Valid options: sasl, imap, lmtp, all"
        exit 1
        ;;
esac