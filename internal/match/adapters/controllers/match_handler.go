package controllers

import (
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/match/adapters/objectstore"
	"github.com/experience-gateway/apps/core-engine/internal/match/domain"
	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
	"github.com/experience-gateway/apps/core-engine/internal/match/service"
	"github.com/experience-gateway/packages/goplatform"
)

type MatchHandler struct {
	svc       *service.MatchService
	presigner objectstore.Presigner
}

// NewMatchHandler takes a nil presigner when object storage is not
// configured; shade asset reads then answer 503.
func NewMatchHandler(svc *service.MatchService, presigner objectstore.Presigner) *MatchHandler {
	return &MatchHandler{svc: svc, presigner: presigner}
}

// Evaluate handles POST /api/matching/evaluate
func (h *MatchHandler) Evaluate(c echo.Context) error {
	var req domain.MatchRequest
	if err := c.Bind(&req); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
	}

	resp, err := h.svc.Evaluate(c.Request().Context(), req)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Product Catalog Handlers (read-only listing, used by the Product Group picker)
func (h *MatchHandler) ListProducts(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	items, err := h.svc.ListProducts(c.Request().Context(), brandID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "products": items})
}

// Conflict Matrix Handlers
func (h *MatchHandler) ListConflictRules(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")
	items, err := h.svc.ListConflictRules(c.Request().Context(), brandID, appID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "conflicts": items})
}

func (h *MatchHandler) CreateConflictRule(c echo.Context) error {
	var item entity.ConflictMatrixRule
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	created, err := h.svc.CreateConflictRule(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "conflict": created})
}

func (h *MatchHandler) UpdateConflictRule(c echo.Context) error {
	var item entity.ConflictMatrixRule
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	updated, err := h.svc.UpdateConflictRule(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "conflict": updated})
}

func (h *MatchHandler) DeleteConflictRule(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "Missing id")
	}
	if err := h.svc.DeleteConflictRule(c.Request().Context(), id); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Product Group Handlers (brand campaign product/category curation)
func (h *MatchHandler) ListProductGroups(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")
	items, err := h.svc.ListProductGroups(c.Request().Context(), brandID, appID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "groups": items})
}

func (h *MatchHandler) CreateProductGroup(c echo.Context) error {
	var item entity.ProductGroup
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	if item.BrandID == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "brandId is required")
	}
	created, err := h.svc.CreateProductGroup(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "group": created})
}

func (h *MatchHandler) UpdateProductGroup(c echo.Context) error {
	var item entity.ProductGroup
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	updated, err := h.svc.UpdateProductGroup(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "group": updated})
}

func (h *MatchHandler) DeleteProductGroup(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "Missing id")
	}
	if err := h.svc.DeleteProductGroup(c.Request().Context(), id); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Shade Handlers (product color variants; extraction runs in tryon-engine)
func (h *MatchHandler) ListShades(c echo.Context) error {
	productID := c.QueryParam("product_id")
	items, err := h.svc.ListShades(c.Request().Context(), productID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "shades": items})
}

func (h *MatchHandler) CreateShade(c echo.Context) error {
	var item entity.Shade
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	if item.ProductID == "" || item.HexColor == "" || item.Region == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "productId, hexColor, and region are required")
	}
	created, err := h.svc.CreateShade(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}

	if created.ReferencePhotoURL != "" {
		if err := h.svc.TriggerShadeExtraction(c.Request().Context(), *created); err != nil {
			log.Printf("failed to trigger shade extraction for %s: %v", created.ID, err)
		}
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "shade": created})
}

func (h *MatchHandler) UpdateShade(c echo.Context) error {
	var item entity.Shade
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	updated, err := h.svc.UpdateShade(c.Request().Context(), item)
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "shade": updated})
}

func (h *MatchHandler) DeleteShade(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return goplatform.JSONError(c, http.StatusBadRequest, "Missing id")
	}
	if err := h.svc.DeleteShade(c.Request().Context(), id); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// shadeAssetResponse is the web contract for a shade asset: URLs, freshly
