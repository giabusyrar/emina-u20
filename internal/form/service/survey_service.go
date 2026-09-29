package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/form/adapters/cache"
	"github.com/experience-gateway/apps/core-engine/internal/form/adapters/scoreengine"
	"github.com/experience-gateway/apps/core-engine/internal/form/adapters/visionclient"
	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
	"github.com/experience-gateway/apps/core-engine/internal/form/entity"
	"github.com/experience-gateway/apps/core-engine/internal/form/repository"
)

// ScoreEngine is what the Form Engine needs from the Score Module: resolve a
// survey's ruleset and score an assembled payload. In the running service it
// is scoreengine.Adapter, an in-process call into score's ScoreService.
type ScoreEngine interface {
	ListRulesets(ctx context.Context, brandID, appID string) ([]scoreengine.Ruleset, error)
	GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*scoreengine.Ruleset, error)
	Evaluate(ctx context.Context, req domain.ScoreModuleRequest) (*domain.ScoreModuleResponse, error)
}

type SurveyService struct {
	repo         *repository.SurveyRepo
	cache        *cache.SurveyCache
	scoreClient  ScoreEngine
	visionClient *visionclient.Client
}

func NewSurveyService(repo *repository.SurveyRepo, cache *cache.SurveyCache, scoreClient ScoreEngine, visionClient *visionclient.Client) *SurveyService {
	return &SurveyService{
		repo:         repo,
		cache:        cache,
		scoreClient:  scoreClient,
		visionClient: visionClient,
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
	status = entity.NormalizeSurveyStatus(status)

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
	// isRequired is only enforced when answers were actually submitted — a
	// vision-only call (no data at all) never fails this check.
	if len(req.Data) > 0 {
		if verrs := domain.ValidateRequiredAnswers(survey, req.Data); verrs.HasErrors() {
			return nil, verrs
		}
	}

	answerList, dimensionRefs, customerCondition, labels, err := domain.ExtractEvaluationData(survey, req.Data)
	if err != nil {
		return nil, err
	}
	dimensionRefs = append(dimensionRefs, req.ComputedDimensions...)

	// Merge direct customer conditions submitted in the evaluate request
	for cond, val := range req.CustomerConditions {
		customerCondition[cond] = val
	}

	// Forward to Score Engine (:8084). A survey's own code and the ruleset
	// that scores it are not guaranteed to match (e.g. survey
	// "skinverse_q1_q6" is scored by ruleset "skinverse_skin_character") —
	// resolve via the ruleset's own form_survey_code link when one exists,
	// falling back to survey.Code so setups that share one code still work.
	var scoreResp *domain.ScoreModuleResponse
	if s.scoreClient != nil {
		rulesetCode := s.resolveRulesetCode(ctx, req.BrandID, req.ApplicationID, survey.Code)

		scoreResp, err = s.scoreClient.Evaluate(ctx, domain.ScoreModuleRequest{
			Code:              rulesetCode,
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
		Success:            true,
		Code:               survey.Code,
		BrandID:            req.BrandID,
		ApplicationID:      req.ApplicationID,
		CustomerID:         req.CustomerID,
		TotalScore:         scoreResp.TotalScore,
		DimensionScores:    scoreResp.DimensionScores,
		ScoreRange:         scoreResp.ScoreRange,
		SeverityLevel:      scoreResp.SeverityLevel,
		SkinConcern:        scoreResp.SkinConcern,
		SkinProfile:        scoreResp.SkinProfile,
		SkinGradingTiers:   scoreResp.SkinGradingTiers,
		CustomerCondition:  customerCondition,
		AnswerList:         answerList,
		AppliedRules:       scoreResp.AppliedRules,
		SubClassification:  scoreResp.SubClassification,
		RawDimensionScores: scoreResp.RawDimensionScores,
		Labels:             labels,
		EvaluatedAt:        time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// resolveRulesetCode finds the ruleset actually scoring this survey via its
// own form_survey_code link, falling back to the survey's own code. Errors
// listing rulesets are swallowed deliberately — this is a best-effort lookup
// on top of Evaluate, which still surfaces "ruleset not found" clearly if
// the fallback code doesn't resolve to anything either.
func (s *SurveyService) resolveRulesetCode(ctx context.Context, brandID, appID, surveyCode string) string {
	rulesets, err := s.scoreClient.ListRulesets(ctx, brandID, appID)
	if err != nil {
		return surveyCode
	}
	candidates := make([]domain.RulesetSchemaWithCode, len(rulesets))
	for i, rs := range rulesets {
		candidates[i] = domain.RulesetSchemaWithCode{Code: rs.Code, Status: rs.Status, Schema: rs.Schema}
	}
	return domain.ResolveRulesetCode(candidates, surveyCode)
}

// EvaluateWithPhotos is the one call a brand's Submit button makes: it
// collects the applicant's photo alongside their Q1-Q6 answers and personal
// data, calls Paradev itself (server-side — the brand never sees any vendor
// key), converts the vendor's health-space result into the concern-space
// vision_signals /evaluate expects, computes the Aging axis's form-side
// value from date_of_birth (no Q1-Q6 question asks age), and forwards
// everything to the same EvaluateWithModel the plain evaluate endpoint uses.
func (s *SurveyService) EvaluateWithPhotos(ctx context.Context, code string, req domain.EvaluateWithPhotosRequest, image io.Reader, imageFilename string) (*domain.EvaluateWithPhotosResponse, error) {
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
	if req.DateOfBirth == "" {
		verrs.Add("date_of_birth is required")
	}
	if image == nil {
		verrs.Add("a photo is required")
	}
	if verrs.HasErrors() {
		return nil, verrs
	}

	survey, err := s.GetSurvey(ctx, req.BrandID, req.ApplicationID, code)
	if err != nil {
		return nil, err
	}

	rulesetCode := s.resolveRulesetCode(ctx, req.BrandID, req.ApplicationID, survey.Code)
	ruleset, err := s.scoreClient.GetActiveRuleset(ctx, req.BrandID, req.ApplicationID, rulesetCode)
	if err != nil || ruleset == nil {
		return nil, fmt.Errorf("active scoring ruleset not found for %q: %w", rulesetCode, err)
	}

	if s.visionClient == nil {
		return nil, fmt.Errorf("%w: vision client not configured", domain.ErrVisionUnavailable)
	}
	visionResult, err := s.visionClient.Submit(ctx, visionclient.Submission{
		FullName:              req.FullName,
		PhoneNumber:           req.PhoneNumber,
		Email:                 req.Email,
		DateOfBirth:           req.DateOfBirth,
		ConsentDataProcessing: req.ConsentDataProcessing,
		ConsentMarketing:      req.ConsentMarketing,
		Image:                 image,
		ImageFilename:         imageFilename,
	})
	if err != nil {
		return nil, err
	}

	visionSignals := domain.RulesetVisionSignals(ruleset.Schema, visionResult.Flattened)

	// Aging (or any other axis fed by date_of_birth rather than a Q1-Q6
	// question) needs its own form-side value or fusion silently falls back
	// to 100% vision instead of the ruleset's configured blend.
	overThirty, ageErr := domain.AgeOverThirty(req.DateOfBirth, time.Now())
	if ageErr != nil {
		return nil, fmt.Errorf("invalid date_of_birth: %w", ageErr)
	}
	ageConcern := 0
	if overThirty {
		ageConcern = 100
	}
	var computedDimensions []domain.DimensionRef
	for _, dimKey := range domain.RulesetFormDimensionKeys(ruleset.Schema, "age_over_30") {
		computedDimensions = append(computedDimensions, domain.DimensionRef{
			Key:               dimKey,
			MinScore:          0,
			MaxScore:          100,
			CalculationMethod: "sum",
			Answers:           []int{ageConcern},
		})
	}

	out, err := s.EvaluateWithModel(ctx, survey, domain.SurveyEvaluateRequest{
		BrandID:            req.BrandID,
		ApplicationID:      req.ApplicationID,
		CustomerID:         req.CustomerID,
		Data:               req.Data,
		CustomerConditions: req.CustomerConditions,
		VisionSignals:      visionSignals,
		ComputedDimensions: computedDimensions,
	})
	if err != nil {
		return nil, err
	}

	return &domain.EvaluateWithPhotosResponse{
		EvaluationOutput:     out,
		ParadevTransactionID: visionResult.TransactionID,
		VisionSignalsUsed:    visionSignals,
	}, nil
}
