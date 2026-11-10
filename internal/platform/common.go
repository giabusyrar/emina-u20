package goplatform

import (
	"errors"
	"time"
)

// Defaults shared by both Go services. Each is the literal that sat in
// cmd/server/main.go before the 2026-09-26 standardization, moved here
// unchanged so that no deployment has to set a new variable.
const (
	DefaultLogLevel = "info"
	// reference-service main.go, e.Shutdown timeout.
	DefaultShutdownTimeout = 10 * time.Second
	// Both main.go files, gormlogger.Config.SlowThreshold.
	DefaultDBSlowQueryThreshold = 2 * time.Second
	// Both main.go files, Redis startup ping. Now also bounds the Postgres
	// startup ping and each /health probe.
	DefaultDependencyPingTimeout = 3 * time.Second
)

// DefaultCORSAllowOrigins is what both services allowed before standardization.
var DefaultCORSAllowOrigins = []string{"*"}

// CommonConfig is the configuration every Go service here loads the same way.
type CommonConfig struct {
	DatabaseURL           string
	RedisURL              string
	LogLevel              string
	CORSAllowOrigins      []string
	CORSAllowHeaders      []string
	ShutdownTimeout       time.Duration
	DBSlowQueryThreshold  time.Duration
	DependencyPingTimeout time.Duration
}

// CommonDefaults are the per-service defaults for CommonConfig fields whose
// pre-standardization value differed between services.
type CommonDefaults struct {
	CORSAllowHeaders []string
}

// LoadCommon reads CommonConfig from the process environment. It does not
// load .env files; each service's config.Load does that first.
func LoadCommon(d CommonDefaults) (CommonConfig, error) {
	c := CommonConfig{
		DatabaseURL:      First("DATABASE_URL"),
		RedisURL:         First("REDIS_URL"),
		LogLevel:         First("LOG_LEVEL"),
		CORSAllowOrigins: List("CORS_ALLOW_ORIGINS", DefaultCORSAllowOrigins),
		CORSAllowHeaders: List("CORS_ALLOW_HEADERS", d.CORSAllowHeaders),
	}
	if c.DatabaseURL == "" {
		return CommonConfig{}, errors.New("DATABASE_URL is required")
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
	if _, err := ParseLevel(c.LogLevel); err != nil {
		return CommonConfig{}, err
	}

	var err error
	for _, dur := range []struct {
		key string
		def time.Duration
		dst *time.Duration
	}{
		{"SHUTDOWN_TIMEOUT", DefaultShutdownTimeout, &c.ShutdownTimeout},
		{"DB_SLOW_QUERY_THRESHOLD", DefaultDBSlowQueryThreshold, &c.DBSlowQueryThreshold},
		{"DEPENDENCY_PING_TIMEOUT", DefaultDependencyPingTimeout, &c.DependencyPingTimeout},
	} {
		if *dur.dst, err = Duration(dur.key, dur.def); err != nil {
			return CommonConfig{}, err
		}
	}
	return c, nil
}
