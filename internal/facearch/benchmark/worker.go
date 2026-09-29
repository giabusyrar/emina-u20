package benchmark

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// Worker is the ai-worker as the harness needs it: status and body bytes,
// so the exact JSON can be cached. measureclient.Client.Measure returns
// decoded responses only, and discards 200 bodies after decoding.
type Worker interface {
	// Catalogue returns the body of GET /api/v1/face-measure/catalogue.
	Catalogue(ctx context.Context) ([]byte, error)
	// Measure posts one image. It returns the status and body for 200, 400,
	// 422 and 503 (the answers the endpoint also passes through), and an
	// error for anything else.
	Measure(ctx context.Context, image []byte, filename string) (int, []byte, error)
}

// HTTPWorker calls the ai-worker through measureclient.Client's raw methods,
// so it sends exactly what the endpoint sends and cached bodies are what the
// endpoint would receive.
type HTTPWorker struct {
	baseURL string
	client  *measureclient.Client
}

// NewHTTPWorker returns a Worker for the ai-worker at baseURL.
func NewHTTPWorker(baseURL string, timeout time.Duration) *HTTPWorker {
	return &HTTPWorker{baseURL: strings.TrimRight(baseURL, "/"), client: measureclient.New(baseURL, timeout)}
}

// BaseURL is where images are sent.
func (w *HTTPWorker) BaseURL() string { return w.baseURL }

// Catalogue implements Worker.
func (w *HTTPWorker) Catalogue(ctx context.Context) ([]byte, error) {
	return w.client.CatalogueRaw(ctx)
}

// Measure implements Worker.
func (w *HTTPWorker) Measure(ctx context.Context, image []byte, filename string) (int, []byte, error) {
	status, raw, err := w.client.MeasureRaw(ctx, image, filename)
	if err != nil {
		return 0, nil, err
	}
	switch status {
	case http.StatusOK, http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusServiceUnavailable:
		return status, raw, nil
	}
	return 0, nil, fmt.Errorf("%w: face-measure returned status %d", measureclient.ErrUnavailable, status)
}
