package scoreengine_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/form/adapters/scoreengine"
	formdomain "github.com/experience-gateway/apps/core-engine/internal/form/domain"
	scoredomain "github.com/experience-gateway/apps/core-engine/internal/score/domain"
	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
)

// fakeScorer stands in for score's ScoreService: it records what the adapter
// passes in and returns canned results.
type fakeScorer struct {
	gotReq      scoredomain.ScoreModuleRequest
	resp        *scoredomain.ScoreModuleResponse
	rulesets    []entity.ScoreRuleset
	active      *entity.ScoreRuleset
	err         error
	sawDeadline bool
}

func (f *fakeScorer) Evaluate(ctx context.Context, req scoredomain.ScoreModuleRequest) (*scoredomain.ScoreModuleResponse, error) {
	f.gotReq = req
	_, f.sawDeadline = ctx.Deadline()
	return f.resp, f.err
}

func (f *fakeScorer) ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error) {
	return f.rulesets, f.err
}

func (f *fakeScorer) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error) {
	return f.active, f.err
}

func TestEvaluateTranslatesBothWaysAndBoundsTheCall(t *testing.T) {
	f := &fakeScorer{resp: &scoredomain.ScoreModuleResponse{
		Success:         true,
		TotalScore:      42,
		DimensionScores: map[string]float64{"sebum": 80},
		SkinProfile:     scoredomain.SkinProfileType{Code: "OSPT", AxisValues: map[string]string{"OILINESS": "O"}},
	}}
	a := scoreengine.New(f, time.Second)

	out, err := a.Evaluate(context.Background(), formdomain.ScoreModuleRequest{
		Code:          "baumann_16_types",
		BrandID:       "b",
		ApplicationID: "app",
		Dimensions:    []formdomain.DimensionRef{{Key: "sebum", MaxScore: 100, Answers: []int{40, 40}}},
		VisionSignals: map[string]float64{"acne": 0.3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.gotReq.Code != "baumann_16_types" || f.gotReq.BrandID != "b" || f.gotReq.ApplicationID != "app" {
		t.Fatalf("request identity not carried: %+v", f.gotReq)
	}
	if len(f.gotReq.Dimensions) != 1 || f.gotReq.Dimensions[0].Key != "sebum" || len(f.gotReq.Dimensions[0].Answers) != 2 {
		t.Fatalf("dimensions not carried: %+v", f.gotReq.Dimensions)
	}
	if f.gotReq.VisionSignals["acne"] != 0.3 {
		t.Fatalf("vision signals not carried: %+v", f.gotReq.VisionSignals)
	}
	if !f.sawDeadline {
		t.Fatal("the score call must run under the configured timeout")
	}
	if !out.Success || out.TotalScore != 42 || out.DimensionScores["sebum"] != 80 ||
		out.SkinProfile.Code != "OSPT" || out.SkinProfile.AxisValues["OILINESS"] != "O" {
		t.Fatalf("response not carried: %+v", out)
	}
}

func TestScoreFailuresAreUpstreamErrors(t *testing.T) {
	a := scoreengine.New(&fakeScorer{err: errors.New("jdm exploded")}, time.Second)

	if _, err := a.Evaluate(context.Background(), formdomain.ScoreModuleRequest{}); !errors.Is(err, formdomain.ErrUpstream) {
		t.Fatalf("Evaluate: want ErrUpstream, got %v", err)
	}
	if _, err := a.ListRulesets(context.Background(), "b", "app"); !errors.Is(err, formdomain.ErrUpstream) {
		t.Fatalf("ListRulesets: want ErrUpstream, got %v", err)
	}
	if _, err := a.GetActiveRuleset(context.Background(), "b", "app", "c"); !errors.Is(err, formdomain.ErrUpstream) {
		t.Fatalf("GetActiveRuleset: want ErrUpstream, got %v", err)
	}
}

func TestMissingActiveRulesetIsAnUpstreamError(t *testing.T) {
	// The HTTP path answered 404 here, which the old client reported as ErrUpstream.
	a := scoreengine.New(&fakeScorer{}, time.Second)
	if _, err := a.GetActiveRuleset(context.Background(), "b", "app", "c"); !errors.Is(err, formdomain.ErrUpstream) {
		t.Fatalf("want ErrUpstream, got %v", err)
	}
}

func TestRulesetsKeepIDCodeStatusAndSchema(t *testing.T) {
	rs := entity.ScoreRuleset{ID: "r1", Code: "skin", Status: "ACTIVE", Schema: `{"nodes":[]}`}
	a := scoreengine.New(&fakeScorer{rulesets: []entity.ScoreRuleset{rs}, active: &rs}, time.Second)

	list, err := a.ListRulesets(context.Background(), "b", "app")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
	active, err := a.GetActiveRuleset(context.Background(), "b", "app", "skin")
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []scoreengine.Ruleset{list[0], *active} {
		if got.ID != "r1" || got.Code != "skin" || got.Status != "ACTIVE" || got.Schema != `{"nodes":[]}` {
			t.Fatalf("ruleset not carried: %+v", got)
		}
	}
}
