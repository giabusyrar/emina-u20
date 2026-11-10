package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DBOptions struct {
	Logger             *slog.Logger
	SlowQueryThreshold time.Duration
	PingTimeout        time.Duration
}

// OpenPostgres opens and pings Postgres, and returns an error if either
// fails or if the database is Seagull-gateway's (see refuseGatewayDatabase). There is no degraded mode: a service that cannot reach its database
// must not start and report itself healthy.
func OpenPostgres(ctx context.Context, url string, o DBOptions) (*gorm.DB, error) {
	if url == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: NewGormLogger(o.Logger, o.SlowQueryThreshold),
		// GORM's own ping has no deadline. Ping below with PingTimeout instead.
		DisableAutomaticPing: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres handle: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, o.PingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := refuseGatewayDatabase(pingCtx, sqlTableExists(sqlDB)); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}
