package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type MatchCache struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewMatchCache(rdb *redis.Client, ttl time.Duration) *MatchCache {
	return &MatchCache{
		rdb: rdb,
		ttl: ttl,
	}
}

func (c *MatchCache) Get(ctx context.Context, key string, target interface{}) bool {
	if c == nil || c.rdb == nil {
		return false
	}
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(data, target) == nil
}

func (c *MatchCache) Set(ctx context.Context, key string, val interface{}, ttl ...time.Duration) {
	if c == nil || c.rdb == nil {
		return
	}
	d, err := json.Marshal(val)
	if err != nil {
		return
	}
	t := c.ttl
	if len(ttl) > 0 && ttl[0] > 0 {
		t = ttl[0]
	}
	_ = c.rdb.Set(ctx, key, d, t).Err()
}

func (c *MatchCache) Invalidate(ctx context.Context, keys ...string) {
	if c == nil || c.rdb == nil || len(keys) == 0 {
		return
	}
	_ = c.rdb.Del(ctx, keys...).Err()
}

func (c *MatchCache) InvalidatePrefix(ctx context.Context, prefix string) {
	if c == nil || c.rdb == nil || prefix == "" {
		return
	}
	iter := c.rdb.Scan(ctx, 0, prefix+"*", 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 100 {
			_ = c.rdb.Del(ctx, keys...).Err()
			keys = keys[:0]
		}
	}
	if len(keys) > 0 {
		_ = c.rdb.Del(ctx, keys...).Err()
	}
}
