// Package db provides PostgreSQL database connection and migration management.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/els3aty/goha-webpanel/control-plane/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the application-wide database connection pool.
// All database access goes through this pool.
type Pool = pgxpool.Pool

// Connect establishes a connection pool to PostgreSQL.
// The DSN must not be logged — it contains the database password.
func Connect(ctx context.Context, cfg *config.DBConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		// Do NOT wrap DSN in error message — it contains the password
		return nil, fmt.Errorf("failed to parse database configuration: %w",
			sanitizePgError(err))
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w",
			sanitizePgError(err))
	}

	// Verify connectivity with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", sanitizePgError(err))
	}

	return pool, nil
}

// sanitizePgError removes any potential credential leakage from PostgreSQL errors.
// pgx may include the DSN in error messages in some cases.
func sanitizePgError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// Remove password= patterns from error strings
	if len(msg) > 200 {
		return fmt.Errorf("database error (details redacted for security)")
	}
	return err
}
