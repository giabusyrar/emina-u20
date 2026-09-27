package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/score/adapters/cache"
	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
)

type RulesetRepository interface {
	GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error)
	ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error)
	GetRulesetByID(ctx context.Context, id string) (*entity.ScoreRuleset, error)
	SaveRuleset(ctx context.Context, item *entity.ScoreRuleset) error
	DeleteRuleset(ctx context.Context, id string) error
}

type RulesetRepo struct {
	db    *gorm.DB
	cache *cache.ScoreCache
}

func NewRulesetRepo(db *gorm.DB, c ...*cache.ScoreCache) *RulesetRepo {
	var sc *cache.ScoreCache
	if len(c) > 0 {
		sc = c[0]
	}
	return &RulesetRepo{db: db, cache: sc}
}

// GetActiveRuleset finds the best matching active JDM ruleset with tenant fallback hierarchy
func (r *RulesetRepo) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error) {
	if brandID == "" {
		brandID = "*"
	}
	if appID == "" {
		appID = "*"
	}
	if code == "" {
		code = "baumann_16_types"
	}

	cacheKey := fmt.Sprintf("ruleset:%s:%s:%s", brandID, appID, code)
	var cached entity.ScoreRuleset
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &cached) && cached.ID != "" {
		return &cached, nil
	}

	if r.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	var ruleset entity.ScoreRuleset

	// 1. Exact match: Brand + App
	err := r.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND code = ? AND UPPER(status) = ?", brandID, appID, code, entity.RulesetStatusActive).
		Order("version desc").
		First(&ruleset).Error

	// 2. Fallback: Brand + Global App
	if err != nil && appID != "*" {
		err = r.db.WithContext(ctx).
			Where("brand_id = ? AND application_id = '*' AND code = ? AND UPPER(status) = ?", brandID, code, entity.RulesetStatusActive).
			Order("version desc").
			First(&ruleset).Error
	}

	// 3. Fallback: Global Brand + Global App
	if err != nil && brandID != "*" {
		err = r.db.WithContext(ctx).
			Where("brand_id = '*' AND application_id = '*' AND code = ? AND UPPER(status) = ?", code, entity.RulesetStatusActive).
			Order("version desc").
			First(&ruleset).Error
	}

	// 4. Default fallback: Baumann default model
	if err != nil {
		err = r.db.WithContext(ctx).
			Where("code = ? AND UPPER(status) = ?", "baumann_16_types", entity.RulesetStatusActive).
			Order("version desc").
			First(&ruleset).Error
		if err != nil {
			return nil, fmt.Errorf("ruleset not found: brand=%s app=%s code=%s", brandID, appID, code)
		}
	}

	if r.cache != nil && ruleset.ID != "" {
		r.cache.Set(ctx, cacheKey, ruleset)
	}

	return &ruleset, nil
}

// ListRulesets lists all rulesets for brand / application
func (r *RulesetRepo) ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error) {
	if r.db == nil {
		return []entity.ScoreRuleset{}, nil
	}

	var items []entity.ScoreRuleset
	query := r.db.WithContext(ctx).Order("updated_at desc")

	if brandID != "" && brandID != "*" {
		query = query.Where("brand_id = ? OR brand_id = '*'", brandID)
	}
	if appID != "" && appID != "*" {
		query = query.Where("application_id = ? OR application_id = '*'", appID)
	}

	err := query.Find(&items).Error
	if err != nil {
		return nil, err
	}

	return items, nil
}

// GetRulesetByID retrieves a ruleset by ID
func (r *RulesetRepo) GetRulesetByID(ctx context.Context, id string) (*entity.ScoreRuleset, error) {
	if r.db == nil {
		return nil, fmt.Errorf("ruleset not found: id=%s", id)
	}

	var item entity.ScoreRuleset
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, fmt.Errorf("ruleset not found: id=%s", id)
	}
	return &item, nil
}

// SaveRuleset creates or updates a ruleset
func (r *RulesetRepo) SaveRuleset(ctx context.Context, item *entity.ScoreRuleset) error {
	if r.db == nil {
		return nil
	}

	if item.BrandID == "" {
		item.BrandID = "*"
	}
	if item.ApplicationID == "" {
		item.ApplicationID = "*"
	}
	item.Status = entity.NormalizeRulesetStatus(item.Status)

	now := time.Now()
	item.UpdatedAt = &now

	err := r.db.WithContext(ctx).Save(item).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, fmt.Sprintf("ruleset:%s:%s:%s", item.BrandID, item.ApplicationID, item.Code))
		r.cache.Invalidate(ctx, fmt.Sprintf("ruleset:*:%s:%s", item.ApplicationID, item.Code))
		r.cache.Invalidate(ctx, fmt.Sprintf("ruleset:%s:*:%s", item.BrandID, item.Code))
		r.cache.Invalidate(ctx, fmt.Sprintf("ruleset:*:*:%s", item.Code))
	}
	return err
}

// DeleteRuleset deletes a ruleset by ID
func (r *RulesetRepo) DeleteRuleset(ctx context.Context, id string) error {
	if r.db == nil {
		return nil
	}

	var existing entity.ScoreRuleset
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&existing).Error; err == nil {
		if r.cache != nil {
			r.cache.Invalidate(ctx, fmt.Sprintf("ruleset:%s:%s:%s", existing.BrandID, existing.ApplicationID, existing.Code))
		}
	}

	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&entity.ScoreRuleset{}).Error
}
