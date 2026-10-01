package facearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/headclient"
)

const (
	CodeUploadTooLarge      = "upload_too_large"
	CodeFaceHeadUnreachable = "face_head_unreachable"
	CodeFaceHeadTimeout     = "face_head_timeout"
)

// HeadViews are the multipart fields the head route accepts; front is required.
var HeadViews = []string{"front", "left", "right"}

type HeadFitter interface {
	Head(ctx context.Context, parts []headclient.Part) (int, string, []byte, error)
}

// HeadHandler serves POST /api/v1/vision/face-architecture/:brandId/:applicationId/head.
type HeadHandler struct {
	fitter         HeadFitter
	profiles       ProfileFinder
	maxUploadBytes int64
}

func NewHeadHandler(f HeadFitter, p ProfileFinder, maxUploadBytes int64) *HeadHandler {
	return &HeadHandler{fitter: f, profiles: p, maxUploadBytes: maxUploadBytes}
}

// Head forwards up to three photos (front required, left and right optional)
// to the face worker and returns its response unchanged: the GLB, or the
// worker's JSON error.
//
//	400 no front photo, or an upload that cannot be read
//	409 no active profile for brand+app, brand+*, or *+*
//	413 the upload exceeds the configured cap
//	500 profile lookup failed
//	502 the face worker is unreachable; 504 it timed out
func (h *HeadHandler) Head(c echo.Context) error {
	r := c.Request()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return c.JSON(http.StatusRequestEntityTooLarge, detail(CodeUploadTooLarge, nil))
		}
		return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, nil))
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File["front"]) == 0 {
		return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"view": "front"}))
	}
	if _, failed, err := activeProfile(c, h.profiles); failed {
		return err
	}
	var parts []headclient.Part
	for _, name := range HeadViews {
		fhs := r.MultipartForm.File[name]
		if len(fhs) == 0 {
			continue
		}
		f, err := fhs[0].Open()
		if err != nil {
			return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"view": name}))
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"view": name}))
		}
		parts = append(parts, headclient.Part{Name: name, Filename: fhs[0].Filename, Data: data})
	}
	status, ct, body, err := h.fitter.Head(r.Context(), parts)
	if err != nil {
		slog.ErrorContext(r.Context(), "face-head call failed", "err", err)
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return c.JSON(http.StatusGatewayTimeout, detail(CodeFaceHeadTimeout, nil))
		}
		return c.JSON(http.StatusBadGateway, detail(CodeFaceHeadUnreachable, nil))
	}
	return c.Blob(status, ct, body)
}
