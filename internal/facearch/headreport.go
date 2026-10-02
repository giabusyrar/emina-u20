package facearch

// HeadReport is asset.extras of the GLB returned by
// POST /api/v1/vision/face-architecture/:brandId/:applicationId/head. The
// face worker builds it; this struct is the contract's source for the
// gateway contracts generator. testdata/head_report.json is shared with the
// worker's test (apps/workers/face/tests/test_head_report.py).
type HeadReport struct {
	Kind  string       `json:"kind"`
	Model HeadModelRef `json:"model"`
	// Segmenter is the photo segmentation model whose classes decide which
	// pixels may texture skin and the eyeballs, and so which vertices count as
	// observed.
	Segmenter HeadModelRef `json:"segmenter"`
	// Parts lists the GLB primitives and what each claims. On skin and
	// eyeball parts, _OBSERVED > 0 at a vertex: a photo pixel of the right
	// class (skin for skin, the eye region for eyeballs) saw it; 0: no photo
	// saw it, or its pixel was hair, clothes, background or (for an eyeball)
	// outside the eye opening. On hair_* parts, _OBSERVED > 0 where a photo's
	// hair/covering pixel saw the vertex. Textured parts carry photo colour on
	// triangles whose vertices are all observed and, where fill applied, an
	// estimated colour elsewhere; where it did not, grey (skin, eyeballs) or
	// transparent (hair_*). The cornea is geometry only (always 0).
	Parts []HeadPart `json:"parts"`
	// Fill says, per filled part, whether its unseen region carries an
	// estimated colour (diffused from the seen vertices toward the part's
	// median tone) or stays grey, and why. Filled vertices keep _OBSERVED 0.
	Fill []HeadFill `json:"fill"`
	// Hair is the hair / head-covering geometry (parts hair_shell, hair_drape
	// when the drape is built, hair_cards): fitted to the photos' head-top
	// outlines, estimated where _OBSERVED is 0, without _SIGMA_MM, and never
	// used for measurement. Reason is set only when Built is false; Fill and
	// StripsFill only when Built is true; Source whenever head-top pixels were
	// seen (Built, or Reason "no_shell" / "not_converged").
	Hair            HeadHair           `json:"hair"`
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

type HeadPart struct {
	Name              string `json:"name"`
	Textured          bool   `json:"textured"`
	ObservedFromPhoto bool   `json:"observedFromPhoto"`
}

// HeadFill: Reason is set only when Applied is false ("too_few_observed" or
// "not_converged"); ToneLinear/ToneSrgbHex are present when the fill was
// attempted (anchors cleared the gate): applied, or not_converged; absent for
// too_few_observed. The tone is an estimate derived from this request's photos.
//
// The three counts are over the part's welded GNM model vertices. The GLB
// primitive is unwelded by UV, so counts taken from the GLB can differ by seam
// copies. ObservedVertices: _OBSERVED > 0 with non-zero photo weight.
// AnchorVertices: the subset the fill is anchored to (seen frontally enough in
// some single view); the minimum-fraction gate behind "too_few_observed"
// counts AnchorVertices, not ObservedVertices. TotalVertices: all of the
// part's welded vertices.
//
// Hair fills (HeadHair.Fill, HeadHair.StripsFill) differ: their counts are
// over their own part's vertices (the shell's; the drape's and cards'), only
// mask-interior seen vertices anchor them, and "too_few_observed" means fewer
// than an absolute FACE_HEAD_HAIR_FILL_MIN_ANCHORS anchor vertices, not a
// fraction of the part.
type HeadFill struct {
	Part             string      `json:"part"`
	Applied          bool        `json:"applied"`
	Reason           string      `json:"reason,omitempty"`
	Method           string      `json:"method"`
	ToneLinear       *[3]float64 `json:"toneLinear,omitempty"`
	ToneSrgbHex      string      `json:"toneSrgbHex,omitempty"`
	ObservedVertices int         `json:"observedVertices"`
	AnchorVertices   int         `json:"anchorVertices"`
	TotalVertices    int         `json:"totalVertices"`
	DecayMm          float64     `json:"decayMm"`
}

// HeadHair: Reason is "no_head_top_pixels" (no view has hair or covering
// pixels), "no_shell" (head-top pixels, but no shell triangle reached the
// minimum thickness of one outline march step) or "not_converged" (the shell
// thickness diffusion did not converge); in each case no hair_* parts are in
// the GLB and the rest of the head is unchanged. Source ("hair", "covering" or
// "both") is present whenever head-top pixels were seen: when Built, and with
// "no_shell" or "not_converged". Fill is the shell atlas's colour fill;
// StripsFill (part "hair_strips") the second atlas's, which holds the drape
// when built and the cards, present only when that atlas was baked. Both are
// gated by an absolute anchor count (FACE_HEAD_HAIR_FILL_MIN_ANCHORS).
// DrapeDepthMm is present only when the drape was built.
type HeadHair struct {
	Built                 bool          `json:"built"`
	Reason                string        `json:"reason,omitempty"`
	Source                string        `json:"source,omitempty"`
	Parts                 []string      `json:"parts"`
	ShellObservedVertices int           `json:"shellObservedVertices"`
	ShellTotalVertices    int           `json:"shellTotalVertices"`
	Drape                 HeadHairDrape `json:"drape"`
	MaxThicknessMm        float64       `json:"maxThicknessMm"`
	DrapeDepthMm          *float64      `json:"drapeDepthMm,omitempty"`
	Fill                  *HeadFill     `json:"fill,omitempty"`
	StripsFill            *HeadFill     `json:"stripsFill,omitempty"`
}

// HeadHairDrape: DepthSource is "views" (side photos constrained it) or
// "estimated" (FACE_HEAD_HAIR_DRAPE_DEPTH_MM behind the neck), set only when Built.
type HeadHairDrape struct {
	Built       bool   `json:"built"`
	DepthSource string `json:"depthSource,omitempty"`
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
