package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/match/adapters/cache"
	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
)

type MatchRepo struct {
	db        *gorm.DB
	cache     *cache.MatchCache
	memMu     sync.RWMutex
	useMemory bool

	// In-Memory fallback tables
	products      []entity.ProductCatalogItem
	conflicts     []entity.ConflictMatrixRule
	productGroups []entity.ProductGroup
	shades        []entity.Shade
	shadeAssets   []entity.ShadeAsset
}

func NewMatchRepo(db *gorm.DB, c ...*cache.MatchCache) *MatchRepo {
	var mc *cache.MatchCache
	if len(c) > 0 {
		mc = c[0]
	}
	return &MatchRepo{
		db:            db,
		cache:         mc,
		useMemory:     db == nil,
		products:      []entity.ProductCatalogItem{},
		conflicts:     []entity.ConflictMatrixRule{},
		productGroups: []entity.ProductGroup{},
		shades:        []entity.Shade{},
		shadeAssets:   []entity.ShadeAsset{},
	}
}

// -------------------------------------------------------------
// Product Master Catalog (read-only; written by reference-service)
// -------------------------------------------------------------
// productsQuery reads the product master reference-service owns
// (ref_products, docs/superpowers/specs/2026-10-01-reference-products-design.md).
// A product without a category or texture gets an empty one, never a guess.
func productsQuery(brandID string) (string, []any) {
	q := `
		SELECT
			p.id AS id,
			p.brand_id::text AS brand_id,
			p.name AS name,
			COALESCE(c.name, '') AS category,
			COALESCE(t.name, '') AS texture,
			COALESCE(
				(SELECT string_agg(ing.name, ', ')
				 FROM ref_product_ingredients rpi
				 JOIN ref_ingredients ing ON ing.id = rpi.ingredient_id
				 WHERE rpi.product_id = p.id), ''
			) AS ingredients,
			COALESCE(p.description, '') AS key_actives,
			COALESCE(p.image_url, '') AS image_url,
			p.is_active AS is_active,
			p.created_at AS created_at,
			p.updated_at AS updated_at
		FROM ref_products p
		LEFT JOIN ref_brands b ON b.id = p.brand_id
		LEFT JOIN ref_categories c ON c.id = p.category_id
		LEFT JOIN ref_textures t ON t.id = p.texture_id
		WHERE p.deleted_at IS NULL AND p.is_active`
	if brandID == "" || brandID == "*" {
		return q + " ORDER BY c.name ASC, p.name ASC", nil
	}
	return q + " AND (LOWER(p.brand_id::text) = LOWER(?) OR LOWER(b.name) = LOWER(?)) ORDER BY c.name ASC, p.name ASC",
		[]any{brandID, brandID}
}

func (r *MatchRepo) ListProducts(ctx context.Context, brandID string) ([]entity.ProductCatalogItem, error) {
	cacheKey := fmt.Sprintf("match:products:%s", strings.ToLower(brandID))
	var cached []entity.ProductCatalogItem
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &cached) && len(cached) > 0 {
		return cached, nil
	}

	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		var res []entity.ProductCatalogItem
		for _, p := range r.products {
			if !p.IsActive {
				continue
			}
			if brandID == "" || brandID == "*" || strings.EqualFold(p.BrandID, brandID) {
				res = append(res, p)
			}
		}
		if r.cache != nil && len(res) > 0 {
			r.cache.Set(ctx, cacheKey, res)
		}
		return res, nil
	}

	var res []entity.ProductCatalogItem
	q, args := productsQuery(brandID)
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&res).Error; err != nil {
		return nil, err
	}
	if r.cache != nil && len(res) > 0 {
		r.cache.Set(ctx, cacheKey, res)
	}
	return res, nil
}

