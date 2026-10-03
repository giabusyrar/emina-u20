package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/entity"
)

type SurveyRepo struct {
	db *gorm.DB
}

func NewSurveyRepo(db *gorm.DB) *SurveyRepo {
	return &SurveyRepo{db: db}
}

func (r *SurveyRepo) GetActive(ctx context.Context, brandID, applicationID, code string) (*entity.SurveyDefinition, error) {
	var row entity.SurveyDefinition
	err := r.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND code = ? AND LOWER(status) = ?", brandID, applicationID, code, entity.SurveyStatusActive).
		Order("version desc").
		First(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SurveyRepo) GetByCode(ctx context.Context, brandID, applicationID, code string) (*entity.SurveyDefinition, error) {
	var row entity.SurveyDefinition
	err := r.db.WithContext(ctx).
		Where("brand_id = ? AND application_id = ? AND code = ?", brandID, applicationID, code).
		Order("version desc").
		First(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SurveyRepo) List(ctx context.Context, brandID, applicationID string) ([]entity.SurveyDefinition, error) {
	var rows []entity.SurveyDefinition
	q := r.db.WithContext(ctx)
	if brandID != "" {
		q = q.Where("brand_id = ?", brandID)
	}
	if applicationID != "" {
		q = q.Where("application_id = ?", applicationID)
	}
	err := q.Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (r *SurveyRepo) Save(ctx context.Context, survey *entity.SurveyDefinition) error {
	// Upsert on the tenant identity so re-saving an existing questionnaire updates
	// it in place instead of colliding with idx_survey_tenant
	// (brand_id, application_id, code, version).
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "brand_id"}, {Name: "application_id"}, {Name: "code"}, {Name: "version"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "status", "schema", "updated_at"}),
		}).
		Create(survey).Error
}
