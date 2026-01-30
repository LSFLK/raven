#!/bin/bash
set -e

echo "Starting Delivery service (LMTP)..."
exec ./raven-delivery -db ${DB_PATH:-/app/data/databases}