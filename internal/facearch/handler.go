package facearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Measurer is the ai-worker face-measure call (measureclient.Client).
type Measurer interface {
	Measure(ctx context.Context, image []byte, filename string) (*measureclient.Response, error)
}

// ProfileFinder finds the active profile for a brand and application.
type ProfileFinder interface {
	Active(ctx context.Context, brandID, appID string) (*profile.FaceArchitectureProfile, error)
}

// Error codes core-engine itself returns. Every error body has the shape
// {"detail": {"code": ..., ...}}, the same shape as the ai-worker errors
// passed through unchanged, so a client reads detail.code for all of them.
const (
	CodeMissingImage           = "missing_image"
	CodeNoActiveProfile        = "no_active_face_architecture_profile"
	CodeProfileLookupFailed    = "profile_lookup_failed"
	CodeProfileDocumentInvalid = "profile_document_invalid"
	CodeFaceMeasureUnreachable = "face_measure_unreachable"
)

// Handler serves POST /api/v1/vision/face-architecture/:brandId/:applicationId.
type Handler struct {
	measurer Measurer
	profiles ProfileFinder
}

// NewHandler wires the handler.
func NewHandler(m Measurer, p ProfileFinder) *Handler {
	return &Handler{measurer: m, profiles: p}
}

func detail(code string, extra map[string]any) map[string]any {
	d := map[string]any{"code": code}
	for k, v := range extra {
		d[k] = v
	}
	return map[string]any{"detail": d}
}

// Analyze takes multipart field "image". The profile is looked up before
// the ai-worker is called, so a scope with no profile costs no inference.
//
//	200 interpretation
//	400 missing image, or the ai-worker's 400 body (unreadable image)
//	409 no active profile for brand+app, brand+*, or *+*
//	422 the ai-worker's body (face count or quality gates), unchanged
//	503 the ai-worker's body (model unavailable), or face_measure_unreachable
//	500 profile lookup failed or the stored document no longer loads
func (h *Handler) Analyze(c echo.Context) error {
	brandID, appID := c.Param("brandId"), c.Param("applicationId")

	fh, err := c.FormFile("image")
	if err != nil {
		return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"reason": "multipart field \"image\" is required"}))
	}
	src, err := fh.Open()
	if err != nil {
		return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"reason": err.Error()}))
	}
	data, err := io.ReadAll(src)
	_ = src.Close()
	if err != nil {
		return c.JSON(http.StatusBadRequest, detail(CodeMissingImage, map[string]any{"reason": err.Error()}))
	}

	ctx := c.Request().Context()
	p, err := h.profiles.Active(ctx, brandID, appID)
	if errors.Is(err, profile.ErrNoActiveProfile) {
		return c.JSON(http.StatusConflict, detail(CodeNoActiveProfile, map[string]any{"brandId": brandID, "applicationId": appID}))
	}
	if err != nil {
		slog.ErrorContext(ctx, "face-architecture profile lookup failed", "brandId", brandID, "applicationId", appID, "err", err)
		return c.JSON(http.StatusInternalServerError, detail(CodeProfileLookupFailed, nil))
	}
	doc, err := profile.Load(p)
	if err != nil {
		slog.ErrorContext(ctx, "face-architecture profile does not load", "profileId", p.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, detail(CodeProfileDocumentInvalid, map[string]any{"profileId": p.ID, "reason": err.Error()}))
	}

	m, err := h.measurer.Measure(ctx, data, fh.Filename)
	var up *measureclient.UpstreamError
	switch {
	case errors.As(err, &up):
		return c.Blob(up.Status, echo.MIMEApplicationJSON, up.Body)
	case err != nil:
		return c.JSON(http.StatusServiceUnavailable, detail(CodeFaceMeasureUnreachable, map[string]any{"reason": err.Error()}))
	}
	return c.JSON(http.StatusOK, Interpret(p, doc, m))
}
