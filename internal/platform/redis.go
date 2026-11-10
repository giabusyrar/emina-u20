package goplatform

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// OpenRedis returns (nil, nil) when url is empty: Redis is optional, and
// every cache treats a nil client as a miss. A set but unreachable url
// returns (nil, err). The caller logs it and runs without cache, and
// /health reports redis "down" until restart.
//
// A url that does not parse as redis:// is used as a bare host:port, as
// both services did before.
func OpenRedis(ctx context.Context, url string, pingTimeout time.Duration) (*redis.Client, error) {
	if url == "" {
		return nil, nil
	}
	var client *redis.Client
	if opt, err := redis.ParseURL(url); err == nil {
		client = redis.NewClient(opt)
	} else {
		client = redis.NewClient(&redis.Options{Addr: url})
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