// AddInMemoryProduct loads a product into the in-memory repo (no database:
// tests and local runs without Postgres). With a database it refuses:
// products are written through reference-service, and a write here would
// vanish on restart while looking saved.
func (r *MatchRepo) AddInMemoryProduct(ctx context.Context, product entity.ProductCatalogItem) (*entity.ProductCatalogItem, error) {
	if !r.useMemory {
		return nil, fmt.Errorf("match products are read-only here; write them through reference-service /api/reference/products")
	}
	if product.ID == "" {
		product.ID = fmt.Sprintf("prod-%d", time.Now().UnixNano())
	}
	product.IsActive = true
	product.CreatedAt = time.Now()

	r.memMu.Lock()
	defer r.memMu.Unlock()
	r.products = append([]entity.ProductCatalogItem{product}, r.products...)
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:products:")
	}
	return &product, nil
}

// -------------------------------------------------------------
// Conflict Matrix Rules CRUD
// -------------------------------------------------------------
func (r *MatchRepo) ListConflictRules(ctx context.Context, brandID, appID string) ([]entity.ConflictMatrixRule, error) {
	cacheKey := fmt.Sprintf("match:conflicts:%s:%s", brandID, appID)
	var cached []entity.ConflictMatrixRule
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &cached) && len(cached) > 0 {
		return cached, nil
	}

	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		var res []entity.ConflictMatrixRule
		for _, c := range r.conflicts {
			if matchesScope(c.BrandID, c.ApplicationID, brandID, appID) {
				res = append(res, c)
			}
		}
		if r.cache != nil && len(res) > 0 {
			r.cache.Set(ctx, cacheKey, res)
		}
		return res, nil
	}

	var res []entity.ConflictMatrixRule
	query := r.db.WithContext(ctx)
	if brandID != "" && brandID != "*" {
		query = query.Where("brand_id IN (?, '*')", brandID)
	}
	if appID != "" && appID != "*" {
		query = query.Where("application_id IN (?, '*')", appID)
	}
	if err := query.Order("created_at desc").Find(&res).Error; err != nil {
		return nil, err
	}
	if r.cache != nil && len(res) > 0 {
		r.cache.Set(ctx, cacheKey, res)
	}
	return res, nil
}

func (r *MatchRepo) CreateConflictRule(ctx context.Context, rule entity.ConflictMatrixRule) (*entity.ConflictMatrixRule, error) {
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("conf-%d", time.Now().UnixNano())
	}
	rule.CreatedAt = time.Now()

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		r.conflicts = append([]entity.ConflictMatrixRule{rule}, r.conflicts...)
		if r.cache != nil {
			r.cache.InvalidatePrefix(ctx, "match:conflicts:")
		}
		return &rule, nil
	}

	if err := r.db.WithContext(ctx).Create(&rule).Error; err != nil {
		return nil, err
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:conflicts:")
	}
	return &rule, nil
}

func (r *MatchRepo) UpdateConflictRule(ctx context.Context, rule entity.ConflictMatrixRule) (*entity.ConflictMatrixRule, error) {
	now := time.Now()
	rule.UpdatedAt = &now

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, existing := range r.conflicts {
			if existing.ID == rule.ID {
				r.conflicts[i] = rule
				if r.cache != nil {
					r.cache.InvalidatePrefix(ctx, "match:conflicts:")
				}
				return &rule, nil
			}
		}
		r.conflicts = append([]entity.ConflictMatrixRule{rule}, r.conflicts...)
		if r.cache != nil {
			r.cache.InvalidatePrefix(ctx, "match:conflicts:")
		}
		return &rule, nil
	}

	if err := r.db.WithContext(ctx).Save(&rule).Error; err != nil {
		return nil, err
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:conflicts:")
	}
	return &rule, nil
}

