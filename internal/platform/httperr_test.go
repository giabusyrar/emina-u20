package goplatform

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func body(t *testing.T, write func(echo.Context) error) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := write(c); err != nil {
		t.Fatal(err)
	}
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name  string
		write func(echo.Context) error
		code  int
		want  string
	}{
		{"plain", func(c echo.Context) error { return JSONError(c, 404, "survey not found") },
			404, `{"success":false,"error":"survey not found"}`},
		{"coded", func(c echo.Context) error { return JSONErrorCode(c, 503, "VISION_UNAVAILABLE", "down") },
			503, `{"success":false,"error":"down","error_code":"VISION_UNAVAILABLE"}`},
		{"validation", func(c echo.Context) error { return JSONValidationError(c, 422, []string{"a", "b"}) },
			422, `{"success":false,"error":"validation failed","errors":["a","b"]}`},
	}
	for _, tc := range cases {
		code, got := body(t, tc.write)
		if code != tc.code || got != tc.want {
			t.Errorf("%s: got %d %s, want %d %s", tc.name, code, got, tc.code, tc.want)
		}
	}
}
