package goplatform

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestNewEchoAppliesCORSConfig(t *testing.T) {
	e := NewEcho(ServerOptions{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSAllowOrigins: []string{"*"},
		CORSAllowHeaders: []string{"Origin", "Content-Type"},
	})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Origin,Content-Type" {
		t.Fatalf("Allow-Headers = %q", got)
	}
}

func TestNewEchoSetsRequestID(t *testing.T) {
	e := NewEcho(ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CORSAllowOrigins: []string{"*"}})
	e.GET("/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Header().Get(echo.HeaderXRequestID) == "" {
		t.Fatal("want X-Request-Id on response")
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, echo.New(), "127.0.0.1:0", time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := Run(context.Background(), echo.New(), l.Addr().String(), time.Second); err == nil {
		t.Fatal("want error when the port is taken")
	}
}
