package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/assessment/service"
)

type memStore struct {
	saved   []entity.Record
	lastF   service.Filter
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

func (m *memStore) List(_ context.Context, f service.Filter) ([]entity.Record, error) {
	m.lastF = f
	return m.saved, nil
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
	if s.lastF != (service.Filter{BrandID: "wardah", ApplicationID: "skinverse", CustomerID: "c-1", Limit: 200}) {
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
