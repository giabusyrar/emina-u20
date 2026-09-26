package goplatform

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type CheckState string

const (
	CheckUp       CheckState = "up"
	CheckDown     CheckState = "down"
	CheckDisabled CheckState = "disabled"
)

// Check is one dependency probed by /health. Only a Required check that is
// down turns the response into 503.
type Check struct {
	Name     string
	Required bool
	Probe    func(ctx context.Context) CheckState
}

type HealthResponse struct {
	Status  string                `json:"status"`
	Service string                `json:"service"`
	Checks  map[string]CheckState `json:"checks"`
}

// HealthHandler runs every check within timeout. It answers 200 "ok", or
// 503 "degraded" when a required check is down.
func HealthHandler(service string, timeout time.Duration, checks ...Check) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
		defer cancel()

		resp := HealthResponse{Status: "ok", Service: service, Checks: make(map[string]CheckState, len(checks))}
		code := http.StatusOK
		for _, chk := range checks {
			state := chk.Probe(ctx)
			resp.Checks[chk.Name] = state
			if chk.Required && state == CheckDown {
				resp.Status = "degraded"
				code = http.StatusServiceUnavailable
			}
		}
		return c.JSON(code, resp)
	}
}

func DatabaseCheck(db *gorm.DB) Check {
	return Check{Name: "database", Required: true, Probe: func(ctx context.Context) CheckState {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			return CheckDown
		}
		return CheckUp
	}}
}

// RedisCheck reports disabled when REDIS_URL is unset, and down when it is
// set but the client is nil (unreachable at startup) or does not answer.
func RedisCheck(rdb *redis.Client, configured bool) Check {
	return Check{Name: "redis", Probe: func(ctx context.Context) CheckState {
		switch {
		case !configured:
			return CheckDisabled
		case rdb == nil, rdb.Ping(ctx).Err() != nil:
			return CheckDown
		default:
			return CheckUp
		}
	}}
}
