// Package visionclient calls the Paradev Skin Analyzer vendor through our
// own gateway; this package only ever sends our gateway key (X-API-Key),
// never a vendor key. Whatever reaches the vendor is set by the gateway
// route's config (Seagull gateway-engine), not here. As verified 2026-09-30,
// that config does NOT inject a Paradev-issued key: it forwards one of our
// own gateway keys to the vendor (reported to Seagull-gateway to fix).
package visionclient

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
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

type Client struct {
	GatewayURL string
	APIKey     string
	HTTP       *http.Client
}

// New builds a client with an explicit timeout. Paradev's scoring call runs
// a synchronous LLM pass — per the reviewed spec this needs a generous
// timeout (45-60s), not the short default used for our own internal calls.
// The timeout comes from config (PARADEV_TIMEOUT).
func New(gatewayURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		GatewayURL: gatewayURL,
		APIKey:     apiKey,
		HTTP:       &http.Client{Timeout: timeout},
	}
}

// Submission is what the caller already has in hand before it can ask
// Paradev anything: the applicant's personal data (consent already
// collected) and their photo(s).
type Submission struct {
	FullName              string
	PhoneNumber           string
	Email                 string
	DateOfBirth           string // YYYY-MM-DD
	ConsentDataProcessing bool
	ConsentMarketing      bool
	Image                 io.Reader
	ImageFilename         string
}

// Result is the vendor's response, reduced to what the rest of the
// orchestration needs: every numeric leaf value the vendor returned,
// flattened by its full dot-path from the response root (matching how
// field_mapping's vision codes are recorded — e.g.
// "inference_result.results.skin_scoring.Pores"), plus a trace id for support.
type Result struct {
	Flattened     map[string]float64
	TransactionID string
	Raw           map[string]interface{}
}

// These four fields are configured as "static" in the Paradev route's own
// Route Settings (Seagull gateway-engine) — for a JSON body the gateway
// injects them itself, but that injection is deliberately skipped for a
// multipart body (it can't parse/re-serialize multipart), so this caller
// must supply them directly. Keep in sync with Route Settings if the vendor
// integration's fixed values ever change.
const (
	staticTemplateUUID = "01999999-9999-7999-9999-999999999902"
	staticIsGuestMode  = "true"
	staticUseLLM       = "true"
	staticIsAdhocReq   = "true"
)

// Submit sends the multipart request and returns the vendor's flattened
// result. Any failure — network, timeout, non-2xx — is wrapped in
// domain.ErrVisionUnavailable so the caller can fail the whole evaluation
// fast rather than return a profile silently missing its vision-derived axes.
func (c *Client) Submit(ctx context.Context, sub Submission) (*Result, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	fields := map[string]string{
		"template_uuid":           staticTemplateUUID,
		"full_name":               sub.FullName,
		"phone_number":            sub.PhoneNumber,
		"email":                   sub.Email,
		"date_of_birth":           sub.DateOfBirth,
		"consent_data_processing": boolStr(sub.ConsentDataProcessing),
		"consent_marketing":       boolStr(sub.ConsentMarketing),
		"is_guest_mode":           staticIsGuestMode,
		"use_llm":                 staticUseLLM,
		"is_adhoc_req":            staticIsAdhocReq,
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("building vision request: %w", err)
		}
	}
	if sub.Image != nil {
		// mime/multipart's own CreateFormFile always stamps
		// application/octet-stream regardless of filename — Paradev checks
		// the part's actual Content-Type and rejects that as an unsupported
		// format, so the part is built by hand with the real image type.
		imgType := mime.TypeByExtension(filepath.Ext(sub.ImageFilename))
		if imgType == "" {
			imgType = "image/jpeg"
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="images"; filename="%s"`, sub.ImageFilename))
		h.Set("Content-Type", imgType)
		fw, err := mw.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("building vision request: %w", err)
		}
		if _, err := io.Copy(fw, sub.Image); err != nil {
			return nil, fmt.Errorf("building vision request: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("building vision request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.GatewayURL, &body)
	if err != nil {
		return nil, fmt.Errorf("building vision request: %w", err)
	}
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	httpReq.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrVisionUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: reading response: %v", domain.ErrVisionUnavailable, err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: vendor returned %d: %s", domain.ErrVisionUnavailable, resp.StatusCode, truncate(string(respBody), 500))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("%w: invalid vendor response: %v", domain.ErrVisionUnavailable, err)
	}

	txID := firstStringField(raw, "transaction_id", "trx_id", "request_id", "id")
	if txID == "" {
		// Our own gateway stamps every response with this, so there is
		// always something to trace back through even when the vendor's
		// own payload carries no id of its own.
		txID = resp.Header.Get("X-Request-Id")
	}

	return &Result{
		Flattened:     flattenNumeric(raw),
		TransactionID: txID,
		Raw:           raw,
	}, nil
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstStringField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// flattenNumeric walks the whole vendor response and collects every leaf
// numeric field keyed by its FULL dot-path from the response root (e.g.
// "inference_result.baumann.dimensions.wrinkle.score_wrinkle",
// "inference_result.results.skin_scoring.Pores").
//
// A leaf-name-only key was tried first and found unsafe: the vendor's own
// response repeats the same short name in more than one place with
// different meanings/scales — confirmed via a real staging response where
// "Pores" appears both under results.skin_level (a 1-5 severity grade) and
// results.skin_scoring (a 0-100 health score). Since Go map iteration order
// is randomized, matching by leaf name alone could silently pick either one
// on any given call. field_mapping.vision values must be the full path.
func flattenNumeric(v interface{}) map[string]float64 {
	out := make(map[string]float64)
	var walk func(node interface{}, path string)
	walk = func(node interface{}, path string) {
		switch n := node.(type) {
		case map[string]interface{}:
			for k, v := range n {
				next := k
				if path != "" {
					next = path + "." + k
				}
				walk(v, next)
			}
		case float64:
			if path != "" {
				out[path] = n
			}
		}
	}
	walk(v, "")
	return out
}
