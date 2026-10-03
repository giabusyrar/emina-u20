package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/colour/domain"
)

func TestRetakeRequiredIs422WithReasons(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), rec)
	if err := respondErr(c, &domain.RetakeRequiredError{Reasons: []string{"terlalu_gelap"}}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	reasons, _ := body["reasons"].([]any)
	if rec.Code != http.StatusUnprocessableEntity || body["code"] != "retake_required" ||
		body["error_code"] != "retake_required" || body["message"] == "" || body["message"] == nil ||
		len(reasons) != 1 || reasons[0] != "terlalu_gelap" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}
