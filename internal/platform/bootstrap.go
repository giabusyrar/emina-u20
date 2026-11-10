package goplatform

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Deps are the connections every Go service here starts with.
type Deps struct {
	Service string
	Log     *slog.Logger
	DB      *gorm.DB
	// Redis is nil when REDIS_URL is unset or Redis was unreachable at startup.
	Redis *redis.Client

	redisConfigured bool
	pingTimeout     time.Duration
}

// Bootstrap sets up JSON logging (and makes it slog's default), opens
// Postgres (required) and opens Redis (optional).
func Bootstrap(ctx context.Context, service string, c CommonConfig) (*Deps, error) {
	log, err := NewLogger(os.Stdout, service, c.LogLevel)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(log)

	db, err := OpenPostgres(ctx, c.DatabaseURL, DBOptions{
		Logger:             log,
		SlowQueryThreshold: c.DBSlowQueryThreshold,
		PingTimeout:        c.DependencyPingTimeout,
	})
	if err != nil {
		return nil, err
	}
	log.Info("connected to postgres")

	rdb, err := OpenRedis(ctx, c.RedisURL, c.DependencyPingTimeout)
	switch {
	case err != nil:
		log.Warn("redis unreachable at startup; cache disabled until restart", "err", err)
	case rdb == nil:
		log.Info("REDIS_URL unset; cache disabled")
	default:
		log.Info("connected to redis")
	}

	return &Deps{
		Service:         service,
		Log:             log,
		DB:              db,
		Redis:           rdb,
		redisConfigured: c.RedisURL != "",
		pingTimeout:     c.DependencyPingTimeout,
	}, nil
}

func (d *Deps) HealthHandler() echo.HandlerFunc {
	return HealthHandler(d.Service, d.pingTimeout, DatabaseCheck(d.DB), RedisCheck(d.Redis, d.redisConfigured))
}

func (d *Deps) Close() {
	if sqlDB, err := d.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if d.Redis != nil {
		_ = d.Redis.Close()
	}
}
