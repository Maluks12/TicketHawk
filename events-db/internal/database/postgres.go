package database

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(databaseURL string) (*pgxpool.Pool, error) {
	ctx := context.Background()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Printf("Unable to parse DATABASE_URL: %v", err)
		return nil, err
	}

	// ── Pool sizing ───────────────────────────────────────────────────────────
	// Postgres opens one backend process per connection, and a small RDS instance
	// has a low max_connections ceiling (~80 on db.t4g.micro, ~170 on db.t4g.small).
	// Keep MaxConns comfortably under that, leaving headroom for migrations, the
	// background workers, ad-hoc psql sessions, and RDS's own reserved slots.
	// When you run MULTIPLE API instances, remember the ceiling is shared:
	// (instances × DB_MAX_CONNS) must stay under RDS max_connections.
	// Tune per environment with DB_MAX_CONNS / DB_MIN_CONNS.
	config.MaxConns = envInt32("DB_MAX_CONNS", 20)
	config.MinConns = envInt32("DB_MIN_CONNS", 2)
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Printf("Unable to create connection pool: %v", err)
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		log.Printf("Unable to ping database: %v", err)
		pool.Close()
		return nil, err
	}

	log.Printf("Successfully connected to PostgreSQL (max_conns=%d)", config.MaxConns)
	return pool, nil
}

func envInt32(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int32(n)
		}
	}
	return def
}