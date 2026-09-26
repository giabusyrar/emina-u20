package goplatform

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestOpenPostgresRequiresURL(t *testing.T) {
	if _, err := OpenPostgres(context.Background(), "", DBOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PingTimeout: time.Second}); err == nil {
		t.Fatal("want error for empty URL")
	}
}

func TestOpenPostgresFailsWhenUnreachable(t *testing.T) {
	// Port 1 on loopback refuses connections immediately.
	_, err := OpenPostgres(context.Background(),
		"postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		DBOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), PingTimeout: 2 * time.Second})
	if err == nil {
		t.Fatal("want error when Postgres is unreachable")
	}
}

func TestOpenRedis(t *testing.T) {
	if c, err := OpenRedis(context.Background(), "", time.Second); c != nil || err != nil {
		t.Fatalf("empty URL: got %v, %v; want nil, nil", c, err)
	}
	if c, err := OpenRedis(context.Background(), "redis://127.0.0.1:1", time.Second); c != nil || err == nil {
		t.Fatalf("unreachable: got %v, %v; want nil, error", c, err)
	}
}
