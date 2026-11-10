package goplatform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// corsAllowMethods are the methods the routes in this repository register.
// That is a property of the code, not of a deployment, so it is not
// configurable.
var corsAllowMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}

type ServerOptions struct {
	Logger           *slog.Logger
	CORSAllowOrigins []string
	CORSAllowHeaders []string
}

// NewEcho returns an Echo instance with the standard middleware, in this
// order: Recover, RequestID, request logger (slog), CORS.
func NewEcho(o ServerOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			o.Logger.Error("panic recovered", "err", err, "stack", string(stack), "request_id", c.Response().Header().Get(echo.HeaderXRequestID))
			return err
		},
	}))
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogError:     true,
		HandleError:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []any{"method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "request_id", v.RequestID}
			switch {
			case v.Status >= http.StatusInternalServerError:
				if v.Error != nil {
					attrs = append(attrs, "err", v.Error)
				}
				o.Logger.Error("request", attrs...)
			case v.Status >= http.StatusBadRequest:
				if v.Error != nil {
					attrs = append(attrs, "err", v.Error)
				}
				o.Logger.Warn("request", attrs...)
			default:
				o.Logger.Info("request", attrs...)
			}
			return nil
		},
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: o.CORSAllowOrigins,
		AllowMethods: corsAllowMethods,
		AllowHeaders: o.CORSAllowHeaders,
	}))
	return e
}

// Run serves until ctx is cancelled, then shuts down gracefully within
// shutdownTimeout. It returns early with the error if the server cannot
// start.
func Run(ctx context.Context, e *echo.Echo, addr string, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- e.Start(addr) }()
	slog.Info("starting server", "addr", addr)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down", "timeout", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
