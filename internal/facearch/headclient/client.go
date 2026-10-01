// Package headclient calls the face worker's POST /api/v1/face-head and
// returns its response verbatim: core-engine passes the GLB (or the worker's
// JSON error) through without decoding it.
package headclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const Path = "/api/v1/face-head"

// ErrUnavailable wraps transport failures (connection, timeout, body read).
var ErrUnavailable = errors.New("face worker unavailable")

type Part struct {
	Name     string
	Filename string
	Data     []byte
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) Head(ctx context.Context, parts []Part) (int, string, []byte, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for _, p := range parts {
		fw, err := w.CreateFormFile(p.Name, p.Filename)
		if err != nil {
			return 0, "", nil, fmt.Errorf("encode face-head request: %w", err)
		}
		if _, err := fw.Write(p.Data); err != nil {
			return 0, "", nil, fmt.Errorf("encode face-head request: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return 0, "", nil, fmt.Errorf("encode face-head request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+Path, body)
	if err != nil {
		return 0, "", nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), b, nil
}
