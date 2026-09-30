// Package workerclient calls the WCPA endpoints of the colour
// worker (/pca/analyze, /vto/render, /colour/munsell). Internal network only;
// no gateway key.
package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/colour/domain"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New builds a client. Measurement and rendering run MediaPipe + numpy on the
// worker's CPU, so allow seconds, not the short default used for DB calls.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: timeout}}
}

// AnalyzeResult mirrors POST /pca/analyze (contract §2).
type AnalyzeResult struct {
	Measurement struct {
		SkinLab     *domain.Lab        `json:"skinLab"`
		LipLab      *domain.Lab        `json:"lipLab"`
		IrisLab     *domain.Lab        `json:"irisLab"`
		HairLab     *domain.Lab        `json:"hairLab"`
		LPerSite    map[string]float64 `json:"lPerSite"`
		PatchesUsed []string           `json:"patchesUsed"`
		Illuminant  struct {
			Method   string   `json:"method"`
			Residual *float64 `json:"residual"`
		} `json:"illuminant"`
	} `json:"measurement"`
	Munsell struct {
		Skin domain.Munsell `json:"skin"`
		Lip  domain.Munsell `json:"lip"`
	} `json:"munsell"`
	Quality struct {
		QCFailed []string `json:"qcFailed"`
		Degraded []string `json:"degraded"`
	} `json:"quality"`
	PipelineVersion string `json:"pipelineVersion"`
}

// Layer is one product layer for /vto/render.
type Layer struct {
	Category string  `json:"category"`
	HexColor string  `json:"hexColor"`
	Alpha    float64 `json:"alpha"`
	Specular float64 `json:"specular"`
	Mode     string  `json:"mode,omitempty"`
}

// Analyze sends the photo plus the two self-reported headcover answers.
func (c *Client) Analyze(ctx context.Context, image []byte, filename string, hijab, hairVisible bool) (*AnalyzeResult, error) {
	body, ctype, err := buildMultipart(image, filename, map[string]string{
		"hijab":       strconv.FormatBool(hijab),
		"hairVisible": strconv.FormatBool(hairVisible),
	})
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, "/pca/analyze", body, ctype)
	if err != nil {
		return nil, err
	}
	var out AnalyzeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: decoding analyze response: %v", domain.ErrWorkerUnavailable, err)
	}
	if out.Measurement.SkinLab == nil {
		return nil, fmt.Errorf("%w: analyze response has no skinLab", domain.ErrWorkerUnavailable)
	}
	return &out, nil
}

// Render returns the PNG produced by the worker.
func (c *Client) Render(ctx context.Context, image []byte, filename string, layers []Layer) ([]byte, error) {
	req, err := json.Marshal(map[string]interface{}{"layers": layers})
	if err != nil {
		return nil, fmt.Errorf("building render request: %w", err)
	}
	body, ctype, err := buildMultipart(image, filename, map[string]string{"request": string(req)})
	if err != nil {
		return nil, err
	}
	return c.post(ctx, "/vto/render", body, ctype)
}

// Colour is one entry of POST /colour/munsell.
type Colour struct {
	Lab     domain.Lab
	Munsell domain.Munsell
}

// MunsellFromHex converts catalog hex colours with the worker's own hex→Lab
// and §1 Lab→Munsell (catalog seeding), so shades and people share one
// implementation.
func (c *Client) MunsellFromHex(ctx context.Context, hexes []string) ([]Colour, error) {
	req, err := json.Marshal(map[string]interface{}{"hexes": hexes})
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, "/colour/munsell", bytes.NewReader(req), "application/json")
	if err != nil {
		return nil, err
	}
	var out struct {
		Lab     []domain.Lab     `json:"lab"`
		Munsell []domain.Munsell `json:"munsell"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: decoding munsell response: %v", domain.ErrWorkerUnavailable, err)
	}
	if len(out.Munsell) != len(hexes) || len(out.Lab) != len(hexes) {
		return nil, fmt.Errorf("%w: munsell returned %d values for %d colours", domain.ErrWorkerUnavailable, len(out.Munsell), len(hexes))
	}
	res := make([]Colour, len(hexes))
	for i := range hexes {
		res[i] = Colour{Lab: out.Lab[i], Munsell: out.Munsell[i]}
	}
	return res, nil
}

func buildMultipart(image []byte, filename string, fields map[string]string) (io.Reader, string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, "", fmt.Errorf("building worker request: %w", err)
		}
	}
	if filename == "" {
		filename = "photo.jpg"
	}
	imgType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if imgType == "" {
		imgType = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename=%q`, filepath.Base(filename)))
	h.Set("Content-Type", imgType)
	fw, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", fmt.Errorf("building worker request: %w", err)
	}
	if _, err := fw.Write(image); err != nil {
		return nil, "", fmt.Errorf("building worker request: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("building worker request: %w", err)
	}
	return &body, mw.FormDataContentType(), nil
}

// workerError is the worker's {"detail": {"code", "message"}} error body.
type workerError struct {
	Detail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"detail"`
}

func (c *Client) post(ctx context.Context, path string, body io.Reader, ctype string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("building worker request: %w", err)
	}
	req.Header.Set("Content-Type", ctype)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrWorkerUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", domain.ErrWorkerUnavailable, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}

	var we workerError
	_ = json.Unmarshal(raw, &we)
	msg := we.Detail.Message
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	switch we.Detail.Code {
	case "no_face_detected":
		return nil, fmt.Errorf("%w: %s", domain.ErrNoFace, msg)
	case "measurement_failed":
		return nil, fmt.Errorf("%w: %s", domain.ErrMeasurementFailed, msg)
	case "invalid_image", "invalid_request", "invalid_hex", "unsupported_region":
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidInput, msg)
	}
	return nil, fmt.Errorf("%w: worker returned %d: %s", domain.ErrWorkerUnavailable, resp.StatusCode, msg)
}
