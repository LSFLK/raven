#!/bin/bash
set -e

echo "Starting SASL authentication service..."
exec ./raven-sasl -tcp :12345 -config /etc/raven/raven.yaml