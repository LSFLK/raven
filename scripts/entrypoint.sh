#!/bin/bash
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