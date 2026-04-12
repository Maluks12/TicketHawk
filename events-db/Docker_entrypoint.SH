#!/bin/sh
set -e

echo "▶ Running database migrations..."
migrate -path ./migrations -database "$DATABASE_URL" up
echo "✓ Migrations complete"

echo "▶ Starting API server..."
exec ./server