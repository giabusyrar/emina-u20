package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
)

type SurveyCache struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSurveyCache(rdb *redis.Client, ttl time.Duration) *SurveyCache {
	return &SurveyCache{rdb: rdb, ttl: ttl}
}

func surveyKey(brandID, applicationID, code string) string {
	return fmt.Sprintf("survey:%s:%s:%s", brandID, applicationID, code)
}

func (c *SurveyCache) Get(ctx context.Context, brandID, applicationID, code string) (*domain.SurveyModel, bool) {
	if c.rdb == nil {
		return nil, false
	}
	raw, err := c.rdb.Get(ctx, surveyKey(brandID, applicationID, code)).Bytes()
	if err != nil {
		return nil, false
	}
	var s domain.SurveyModel
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	return &s, true
}

func (c *SurveyCache) Set(ctx context.Context, brandID, applicationID, code string, s *domain.SurveyModel) {
	if c.rdb == nil {
		return
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = c.rdb.Set(ctx, surveyKey(brandID, applicationID, code), raw, c.ttl).Err()
}

func (c *SurveyCache) Invalidate(ctx context.Context, brandID, applicationID, code string) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, surveyKey(brandID, applicationID, code)).Err()
}
