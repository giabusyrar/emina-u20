package service

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/brand"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/entity"
)

// An empty request used to score 0 and come back "Sangat Parah" /
// "Perlu Perhatian Khusus": a diagnosis made from nothing.
func TestEvaluateRefusesEmptyRequest(t *testing.T) {
	_, err := newTestScoreService().Evaluate(context.Background(), domain.ScoreModuleRequest{})
	var verrs *domain.ValidationErrors
	if !errors.As(err, &verrs) {
		t.Fatalf("got %v, want a validation error", err)
	}
}

type recordingRepo struct {
	mockRulesetRepo
	brand string
}

func (r *recordingRepo) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error) {
	r.brand = brandID
	return &r.ruleset, nil
}

type oneBrand struct{}

func (oneBrand) Resolve(_ context.Context, ref string) (brand.Brand, error) {
	if ref == "brd-1" || ref == "WARDAH" {
		return brand.Brand{ID: "brd-1", Code: "wardah"}, nil
	}
	return brand.Brand{}, brand.ErrUnknown
}

// A ref_brands id finds the ruleset stored under the brand's code.
func TestEvaluateLooksRulesetsUpByBrandCode(t *testing.T) {
	repo := &recordingRepo{mockRulesetRepo: mockRulesetRepo{ruleset: newTestScoreService().repo.(*mockRulesetRepo).ruleset}}
	svc := NewScoreService(repo).WithBrands(oneBrand{})
	_, err := svc.Evaluate(context.Background(), domain.ScoreModuleRequest{
		BrandID: "brd-1", AnswerList: []domain.AnswerEntry{{Score: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.brand != "wardah" {
		t.Errorf("ruleset looked up under %q, want the brand code wardah", repo.brand)
	}
	if _, err := svc.Evaluate(context.Background(), domain.ScoreModuleRequest{
		BrandID: "nope", AnswerList: []domain.AnswerEntry{{Score: 1}},
	}); !errors.Is(err, brand.ErrUnknown) {
		t.Errorf("unknown brand: got %v, want brand.ErrUnknown", err)
	}
}