func (r *MatchRepo) DeleteConflictRule(ctx context.Context, id string) error {
	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, c := range r.conflicts {
			if c.ID == id {
				r.conflicts = append(r.conflicts[:i], r.conflicts[i+1:]...)
				if r.cache != nil {
					r.cache.InvalidatePrefix(ctx, "match:conflicts:")
				}
				return nil
			}
		}
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Delete(&entity.ConflictMatrixRule{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:conflicts:")
	}
	return nil
}

// -------------------------------------------------------------
// Product Groups CRUD (brand campaign product/category curation)
// -------------------------------------------------------------
func (r *MatchRepo) ListProductGroups(ctx context.Context, brandID, appID string) ([]entity.ProductGroup, error) {
	cacheKey := fmt.Sprintf("match:product_groups:%s:%s", brandID, appID)
	var cached []entity.ProductGroup
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &cached) && len(cached) > 0 {
		return cached, nil
	}

	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		var res []entity.ProductGroup
		for _, g := range r.productGroups {
			if matchesScope(g.BrandID, g.ApplicationID, brandID, appID) {
				res = append(res, g)
			}
		}
		if r.cache != nil && len(res) > 0 {
			r.cache.Set(ctx, cacheKey, res)
		}
		return res, nil
	}

	var res []entity.ProductGroup
	query := r.db.WithContext(ctx)
	if brandID != "" && brandID != "*" {
		query = query.Where("brand_id IN (?, '*')", brandID)
	}
	if appID != "" && appID != "*" {
		query = query.Where("application_id IN (?, '*')", appID)
	}
	if err := query.Order("created_at desc").Find(&res).Error; err != nil {
		return nil, err
	}
	if r.cache != nil && len(res) > 0 {
		r.cache.Set(ctx, cacheKey, res)
	}
	return res, nil
}

func (r *MatchRepo) CreateProductGroup(ctx context.Context, group entity.ProductGroup) (*entity.ProductGroup, error) {
	if group.ID == "" {
		group.ID = fmt.Sprintf("pgrp-%d", time.Now().UnixNano())
	}
	group.CreatedAt = time.Now()

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		r.productGroups = append([]entity.ProductGroup{group}, r.productGroups...)
		if r.cache != nil {
			r.cache.InvalidatePrefix(ctx, "match:product_groups:")
		}
		return &group, nil
	}

	if err := r.db.WithContext(ctx).Create(&group).Error; err != nil {
		return nil, err
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:product_groups:")
	}
	return &group, nil
}

func (r *MatchRepo) UpdateProductGroup(ctx context.Context, group entity.ProductGroup) (*entity.ProductGroup, error) {
	now := time.Now()
	group.UpdatedAt = &now

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, existing := range r.productGroups {
			if existing.ID == group.ID {
				r.productGroups[i] = group
				if r.cache != nil {
					r.cache.InvalidatePrefix(ctx, "match:product_groups:")
				}
				return &group, nil
			}
		}
		r.productGroups = append([]entity.ProductGroup{group}, r.productGroups...)
		if r.cache != nil {
			r.cache.InvalidatePrefix(ctx, "match:product_groups:")
		}
		return &group, nil
	}

	if err := r.db.WithContext(ctx).Save(&group).Error; err != nil {
		return nil, err
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:product_groups:")
	}
	return &group, nil
}

func (r *MatchRepo) DeleteProductGroup(ctx context.Context, id string) error {
	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, g := range r.productGroups {
			if g.ID == id {
				r.productGroups = append(r.productGroups[:i], r.productGroups[i+1:]...)
				if r.cache != nil {
					r.cache.InvalidatePrefix(ctx, "match:product_groups:")
				}
				return nil
			}
		}
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Delete(&entity.ProductGroup{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:product_groups:")
	}
	return nil
}

// -------------------------------------------------------------
// Shades CRUD (per-product color variants feeding tryon-engine)
// -------------------------------------------------------------
func (r *MatchRepo) ListShades(ctx context.Context, productID string) ([]entity.Shade, error) {
	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		var res []entity.Shade
		for _, s := range r.shades {
			if productID == "" || s.ProductID == productID {
				res = append(res, s)
			}
		}
		return res, nil
	}

	var res []entity.Shade
	query := r.db.WithContext(ctx)
	if productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if err := query.Order("created_at desc").Find(&res).Error; err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MatchRepo) GetShadeByID(ctx context.Context, id string) (*entity.Shade, error) {
	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		for _, s := range r.shades {
			if s.ID == id {
				return &s, nil
			}
		}
		return nil, domain.ErrNotFound
	}

	var shade entity.Shade
	if err := r.db.WithContext(ctx).First(&shade, "id = ?", id).Error; err != nil {
		return nil, domain.ErrNotFound
	}
	return &shade, nil
}

