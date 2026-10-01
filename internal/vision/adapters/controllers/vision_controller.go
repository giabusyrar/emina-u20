package controllers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
	"github.com/experience-gateway/apps/core-engine/internal/vision/inference"
	"github.com/experience-gateway/apps/core-engine/internal/vision/orchestrator"
	"github.com/experience-gateway/apps/core-engine/internal/vision/workers"
)

type VisionController struct {
	orchestrator *orchestrator.VisionOrchestrator
	catalog      catalog.Catalog
}

func NewVisionController(orch *orchestrator.VisionOrchestrator, referenceCatalog catalog.Catalog) *VisionController {
	return &VisionController{
		orchestrator: orch,
		catalog:      referenceCatalog,
	}
}

func (c *VisionController) parseStringList(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (c *VisionController) extractImageFile(ctx echo.Context, fieldName string, angle domain.ImageAngle) (*domain.RawImageInput, error) {
	file, err := ctx.FormFile(fieldName)
	if err != nil {
		return nil, nil // optional field
	}
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}

	return &domain.RawImageInput{
		Angle:    angle,
		FileName: file.Filename,
		Data:     data,
	}, nil
}

func (c *VisionController) AnalyzeImage(ctx echo.Context) error {
	var images []domain.RawImageInput

	// 1. Check all uploaded multipart files
	form, err := ctx.MultipartForm()
	if err == nil && form != nil {
		for key, fileHeaders := range form.File {
			for _, fh := range fileHeaders {
				src, err := fh.Open()
				if err != nil {
					continue
				}
				data, err := io.ReadAll(src)
				_ = src.Close()
				if err != nil {
					continue
				}

				angle := domain.ImageAngle("")
				switch key {
				case "image_front":
					angle = domain.AngleFront
				case "image_left":
					angle = domain.AngleLeft
				case "image_right":
					angle = domain.AngleRight
				}

				images = append(images, domain.RawImageInput{
					Angle:    angle,
					FileName: fh.Filename,
					Data:     data,
				})
			}
		}
	}

	// 2. Fallback for single "image" or "file" field if not in multipart map
	if len(images) == 0 {
		if img, _ := c.extractImageFile(ctx, "image", ""); img != nil {
			images = append(images, *img)
		} else if fileImg, _ := c.extractImageFile(ctx, "file", ""); fileImg != nil {
			images = append(images, *fileImg)
		}
	}

	req := &domain.AnalyzeVisionRequest{
		BrandID:        ctx.FormValue("brandId"),
		ApplicationID:  ctx.FormValue("applicationId"),
		Images:         images,
		Dimensions:     c.parseStringList(ctx.FormValue("dimensions")),
		SkinConditions: c.parseStringList(ctx.FormValue("skinConditions")),
	}

	if req.BrandID == "" {
		req.BrandID = "brand_wardah"
	}
	if req.ApplicationID == "" {
		req.ApplicationID = "app_diagnostic"
	}

	// 1. Strict Parameter Validation
	if valErr := domain.ValidateRequest(req); valErr != nil {
		return ctx.JSON(http.StatusBadRequest, valErr)
	}

	// 2. DAG Orchestration Execution
	resp, err := c.orchestrator.Execute(ctx.Request().Context(), req)
	if err != nil {
		return c.writeError(ctx, err)
	}

	return ctx.JSON(http.StatusOK, resp)
}

// writeError maps pipeline failures to HTTP responses.
func (c *VisionController) writeError(ctx echo.Context, err error) error {
	var validationErr *catalog.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return ctx.JSON(http.StatusBadRequest, domain.ValidationErrorResponse{
			Error:             "BAD_REQUEST",
			Message:           "Validation failed on requested parameters",
			InvalidParameters: validationErr.Items,
		})
	case errors.Is(err, catalog.ErrCatalogUnavailable):
		return ctx.JSON(http.StatusServiceUnavailable, map[string]string{
			"error":   "REFERENCE_DATA_UNAVAILABLE",
			"message": err.Error(),
		})
	case errors.Is(err, workers.ErrSegmentationUnavailable):
		return ctx.JSON(http.StatusServiceUnavailable, map[string]string{
			"error":   "SEGMENTATION_UNAVAILABLE",
			"message": err.Error(),
		})
	case errors.Is(err, inference.ErrWorkerUnavailable):
		return ctx.JSON(http.StatusBadGateway, map[string]string{
			"error":   "VISION_WORKER_UNAVAILABLE",
			"message": err.Error(),
		})
	default:
		return ctx.JSON(http.StatusInternalServerError, map[string]string{
			"error":   "INTERNAL_SERVER_ERROR",
			"message": err.Error(),
		})
	}
}

type registryDimension struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	SkinConditions []string `json:"skinConditions"`
}

type registrySkinCondition struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DimensionCode string   `json:"dimensionCode"`
	Capabilities  []string `json:"capabilities"`
}

type registryResponse struct {
	Dimensions     []registryDimension     `json:"dimensions"`
	SkinConditions []registrySkinCondition `json:"skinConditions"`
}

// GetRegistry returns the vision-relevant dimensions and skin conditions from
// Reference Data. Consumers (including the web frontend) must fetch this
// instead of hardcoding their own copy.
func (c *VisionController) GetRegistry(ctx echo.Context) error {
	snapshot, err := c.catalog.Snapshot(ctx.Request().Context())
	if err != nil {
		return c.writeError(ctx, err)
	}

	response := registryResponse{
		Dimensions:     make([]registryDimension, 0, len(snapshot.Dimensions)),
		SkinConditions: make([]registrySkinCondition, 0, len(snapshot.Conditions)),
	}
	for _, dimension := range snapshot.Dimensions {
		conditionCodes := make([]string, 0, len(dimension.Conditions))
		for _, condition := range dimension.Conditions {
			conditionCodes = append(conditionCodes, condition.Code)
		}
		response.Dimensions = append(response.Dimensions, registryDimension{
			Code:           dimension.Code,
			Name:           dimension.Name,
			Description:    dimension.Description,
			SkinConditions: conditionCodes,
		})
	}
	for _, condition := range snapshot.Conditions {
		response.SkinConditions = append(response.SkinConditions, registrySkinCondition{
			Code:          condition.Code,
			Name:          condition.Name,
			Description:   condition.Description,
			DimensionCode: condition.DimensionCode,
			Capabilities:  condition.Capabilities,
		})
	}

	return ctx.JSON(http.StatusOK, response)
}

func (c *VisionController) GetConfig(ctx echo.Context) error {
	brandID := ctx.Param("brandId")
	appID := ctx.Param("applicationId")

	return ctx.JSON(http.StatusOK, map[string]interface{}{
		"brandId":               brandID,
		"applicationId":         appID,
		"agingFilterEnabled":    true,
		"uvCamEnabled":          true,
		"polygonOverlayEnabled": true,
		"uvCoverageThreshold":   85.0,
		"regimenEfficacyFactor": 0.5,
		"recommendedSpfLevel":   50,
	})
}
