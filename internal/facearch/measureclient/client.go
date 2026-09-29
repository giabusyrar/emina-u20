package measureclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable wraps every failure to get an answer from the ai-worker
// other than the statuses it passes through (see UpstreamError):
// connection errors, timeouts, undecodable bodies and unexpected statuses.
var ErrUnavailable = errors.New("face-measure unavailable")

// passThrough are the ai-worker statuses whose bodies core-engine returns
// to its caller unchanged (spec: "Passes 422 and 503 through with their
// bodies"; 400 is the worker's unreadable-image answer and is the caller's
// fault in the same way).
var passThrough = map[int]bool{
	http.StatusBadRequest:          true,
	http.StatusUnprocessableEntity: true,
	http.StatusServiceUnavailable:  true,
}

// UpstreamError is a 400, 422 or 503 from the ai-worker, with its body verbatim.
type UpstreamError struct {
	Status int
	Body   []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("face-measure returned %d: %s", e.Status, strings.TrimSpace(string(e.Body)))
}

// Client calls the ai-worker's face-measure endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New returns a Client for the ai-worker at baseURL (VISION_AI_WORKER_URL).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
			// Never follow a redirect: images only go to the configured URL.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// measureRequest builds the one request every measure call sends: image as
// multipart field "image" with landmarks=true.
func (c *Client) measureRequest(ctx context.Context, image []byte, filename string) (*http.Request, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("image", filename)
	if err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if _, err := part.Write(image); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if err := w.WriteField("landmarks", "true"); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+MeasurePath, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

// Measure posts image as multipart field "image" with landmarks=true.
func (c *Client) Measure(ctx context.Context, image []byte, filename string) (*Response, error) {
	req, err := c.measureRequest(ctx, image, filename)
	if err != nil {
		return nil, err
	}
	var out Response
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MeasureRaw sends the same request as Measure and returns the HTTP status
// and body exactly as received, for any status, without decoding. Only a
// transport failure (connection, timeout, body read) is an error, wrapped in
// ErrUnavailable. It exists for callers that must cache the worker's exact
// bytes (the face benchmark harness).
func (c *Client) MeasureRaw(ctx context.Context, image []byte, filename string) (int, []byte, error) {
	req, err := c.measureRequest(ctx, image, filename)
	if err != nil {
		return 0, nil, err
	}
	return c.doRaw(req)
}

// Catalogue fetches GET /api/v1/face-measure/catalogue.
func (c *Client) Catalogue(ctx context.Context) (*Catalogue, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+CataloguePath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var out Catalogue
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CatalogueRaw fetches the catalogue endpoint and returns its body bytes
// undecoded; any non-200 status is ErrUnavailable.
func (c *Client) CatalogueRaw(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+CataloguePath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	status, raw, err := c.doRaw(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%w: %s returned status %d", ErrUnavailable, req.URL.Path, status)
	}
	return raw, nil
}

// doRaw performs req and returns the status and body for any status.
func (c *Client) doRaw(req *http.Request) (int, []byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: read %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	return resp.StatusCode, raw, nil
}

func (c *Client) do(req *http.Request, out any) error {
	status, raw, err := c.doRaw(req)
	if err != nil {
		return err
	}
	if passThrough[status] {
		return &UpstreamError{Status: status, Body: raw}
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: %s returned status %d", ErrUnavailable, req.URL.Path, status)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	return nil
}
