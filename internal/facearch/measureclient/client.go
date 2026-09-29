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
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Measure posts image as multipart field "image" with landmarks=true.
func (c *Client) Measure(ctx context.Context, image []byte, filename string) (*Response, error) {
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
	var out Response
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	if passThrough[resp.StatusCode] {
		return &UpstreamError{Status: resp.StatusCode, Body: raw}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s returned status %d", ErrUnavailable, req.URL.Path, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	return nil
}
