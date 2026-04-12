#!/bin/sh
set -e

echo "â–¶ Running database migrations..."
migrate -path ./migrations -database "$DATABASE_URL" up
echo "âœ“ Migrations complete"

echo "â–¶ Starting API server..."
exec ./server