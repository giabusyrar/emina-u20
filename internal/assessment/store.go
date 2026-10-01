package assessment

import (
	"context"

	"gorm.io/gorm"
)

// Filter selects stored assessments. BrandID and ApplicationID are always
// required; CustomerID narrows to one customer. Newest first.
type Filter struct {
	BrandID       string
	ApplicationID string
	CustomerID    string
	Limit         int
}

type Store interface {
	Save(ctx context.Context, r *Record) error
	List(ctx context.Context, f Filter) ([]Record, error)
}

type GormStore struct{ db *gorm.DB }

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) Save(ctx context.Context, r *Record) error {
	return s.db.WithContext(ctx).Create(r).Error
}

func (s *GormStore) List(ctx context.Context, f Filter) ([]Record, error) {
	q := s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ?", f.BrandID, f.ApplicationID)
	if f.CustomerID != "" {
		q = q.Where("customer_id = ?", f.CustomerID)
	}
	var out []Record
	err := q.Order("created_at desc").Limit(f.Limit).Find(&out).Error
	return out, err
}
