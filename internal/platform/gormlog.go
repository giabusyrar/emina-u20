package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// gormSlog routes GORM's logging through slog. It keeps the behaviour both
// services configured before: warn level, slow queries above a threshold
// logged as warnings, and record-not-found not treated as an error.
type gormSlog struct {
	log   *slog.Logger
	slow  time.Duration
	level gormlogger.LogLevel
}

// NewGormLogger returns a GORM logger.Interface that routes GORM's log
// output through log, at Warn level, treating queries slower than slow as
// warnings and gorm.ErrRecordNotFound as not an error.
func NewGormLogger(log *slog.Logger, slow time.Duration) gormlogger.Interface {
	return &gormSlog{log: log, slow: slow, level: gormlogger.Warn}
}

func (g *gormSlog) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *g
	c.level = level
	return &c
}

func (g *gormSlog) Info(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Info {
		g.log.InfoContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Warn(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Warn {
		g.log.WarnContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Error(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= gormlogger.Error {
		g.log.ErrorContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormSlog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	switch {
	case err != nil && g.level >= gormlogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		g.log.ErrorContext(ctx, "gorm query failed", "err", err, "elapsed", elapsed, "rows", rows, "sql", sql)
	case g.slow > 0 && elapsed > g.slow && g.level >= gormlogger.Warn:
		sql, rows := fc()
		g.log.WarnContext(ctx, "gorm slow query", "elapsed", elapsed, "threshold", g.slow, "rows", rows, "sql", sql)
	case g.level >= gormlogger.Info:
		sql, rows := fc()
		g.log.DebugContext(ctx, "gorm query", "elapsed", elapsed, "rows", rows, "sql", sql)
	}
}