func (r *MatchRepo) CreateShade(ctx context.Context, shade entity.Shade) (*entity.Shade, error) {
	if shade.ID == "" {
		shade.ID = fmt.Sprintf("shade-%d", time.Now().UnixNano())
	}
	if shade.ExtractionStatus == "" {
		shade.ExtractionStatus = "pending"
	}
	shade.CreatedAt = time.Now()

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		r.shades = append([]entity.Shade{shade}, r.shades...)
		return &shade, nil
	}

	if err := r.db.WithContext(ctx).Create(&shade).Error; err != nil {
		return nil, err
	}
	return &shade, nil
}

func (r *MatchRepo) UpdateShade(ctx context.Context, shade entity.Shade) (*entity.Shade, error) {
	now := time.Now()
	shade.UpdatedAt = &now

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, existing := range r.shades {
			if existing.ID == shade.ID {
				r.shades[i] = shade
				return &shade, nil
			}
		}
		return nil, domain.ErrNotFound
	}

	if err := r.db.WithContext(ctx).Save(&shade).Error; err != nil {
		return nil, err
	}
	return &shade, nil
}

func (r *MatchRepo) DeleteShade(ctx context.Context, id string) error {
	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		for i, s := range r.shades {
			if s.ID == id {
				r.shades = append(r.shades[:i], r.shades[i+1:]...)
				return nil
			}
		}
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Delete(&entity.Shade{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MatchRepo) SaveShadeAsset(ctx context.Context, asset entity.ShadeAsset) (*entity.ShadeAsset, error) {
	if asset.ID == "" {
		asset.ID = fmt.Sprintf("asset-%d", time.Now().UnixNano())
	}
	asset.CreatedAt = time.Now()

	if r.useMemory || r.db == nil {
		r.memMu.Lock()
		defer r.memMu.Unlock()
		r.shadeAssets = append([]entity.ShadeAsset{asset}, r.shadeAssets...)
		return &asset, nil
	}

	if err := r.db.WithContext(ctx).Create(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *MatchRepo) GetShadeAsset(ctx context.Context, shadeID string) (*entity.ShadeAsset, error) {
	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		for _, a := range r.shadeAssets {
			if a.ShadeID == shadeID {
				return &a, nil
			}
		}
		return nil, domain.ErrNotFound
	}

	var asset entity.ShadeAsset
	if err := r.db.WithContext(ctx).Where("shade_id = ?", shadeID).First(&asset).Error; err != nil {
		return nil, domain.ErrNotFound
	}
	return &asset, nil
}

func (r *MatchRepo) GetShadeAssetByID(ctx context.Context, id string) (*entity.ShadeAsset, error) {
	if r.useMemory || r.db == nil {
		r.memMu.RLock()
		defer r.memMu.RUnlock()
		for _, a := range r.shadeAssets {
			if a.ID == id {
				return &a, nil
			}
		}
		return nil, domain.ErrNotFound
	}

	var asset entity.ShadeAsset
	if err := r.db.WithContext(ctx).First(&asset, "id = ?", id).Error; err != nil {
		return nil, domain.ErrNotFound
	}
	return &asset, nil
}

// Scoping helper function
func matchesScope(entityBrand, entityApp, reqBrand, reqApp string) bool {
	brandMatch := entityBrand == "*" || reqBrand == "" || reqBrand == "*" || entityBrand == reqBrand
	appMatch := entityApp == "*" || reqApp == "" || reqApp == "*" || entityApp == reqApp
	return brandMatch && appMatch
}