// presigned per request. The stored keys never leave the service.
type shadeAssetResponse struct {
	ID           string    `json:"id"`
	ShadeID      string    `json:"shadeId"`
	ColorMapURL  string    `json:"colorMapUrl"`
	AlphaMapURL  string    `json:"alphaMapUrl"`
	FinishMapURL string    `json:"finishMapUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// GetShadeAsset returns one ShadeAsset by its own ID (GET /api/matching/shade-assets/:id),
// used by the live try-on frontend to resolve a Shade's AssetID into texture URLs.
// The URLs are presigned per request from the stored object keys.
func (h *MatchHandler) GetShadeAsset(c echo.Context) error {
	if h.presigner == nil {
		return goplatform.JSONError(c, http.StatusServiceUnavailable, "object storage not configured")
	}
	ctx := c.Request().Context()
	asset, err := h.svc.GetShadeAssetByID(ctx, c.Param("id"))
	if err != nil {
		return goplatform.JSONError(c, http.StatusNotFound, "shade asset not found")
	}
	if asset.ColorMapKey == "" || asset.AlphaMapKey == "" {
		return goplatform.JSONError(c, http.StatusConflict, "shade asset has no storage keys; run the shade-asset-keys migration")
	}
	resp := shadeAssetResponse{ID: asset.ID, ShadeID: asset.ShadeID, CreatedAt: asset.CreatedAt}
	for _, m := range []struct {
		key string
		dst *string
	}{
		{asset.ColorMapKey, &resp.ColorMapURL},
		{asset.AlphaMapKey, &resp.AlphaMapURL},
		{asset.FinishMapKey, &resp.FinishMapURL},
	} {
		if m.key == "" {
			continue
		}
		u, err := h.presigner.Presign(ctx, m.key)
		if err != nil {
			slog.ErrorContext(ctx, "presign shade asset failed", "asset", asset.ID, "err", err)
			return goplatform.JSONError(c, http.StatusInternalServerError, "could not sign shade asset links")
		}
		*m.dst = u
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "asset": resp})
}

// ExtractionCallback is called by tryon-engine (POST /api/matching/shades/:id/extraction-callback)
// when an offline shade-extraction job finishes, successfully or not.
func (h *MatchHandler) ExtractionCallback(c echo.Context) error {
	shadeID := c.Param("id")
	var payload struct {
		Status       string `json:"status"` // "ready" | "failed"
		Reason       string `json:"reason,omitempty"`
		ColorMapKey  string `json:"colorMapKey,omitempty"`
		AlphaMapKey  string `json:"alphaMapKey,omitempty"`
		FinishMapKey string `json:"finishMapKey,omitempty"`
	}
	if err := c.Bind(&payload); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, "Invalid JSON")
	}
	if payload.Status != "ready" && payload.Status != "failed" {
		return goplatform.JSONError(c, http.StatusBadRequest, "status must be 'ready' or 'failed'")
	}
	if payload.Status == "ready" && (payload.ColorMapKey == "" || payload.AlphaMapKey == "") {
		return goplatform.JSONError(c, http.StatusBadRequest, "status 'ready' requires colorMapKey and alphaMapKey")
	}

	target, err := h.svc.GetShadeByID(c.Request().Context(), shadeID)
	if err != nil {
		return goplatform.JSONError(c, http.StatusNotFound, "shade not found")
	}

	target.ExtractionStatus = payload.Status
	target.FailureReason = payload.Reason

	if payload.Status == "ready" {
		asset, err := h.svc.SaveShadeAsset(c.Request().Context(), entity.ShadeAsset{
			ShadeID:      shadeID,
			ColorMapKey:  payload.ColorMapKey,
			AlphaMapKey:  payload.AlphaMapKey,
			FinishMapKey: payload.FinishMapKey,
		})
		if err != nil {
			return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
		}
		target.AssetID = &asset.ID
	}

	if _, err := h.svc.UpdateShade(c.Request().Context(), *target); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}
