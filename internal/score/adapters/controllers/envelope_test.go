package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/adapters/controllers"
)

// The error body must keep "success":false and "error":<string>, which
// clients read today.
func TestEvaluateBadJSONKeepsEnvelope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	h := controllers.NewScoreHandler(nil) // Bind fails before the service is touched.
	if err := h.Evaluate(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body["success"] != false {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if msg, ok := body["error"].(string); !ok || !strings.HasPrefix(msg, "Invalid JSON payload") {
		t.Fatalf("error = %v, want the existing message prefix", body["error"])
	}
	if _, has := body["errors"]; has {
		t.Fatalf("non-validation error must not carry errors: %v", body)
	}
}
