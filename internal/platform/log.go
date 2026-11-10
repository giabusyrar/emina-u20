package goplatform

import (
	"fmt"
	"io"
	"log/slog"
)

// ParseLevel accepts debug, info, warn or error, in any case.
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return l, nil
}

// NewLogger returns a JSON logger at level whose every line carries service.
func NewLogger(w io.Writer, service, level string) (*slog.Logger, error) {
	l, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l})).With("service", service), nil
}
