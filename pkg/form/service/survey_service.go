package service

import (
	"context"
	"fmt"
	"time"

	"github.com/experience-gateway/apps/core-engine/pkg/form/adapters/cache"
	"github.com/experience-gateway/apps/core-engine/pkg/form/adapters/scoreclient"
	"github.com/experience-gateway/apps/core-engine/pkg/form/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/form/entity"
	"github.com/experience-gateway/apps/core-engine/pkg/form/repository"
)

type SurveyService struct {
	repo        *repository.SurveyRepo
	cache       *cache.SurveyCache
	scoreClient *scoreclient.Client
}

func NewSurveyService(repo *repository.SurveyRepo, cache *cache.SurveyCache, scoreClient *scoreclient.Client) *SurveyService {
	return &SurveyService{
		repo:        repo,
		cache:       cache,
		scoreClient: scoreClient,
	}
}

func (s *SurveyService) GetSurvey(ctx context.Context, brandID, applicationID, code string) (*domain.SurveyModel, error) {
	if s.cache != nil {
		if cached, ok := s.cache.Get(ctx, brandID, applicationID, code); ok {
			return cached, nil
		}
	}

	if s.repo == nil {
		return nil, domain.ErrNotFound
	}

	row, err := s.repo.GetActive(ctx, brandID, applicationID, code)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, domain.ErrNotFound
	}

	model, err := domain.ParseSurveySchema([]byte(row.SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("invalid survey schema in db: %w", err)
	}

	model.BrandID = row.BrandID
	model.ApplicationID = row.ApplicationID
	model.Code = row.Code
	if model.Title == "" {
		model.Title = row.Title
	}

	if s.cache != nil {
		s.cache.Set(ctx, brandID, applicationID, code, model)
	}

	return model, nil
}

func (s *SurveyService) SaveSurvey(ctx context.Context, brandID, applicationID, code, title, status string, schemaJSON string) error {
	if s.repo == nil {
		return fmt.Errorf("survey repository is not configured")
	}

	model, err := domain.ParseSurveySchema([]byte(schemaJSON))
	if err != nil {
		return fmt.Errorf("invalid SurveyJS schema: %w", err)
	}

	if title == "" && model.Title != "" {
		title = model.Title
	}
	if status == "" {
		status = "active"
	}

	row := &entity.SurveyDefinition{
		BrandID:       brandID,
		ApplicationID: applicationID,
		Code:          code,
		Title:         title,
		Status:        status,
		SchemaJSON:    schemaJSON,
		UpdatedAt:     func() *time.Time { t := time.Now(); return &t }(),
	}

	if err := s.repo.Save(ctx, row); err != nil {
		return err
	}

	if s.cache != nil {
		s.cache.Invalidate(ctx, brandID, applicationID, code)
	}

	return nil
}

func (s *SurveyService) ListSurveys(ctx context.Context, brandID, applicationID string) ([]entity.SurveyDefinition, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.List(ctx, brandID, applicationID)
}

func (s *SurveyService) Evaluate(ctx context.Context, code string, req domain.SurveyEvaluateRequest) (*domain.EvaluationOutput, error) {
	verrs := &domain.ValidationErrors{}
	if req.BrandID == "" {
		verrs.Add("brand_id is required")
	}
	if req.ApplicationID == "" {
		verrs.Add("application_id is required")
	}
	if req.CustomerID == "" {
		verrs.Add("customer_id is required")
	}
	if verrs.HasErrors() {
		return nil, verrs
	}

	survey, err := s.GetSurvey(ctx, req.BrandID, req.ApplicationID, code)
	if err != nil {
		return nil, err
	}

	return s.EvaluateWithModel(ctx, survey, req)
}

func (s *SurveyService) EvaluateWithModel(ctx context.Context, survey *domain.SurveyModel, req domain.SurveyEvaluateRequest) (*domain.EvaluationOutput, error) {
	answerList, dimensionRefs, customerCondition, err := domain.ExtractEvaluationData(survey, req.Data)
	if err != nil {
		return nil, err
	}

	// Merge direct customer conditions submitted in the evaluate request
	for cond, val := range req.CustomerConditions {
		customerCondition[cond] = val
	}

	// Forward to Score Engine (:8084)
	var scoreResp *domain.ScoreModuleResponse
	if s.scoreClient != nil {
		scoreResp, err = s.scoreClient.Evaluate(ctx, domain.ScoreModuleRequest{
			Code:              survey.Code,
			BrandID:           req.BrandID,
			ApplicationID:     req.ApplicationID,
			CustomerID:        req.CustomerID,
			AnswerList:        answerList,
			CustomerCondition: customerCondition,
			Dimensions:        dimensionRefs,
			VisionSignals:     req.VisionSignals,
		})
		if err != nil {
			return nil, fmt.Errorf("score engine error: %w", err)
		}
	} else {
		// Default response when score client is nil (e.g. standalone test mode)
		scoreResp = &domain.ScoreModuleResponse{
			Success:           true,
			DimensionScores:   make(map[string]float64),
			CustomerCondition: customerCondition,
		}
	}

	return &domain.EvaluationOutput{
		Success:           true,
		Code:              survey.Code,
		BrandID:           req.BrandID,
		ApplicationID:     req.ApplicationID,
		CustomerID:        req.CustomerID,
		TotalScore:        scoreResp.TotalScore,
		DimensionScores:   scoreResp.DimensionScores,
		ScoreRange:        scoreResp.ScoreRange,
		SeverityLevel:     scoreResp.SeverityLevel,
		SkinConcern:       scoreResp.SkinConcern,
		SkinProfile:       scoreResp.SkinProfile,
		SkinGradingTiers:  scoreResp.SkinGradingTiers,
		CustomerCondition: customerCondition,
		AnswerList:        answerList,
		AppliedRules:      scoreResp.AppliedRules,
		EvaluatedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}
