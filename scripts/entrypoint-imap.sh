#!/bin/bash
set -e

echo "Starting IMAP server..."
exec ./imap-server -db ${DB_PATH:-/app/data/databases}