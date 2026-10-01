package domain

type RawImageInput struct {
	Angle    ImageAngle `json:"angle"`
	FileName string     `json:"fileName"`
	Data     []byte     `json:"-"`
}

type AnalyzeVisionRequest struct {
	BrandID        string          `json:"brandId"`
	ApplicationID  string          `json:"applicationId"`
	Images         []RawImageInput `json:"images"`
	Dimensions     []string        `json:"dimensions"`
	SkinConditions []string        `json:"skinConditions"`
	CurrentAge     int             `json:"currentAge"`
}

type ProcessedImageMeta struct {
	Angle            ImageAngle `json:"angle"`
	FileName         string     `json:"fileName"`
	LandmarkDetected bool       `json:"landmarkDetected"`
	// LandmarkConfidence is nil: the skin worker reports no landmark
	// confidence (MediaPipe FaceLandmarker gives none), so none is shown.
	LandmarkConfidence *float64 `json:"landmarkConfidence,omitempty"`
	Yaw                float64  `json:"yaw"`
	Pitch              float64  `json:"pitch"`
	Roll               float64  `json:"roll"`
}

type CaptureContext struct {
	CaptureMode          string               `json:"captureMode"` // "SINGLE_ANGLE" | "MULTI_ANGLE"
	InputCount           int                  `json:"inputCount"`
	ProvidedAngles       []ImageAngle         `json:"providedAngles"`
	CoverageCompleteness string               `json:"coverageCompleteness"`
	ProcessedImages      []ProcessedImageMeta `json:"processedImages"`
}

type MetricValue struct {
	Score         float64 `json:"score"`
	Severity      string  `json:"severity,omitempty"`
	DetectedCount int     `json:"detectedCount,omitempty"`
	Priority      string  `json:"priority,omitempty"`
}

type GlobalAggregation struct {
	// OverallSkinHealthScore is null when nothing was scored; the warnings
	// say why.
	OverallSkinHealthScore *float64               `json:"overallSkinHealthScore"`
	Dimensions             map[string]MetricValue `json:"dimensions"`
	SkinConditions         map[string]MetricValue `json:"skinConditions"`
}

type ZoneMetrics struct {
	Dimensions     map[string]MetricValue `json:"dimensions"`
	SkinConditions map[string]MetricValue `json:"skinConditions"`
}

type ZoneBreakdownItem struct {
	ZoneCode    string     `json:"zoneCode"`
	ZoneName    string     `json:"zoneName"`
	SourceAngle ImageAngle `json:"sourceAngle"`
	// IsVisible is false when the zone was out of frame in every image.
	IsVisible   bool              `json:"isVisible"`
	BoundingBox BoundingBox       `json:"boundingBox"`
	Polygon     []NormalizedPoint `json:"polygon"`
	Metrics     ZoneMetrics       `json:"metrics"`
}

type WarningItem struct {
	Code       string  `json:"code"`
	ZoneCode   string  `json:"zoneCode,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Message    string  `json:"message"`
}

type ExecutionMetrics struct {
	TotalLatencyMs      int64    `json:"totalLatencyMs"`
	ExecutedModelsCount int      `json:"executedModelsCount"`
	SkippedModelsCount  int      `json:"skippedModelsCount"`
	ExecutedModels      []string `json:"executedModels"`
}

type VisionAnalysisResponse struct {
	AnalysisID        string              `json:"analysisId"`
	BrandID           string              `json:"brandId"`
	ApplicationID     string              `json:"applicationId"`
	Status            string              `json:"status"`
	CaptureContext    CaptureContext      `json:"captureContext"`
	GlobalAggregation GlobalAggregation   `json:"globalAggregation"`
	ZoneBreakdown     []ZoneBreakdownItem `json:"zoneBreakdown"`
	Warnings          []WarningItem       `json:"warnings"`
	ExecutionMetrics  ExecutionMetrics    `json:"executionMetrics"`
}

type ValidationErrorItem struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type ValidationErrorResponse struct {
	Error             string                `json:"error"`
	Message           string                `json:"message"`
	InvalidParameters []ValidationErrorItem `json:"invalidParameters"`
}
