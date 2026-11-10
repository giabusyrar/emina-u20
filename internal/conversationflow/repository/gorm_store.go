package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/service"
)

type GormStore struct{ db *gorm.DB }

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) first(q *gorm.DB) (*entity.Flow, error) {
	var f entity.Flow
	err := q.First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *GormStore) Get(ctx context.Context, id string) (*entity.Flow, error) {
	return s.first(s.db.WithContext(ctx).Where("id = ?", id))
}

func (s *GormStore) List(ctx context.Context, brandID, applicationID string) ([]entity.Flow, error) {
	var out []entity.Flow
	err := s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ?", brandID, applicationID).
		Order("survey_code").Order("version desc").
		Find(&out).Error
	return out, err
}

func (s *GormStore) Active(ctx context.Context, brandID, applicationID, surveyCode string) (*entity.Flow, error) {
	return s.first(s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND survey_code = ? AND status = ?", brandID, applicationID, surveyCode, entity.StatusActive).
		Order("version desc"))
}

func (s *GormStore) Version(ctx context.Context, brandID, applicationID, surveyCode string, version int) (*entity.Flow, error) {
	return s.first(s.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND survey_code = ? AND version = ?", brandID, applicationID, surveyCode, version))
}

// Create and Update return service.ErrDuplicateVersion for a unique-index
// violation on (brand, application, survey, version), so no SQL reaches the
// caller.
func (s *GormStore) Create(ctx context.Context, f *entity.Flow) error {
	return translate(s.db.WithContext(ctx).Create(f).Error)
}

func (s *GormStore) Update(ctx context.Context, f *entity.Flow) error {
	return translate(s.db.WithContext(ctx).Save(f).Error)
}

func translate(err error) error {
	if isUniqueViolation(err) {
		return service.ErrDuplicateVersion
	}
	return err
}

// sqlUniqueViolation is Postgres SQLSTATE unique_violation.
const sqlUniqueViolation = "23505"

// isUniqueViolation reports a duplicate-key error. goplatform.OpenPostgres does
// not enable GORM's TranslateError, so the pgx error (*pgconn.PgError, which has
// SQLState) arrives as is; gorm.ErrDuplicatedKey covers a translating config.
func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var state interface{ SQLState() string }
	return errors.As(err, &state) && state.SQLState() == sqlUniqueViolation
}
