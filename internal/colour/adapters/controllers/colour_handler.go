package controllers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/service"
	goplatform "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/platform"
)

// maxImageBytes caps uploads read into memory. Config, not a literal at the
// call site: set from COLOUR_MAX_IMAGE_BYTES in main.
type Handler struct {
	svc           *service.Service
	maxImageBytes int64
}

func NewHandler(svc *service.Service, maxImageBytes int64) *Handler {
	return &Handler{svc: svc, maxImageBytes: maxImageBytes}
}

// errorBody is the service's error envelope (goplatform.ErrorBody) plus
// "code", the same value as error_code: seagull-web's colour client reads
// "code" (apps/web/features/colour/types.ts). A superset, like the envelope
// itself, so no existing client loses a field it reads.
type errorBody struct {
	goplatform.ErrorBody
	Code string `json:"code"`
}

func jsonError(c echo.Context, status int, code, msg string) error {
	return c.JSON(status, errorBody{ErrorBody: goplatform.ErrorBody{Error: msg, ErrorCode: code}, Code: code})
}

// disabledCode is the error code of every colour route while the colour
// engine cannot start (e.g. no WCPA config).
const disabledCode = "colour_engine_disabled"

// Disabled answers the colour routes while the colour engine cannot start:
// 503 with the reason.
func Disabled(reason error) echo.HandlerFunc {
	return func(c echo.Context) error {
		return jsonError(c, http.StatusServiceUnavailable, disabledCode, reason.Error())
	}
}

// disabledHealth is the service's health response with the reason added.
type disabledHealth struct {
	goplatform.HealthResponse
	Error string `json:"error"`
	Code  string `json:"code"`
}

// DisabledHealth answers /health on the colour prefixes while the colour
// engine cannot start: 503 in the shape of the service's /health, with the
// WCPA config reported down and the reason added.
func DisabledHealth(service string, reason error) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusServiceUnavailable, disabledHealth{
			HealthResponse: goplatform.HealthResponse{
				Status:  "degraded",
				Service: service,
				Checks:  map[string]goplatform.CheckState{"wcpa-config": goplatform.CheckDown},
			},
			Error: reason.Error(),
			Code:  disabledCode,
		})
	}
}

// retakeBody is the 422 retake_required body: the error envelope plus
// "message" and the failed checks.
type retakeBody struct {
	errorBody
	Message string   `json:"message"`
	Reasons []string `json:"reasons"`
}

func respondErr(c echo.Context, err error) error {
	var re *domain.RetakeRequiredError
	if errors.As(err, &re) {
		const code = "retake_required"
		msg := err.Error()
		return c.JSON(http.StatusUnprocessableEntity, retakeBody{
			errorBody: errorBody{ErrorBody: goplatform.ErrorBody{Error: msg, ErrorCode: code}, Code: code},
			Message:   msg, Reasons: re.Reasons,
		})
	}
	status := http.StatusInternalServerError
	code := "internal_error"
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "invalid_input"
	case errors.Is(err, domain.ErrShadeNotFound):
		status, code = http.StatusNotFound, "shade_not_found"
	case errors.Is(err, domain.ErrNoFace):
		status, code = http.StatusUnprocessableEntity, "no_face_detected"
	case errors.Is(err, domain.ErrMeasurementFailed):
		status, code = http.StatusUnprocessableEntity, "measurement_failed"
	case errors.Is(err, domain.ErrWorkerUnavailable):
		status, code = http.StatusServiceUnavailable, "worker_unavailable"
	}
	return jsonError(c, status, code, err.Error())
}

func (h *Handler) readImage(c echo.Context) ([]byte, string, error) {
	fh, err := c.FormFile("image")
	if err != nil {
		return nil, "", errors.Join(domain.ErrInvalidInput, errors.New("multipart field 'image' is required"))
	}
	if fh.Size > h.maxImageBytes {
		return nil, "", errors.Join(domain.ErrInvalidInput, errors.New("image is too large"))
	}
	f, err := fh.Open()
	if err != nil {
		return nil, "", errors.Join(domain.ErrInvalidInput, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, h.maxImageBytes+1))
	if err != nil {
		return nil, "", errors.Join(domain.ErrInvalidInput, err)
	}
	return b, fh.Filename, nil
}

func requiredBool(c echo.Context, name string) (bool, error) {
	v := c.FormValue(name)
	if v == "" {
		return false, errors.Join(domain.ErrInvalidInput, errors.New("form field '"+name+"' is required (true/false)"))
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, errors.Join(domain.ErrInvalidInput, errors.New("form field '"+name+"' must be true or false"))
	}
	return b, nil
}

// Analyze: POST /analyze (multipart: image, hijab, hairVisible; ?debug=1).
func (h *Handler) Analyze(c echo.Context) error {
	img, name, err := h.readImage(c)
	if err != nil {
		return respondErr(c, err)
	}
	hijab, err := requiredBool(c, "hijab")
	if err != nil {
		return respondErr(c, err)
	}
	hair, err := requiredBool(c, "hairVisible")
	if err != nil {
		return respondErr(c, err)
	}
	debug := c.QueryParam("debug") == "1"
	out, err := h.svc.Analyze(c.Request().Context(), img, name, hijab, hair, debug)
	if err != nil {
		return respondErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// TryOn: POST /tryon (multipart: image, shadeIds — repeated field or comma list).
func (h *Handler) TryOn(c echo.Context) error {
	img, name, err := h.readImage(c)
	if err != nil {
		return respondErr(c, err)
	}
	var ids []string
	if form, ferr := c.MultipartForm(); ferr == nil {
		for _, v := range form.Value["shadeIds"] {
			for _, id := range strings.Split(v, ",") {
				if id = strings.TrimSpace(id); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	png, err := h.svc.TryOn(c.Request().Context(), img, name, ids)
	if err != nil {
		return respondErr(c, err)
	}
	return c.Blob(http.StatusOK, "image/png", png)
}

// Catalog: GET /catalog — every shade to try, grouped by category, without
// an analysis (try-on straight from a photo).
func (h *Handler) Catalog(c echo.Context) error {
	out, err := h.svc.Catalog(c.Request().Context())
	if err != nil {
		return respondErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// Quadrants: GET /quadrants.
func (h *Handler) Quadrants(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{"quadrants": h.svc.Quadrants()})
}
