package facearch

// HeadReport is asset.extras of the GLB returned by
// POST /api/v1/vision/face-architecture/:brandId/:applicationId/head. The
// face worker builds it; this struct is the contract's source for the
// gateway contracts generator. testdata/head_report.json is shared with the
// worker's test (apps/workers/face/tests/test_head_report.py).
type HeadReport struct {
	Kind            string             `json:"kind"`
	Model           HeadModelRef       `json:"model"`
	Correspondence  HeadCorrespondence `json:"correspondence"`
	Camera          HeadCamera         `json:"camera"`
	Views           []HeadView         `json:"views"`
	NMELimit        float64            `json:"nmeLimit"`
	ViewConsistency *HeadConsistency   `json:"viewConsistency"`
	ShapeEvidence   HeadShapeEvidence  `json:"shapeEvidence"`
	SigmaKind       string             `json:"sigmaKind"`
	Components      HeadComponents     `json:"components"`
	// LandmarkVertices and LandmarkPoints are dense in MediaPipe order: entry
	// i is MediaPipe landmark i for i in 0..467, every one mapped; iris
	// landmarks 468-477 are absent past the end. LandmarkVertices holds GNM
	// model vertex indices + barycentric weights; LandmarkPoints holds the same
	// points as [x, y, z] in the GLB's mesh space (metres, the POSITION frame),
	// because the GLB renumbers vertices per primitive.
	LandmarkVertices [][6]float64   `json:"landmarkVertices"`
	LandmarkPoints   [][3]float64   `json:"landmarkPoints"`
	ObservedBits     map[string]int `json:"observedBits"`
	Notice           string         `json:"notice"`
}

type HeadModelRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type HeadCorrespondence struct {
	Version int    `json:"version"`
	SHA256  string `json:"sha256"`
}

// HeadCamera: FovSource is "estimated" or "prior_dominated" (single view, or
// the posterior barely moved from the prior).
type HeadCamera struct {
	FovDeg     float64 `json:"fovDeg"`
	FovSdDeg   float64 `json:"fovSdDeg"`
	FovSource  string  `json:"fovSource"`
	PriorDeg   float64 `json:"priorDeg"`
	PriorLogSd float64 `json:"priorLogSd"`
}

// HeadView: ResidualsPx is keyed by MediaPipe landmark index; iris landmarks
// (468-477) are not mapped and are absent, never zero-filled.
type HeadView struct {
	Name        string             `json:"name"`
	YawDeg      float64            `json:"yawDeg"`
	PitchDeg    float64            `json:"pitchDeg"`
	RollDeg     float64            `json:"rollDeg"`
	NME         float64            `json:"nme"`
	FocalPx     float64            `json:"focalPx"`
	ResidualsPx map[string]float64 `json:"residualsPx"`
}

// HeadConsistency is report-only. Flag: "consistent", "views_may_differ",
// "unconfigured" (no threshold) or "undetermined" (a per-view refit failed;
// RmsMm is null). Null for a single-view request.
type HeadConsistency struct {
	RmsMm       *float64 `json:"rmsMm"`
	ThresholdMm *float64 `json:"thresholdMm"`
	Flag        string   `json:"flag"`
}

type HeadShapeEvidence struct {
	EffectiveComponents float64 `json:"effectiveComponents"`
	DistanceFromMean    float64 `json:"distanceFromMean"`
}

type HeadComponents struct {
	Identity  int `json:"identity"`
	LowerFace int `json:"lowerFace"`
	Eye       int `json:"eye"`
}
