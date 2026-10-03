package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRespondErrValidationKeepsErrorsAndGainsError(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	verrs := &domain.ValidationErrors{}
	verrs.Add("field a is required")

	if err := respondErr(c, verrs); err != nil {
		t.Fatal(err)
	}
	body := decode(t, rec)
	if rec.Code != http.StatusUnprocessableEntity || body["success"] != false || body["error"] != "validation failed" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if errs, ok := body["errors"].([]any); !ok || len(errs) != 1 || errs[0] != "field a is required" {
		t.Fatalf("errors = %v", body["errors"])
	}
}

func TestRespondErrNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := respondErr(c, fmt.Errorf("wrap: %w", domain.ErrNotFound)); err != nil {
		t.Fatal(err)
	}
	body := decode(t, rec)
	if rec.Code != http.StatusNotFound || body["success"] != false || body["error"] != "not found" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}
