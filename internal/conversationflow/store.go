package conversationflow

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Store persists flows. Get and Active return nil, nil when nothing matches.
type Store interface {
	Get(ctx context.Context, id string) (*Flow, error)
	List(ctx context.Context, brandID, applicationID string) ([]Flow, error)
	Active(ctx context.Context, brandID, applicationID, surveyCode string) (*Flow, error)
	Create(ctx context.Context, f *Flow) error
	Update(ctx context.Context, f *Flow) error
}

type GormStore struct{ db *gorm.DB }

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) first(q *gorm.DB) (*Flow, error) {
	var f Flow
	err := q.First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *GormStore) Get(ctx context.Context, id string) (*Flow, error) {
	return s.first(s.db.WithContext(ctx).Where("id = ?", id))
}

func (s *GormStore) List(ctx context.Context, brandID, applicationID string) ([]Flow, error) {
	var out []Flow
	err := s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ?", brandID, applicationID).
		Order("survey_code").Order("version desc").
		Find(&out).Error
	return out, err
}

func (s *GormStore) Active(ctx context.Context, brandID, applicationID, surveyCode string) (*Flow, error) {
	return s.first(s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND survey_code = ? AND status = ?", brandID, applicationID, surveyCode, StatusActive).
		Order("version desc"))
}

func (s *GormStore) Create(ctx context.Context, f *Flow) error {
	return s.db.WithContext(ctx).Create(f).Error
}

func (s *GormStore) Update(ctx context.Context, f *Flow) error {
	return s.db.WithContext(ctx).Save(f).Error
}
