package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

type memStore struct {
	saved   []entity.Record
	lastF   Filter
	saveErr error
}

func (m *memStore) Save(_ context.Context, r *entity.Record) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	r.ID = "id-1"
	m.saved = append(m.saved, *r)
	return nil
}

func (m *memStore) List(_ context.Context, f Filter) ([]entity.Record, error) {
	m.lastF = f
	return m.saved, nil
}

func fullOutput() *domain.EvaluationOutput {
	return &domain.EvaluationOutput{
		Code: "skinverse_q1_q6", BrandID: "wardah", ApplicationID: "skinverse", CustomerID: "c-1",
		TotalScore:       72.5,
		DimensionScores:  map[string]float64{"OILINESS": 80},
		SkinProfile:      domain.SkinProfileType{Code: "OSPT", Name: "Oily"},
		SkinGradingTiers: []domain.SkinGradingTier{{DimensionKey: "OILINESS", Score: 80, Severity: "mild"}},
		AnswerList:       []domain.AnswerEntry{{Question: "Q1", Answer: "a", Score: 3, MaxScore: 4}},
	}
}

func TestFromEvaluationMapsOutput(t *testing.T) {
	rec, err := FromEvaluation(fullOutput(), map[string]float64{"PORE": 40}, 1500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if rec.CustomerID != "c-1" || rec.FormID != "skinverse_q1_q6" || rec.TotalScore != 72.5 || rec.SkinProfileCode != "OSPT" || rec.ExecutionTimeMs != 1500 {
		t.Fatalf("scalar fields wrong: %+v", rec)
	}
	var dims map[string]float64
	if err := json.Unmarshal([]byte(rec.DimensionScores), &dims); err != nil || dims["OILINESS"] != 80 {
		t.Fatalf("dimension_scores = %q", rec.DimensionScores)
	}
	for name, p := range map[string]*string{"skin_profile": rec.SkinProfile, "tiers": rec.SkinGradingTiers, "answers": rec.RawAnswers, "vision": rec.VisionMetrics} {
		if p == nil || !json.Valid([]byte(*p)) {
			t.Fatalf("%s not stored as JSON: %v", name, p)
		}
	}
	if rec.RecommendedProducts != nil {
		t.Fatal("recommended_products must stay NULL: a survey evaluation recommends no products")
	}
}

func TestFromEvaluationLeavesAbsentPartsNull(t *testing.T) {
	out := &domain.EvaluationOutput{Code: "s", BrandID: "b", ApplicationID: "a", CustomerID: "c"}
	rec, err := FromEvaluation(out, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rec.SkinProfile != nil || rec.SkinGradingTiers != nil || rec.RawAnswers != nil || rec.VisionMetrics != nil {
		t.Fatalf("absent parts must be NULL: %+v", rec)
	}
	if rec.DimensionScores != "{}" {
		t.Fatalf("dimension_scores = %q, want {}", rec.DimensionScores)
	}
}

func TestFromEvaluationRequiresCustomer(t *testing.T) {
	if _, err := FromEvaluation(&domain.EvaluationOutput{BrandID: "b"}, nil, 0); err == nil {
		t.Fatal("want error without customer_id")
	}
}

func TestRecorderReturnsStoredIDAndStoreError(t *testing.T) {
	s := &memStore{}
	id, err := NewRecorder(s).Record(context.Background(), fullOutput(), nil, 0)
	if err != nil || id != "id-1" || len(s.saved) != 1 {
		t.Fatalf("got %q, %v, %d saved", id, err, len(s.saved))
	}
	boom := errors.New("db down")
	if _, err := NewRecorder(&memStore{saveErr: boom}).Record(context.Background(), fullOutput(), nil, 0); !errors.Is(err, boom) {
		t.Fatalf("want store error, got %v", err)
	}
}
