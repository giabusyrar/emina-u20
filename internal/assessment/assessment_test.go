package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/form/domain"
)

type memStore struct {
	saved   []Record
	lastF   Filter
	saveErr error
}

func (m *memStore) Save(_ context.Context, r *Record) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	r.ID = "id-1"
	m.saved = append(m.saved, *r)
	return nil
}

func (m *memStore) List(_ context.Context, f Filter) ([]Record, error) {
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

func serve(h *Handler, target string) *httptest.ResponseRecorder {
	e := echo.New()
	h.Mount(e.Group("/api/assessments"))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandlerRequiresBrandScope(t *testing.T) {
	h := NewHandler(&memStore{}, 200)
	for _, target := range []string{"/api/assessments/history", "/api/assessments/customers/c-1?brand_id=wardah"} {
		if rec := serve(h, target); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", target, rec.Code)
		}
	}
}

func TestHandlerFiltersAndCapsLimit(t *testing.T) {
	s := &memStore{}
	h := NewHandler(s, 200)

	rec := serve(h, "/api/assessments/customers/c-1?brand_id=wardah&application_id=skinverse&limit=500")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if s.lastF != (Filter{BrandID: "wardah", ApplicationID: "skinverse", CustomerID: "c-1", Limit: 200}) {
		t.Fatalf("filter = %+v", s.lastF)
	}

	serve(h, "/api/assessments/history?brand_id=wardah&application_id=skinverse&limit=10")
	if s.lastF.CustomerID != "" || s.lastF.Limit != 10 {
		t.Fatalf("history filter = %+v", s.lastF)
	}

	if rec := serve(h, "/api/assessments/history?brand_id=w&application_id=a&limit=0"); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit=0: status %d, want 400", rec.Code)
	}
}
