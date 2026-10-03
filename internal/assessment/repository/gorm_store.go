package repository

import (
	"context"

	"gorm.io/gorm"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/service"
)

type GormStore struct{ db *gorm.DB }

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) Save(ctx context.Context, r *entity.Record) error {
	return s.db.WithContext(ctx).Create(r).Error
}

func (s *GormStore) List(ctx context.Context, f service.Filter) ([]entity.Record, error) {
	q := s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ?", f.BrandID, f.ApplicationID)
	if f.CustomerID != "" {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	var out []entity.Record
	err := q.Order("created_at desc").Limit(f.Limit).Find(&out).Error
	return out, err
}
