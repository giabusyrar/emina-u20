package controllers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/adapters/controllers"
)

// The colour routes use the service's error envelope and keep "code", which
// seagull-web's colour client reads.
func TestDisabledKeepsEnvelopeAndCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := controllers.Disabled(errors.New("wcpa config: no such file"))(c); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable || body["success"] != false {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if body["error"] != "wcpa config: no such file" || body["error_code"] != "colour_engine_disabled" || body["code"] != "colour_engine_disabled" {
		t.Fatalf("got %v", body)
	}
}

// /health keeps the service's health shape (status, service, checks).
func TestDisabledHealthKeepsHealthShape(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := controllers.DisabledHealth("core-engine", errors.New("wcpa config: no such file"))(c); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	checks, _ := body["checks"].(map[string]any)
	if rec.Code != http.StatusServiceUnavailable || body["status"] != "degraded" || body["service"] != "core-engine" ||
		checks["wcpa-config"] != "down" || body["error"] != "wcpa config: no such file" || body["code"] != "colour_engine_disabled" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}
