package controllers

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/pkg/match/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/match/entity"
	"github.com/experience-gateway/apps/core-engine/pkg/match/service"
)

type MatchHandler struct {
	svc *service.MatchService
}

func NewMatchHandler(svc *service.MatchService) *MatchHandler {
	return &MatchHandler{svc: svc}
}

// Evaluate handles POST /api/matching/evaluate
func (h *MatchHandler) Evaluate(c echo.Context) error {
	var req domain.MatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON payload: " + err.Error(),
		})
	}

	resp, err := h.svc.Evaluate(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// Product Catalog Handlers (read-only listing, used by the Product Group picker)
func (h *MatchHandler) ListProducts(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	items, err := h.svc.ListProducts(c.Request().Context(), brandID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "products": items})
}

// Conflict Matrix Handlers
func (h *MatchHandler) ListConflictRules(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")
	items, err := h.svc.ListConflictRules(c.Request().Context(), brandID, appID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "conflicts": items})
}

func (h *MatchHandler) CreateConflictRule(c echo.Context) error {
	var item entity.ConflictMatrixRule
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	created, err := h.svc.CreateConflictRule(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "conflict": created})
}

func (h *MatchHandler) UpdateConflictRule(c echo.Context) error {
	var item entity.ConflictMatrixRule
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	updated, err := h.svc.UpdateConflictRule(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "conflict": updated})
}

func (h *MatchHandler) DeleteConflictRule(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Missing id"})
	}
	if err := h.svc.DeleteConflictRule(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Product Group Handlers (brand campaign product/category curation)
func (h *MatchHandler) ListProductGroups(c echo.Context) error {
	brandID := c.QueryParam("brand_id")
	appID := c.QueryParam("application_id")
	items, err := h.svc.ListProductGroups(c.Request().Context(), brandID, appID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "groups": items})
}

func (h *MatchHandler) CreateProductGroup(c echo.Context) error {
	var item entity.ProductGroup
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	if item.BrandID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "brandId is required"})
	}
	created, err := h.svc.CreateProductGroup(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{"success": true, "group": created})
}

func (h *MatchHandler) UpdateProductGroup(c echo.Context) error {
	var item entity.ProductGroup
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	updated, err := h.svc.UpdateProductGroup(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "group": updated})
}

func (h *MatchHandler) DeleteProductGroup(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Missing id"})
	}
	if err := h.svc.DeleteProductGroup(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Shade Handlers (product color variants; extraction runs in tryon-engine)
func (h *MatchHandler) ListShades(c echo.Context) error {
	productID := c.QueryParam("product_id")
	items, err := h.svc.ListShades(c.Request().Context(), productID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "shades": items})
}

func (h *MatchHandler) CreateShade(c echo.Context) error {
	var item entity.Shade
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	if item.ProductID == "" || item.HexColor == "" || item.Region == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "productId, hexColor, and region are required"})
	}
	created, err := h.svc.CreateShade(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
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
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	updated, err := h.svc.UpdateShade(c.Request().Context(), item)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "shade": updated})
}

func (h *MatchHandler) DeleteShade(c echo.Context) error {
	id := c.QueryParam("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Missing id"})
	}
	if err := h.svc.DeleteShade(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// GetShadeAsset returns one ShadeAsset by its own ID (GET /api/matching/shade-assets/:id),
// used by the live try-on frontend to resolve a Shade's AssetID into actual texture URLs.
func (h *MatchHandler) GetShadeAsset(c echo.Context) error {
	id := c.Param("id")
	asset, err := h.svc.GetShadeAssetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"success": false, "error": "shade asset not found"})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true, "asset": asset})
}

// ExtractionCallback is called by tryon-engine (POST /api/matching/shades/:id/extraction-callback)
// when an offline shade-extraction job finishes, successfully or not.
func (h *MatchHandler) ExtractionCallback(c echo.Context) error {
	shadeID := c.Param("id")
	var payload struct {
		Status       string `json:"status"` // "ready" | "failed"
		Reason       string `json:"reason,omitempty"`
		ColorMapURL  string `json:"colorMapUrl,omitempty"`
		AlphaMapURL  string `json:"alphaMapUrl,omitempty"`
		FinishMapURL string `json:"finishMapUrl,omitempty"`
	}
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid JSON"})
	}
	if payload.Status != "ready" && payload.Status != "failed" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"success": false, "error": "status must be 'ready' or 'failed'"})
	}

	target, err := h.svc.GetShadeByID(c.Request().Context(), shadeID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{"success": false, "error": "shade not found"})
	}

	target.ExtractionStatus = payload.Status
	target.FailureReason = payload.Reason

	if payload.Status == "ready" {
		asset, err := h.svc.SaveShadeAsset(c.Request().Context(), entity.ShadeAsset{
			ShadeID:      shadeID,
			ColorMapURL:  payload.ColorMapURL,
			AlphaMapURL:  payload.AlphaMapURL,
			FinishMapURL: payload.FinishMapURL,
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		}
		target.AssetID = &asset.ID
	}

	if _, err := h.svc.UpdateShade(c.Request().Context(), *target); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}
