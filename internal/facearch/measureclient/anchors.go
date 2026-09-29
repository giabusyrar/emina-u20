package measureclient

import "sort"

// Landmark facts the ai-worker defines in
// apps/ai-worker/ai_worker/vision/facegeom/anchors.py (catalogue
// fa-measure/1). They describe the MediaPipe mesh and the worker's region
// definitions, not policy, so they are code. They are recorded here once,
// for guidance resolution and validation, and pinned by anchors_test.go
// against testdata/anchors_fa-measure-1.json and the SHA-256 of anchors.py:
// if anchors.py changes, that test fails until the recording is regenerated
// (from the repository root, run
//
//	python apps/core-engine/internal/facearch/measureclient/testdata/record_anchors.py > apps/core-engine/internal/facearch/measureclient/testdata/anchors_fa-measure-1.json
//
// ) and this table is brought back in line.

// NLandmarks is the number of FaceLandmarker points the worker returns
// (anchors.N_LANDMARKS: 468 mesh points plus 10 iris points).
const NLandmarks = 478

// Eye corners (anchors.EXO_R, ENDO_R, EXO_L, ENDO_L). An eye centre is the
// midpoint of its two corners, the Face Mesh V2 model card's definition,
// which the worker's canonical frame uses (facegeom/frame.py). Subject's
// right is image left.
const (
	ExoR  = 33
	EndoR = 133
	ExoL  = 263
	EndoL = 362
)

// RegionEyes is the region the canonical frame is built from: with it
// occluded the worker withholds every measurement, and guidance places
// nothing.
const RegionEyes = "eyes"

// regions is anchors.REGIONS: the landmarks whose visibility each region
// reports. A landmark in no region has no visibility of its own.
var regions = map[string][]int{
	"forehead":      {10, 338, 297, 332, 284, 54, 103, 67, 109, 151},
	"contour_right": {162, 127, 234, 93, 132, 21},
	"contour_left":  {389, 356, 454, 323, 361, 251},
	"jaw":           {58, 172, 136, 150, 149, 176, 148, 152, 377, 400, 378, 379, 365, 397, 288},
	"brows":         {70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285, 9, 8},
	"eyes":          {33, 7, 163, 144, 145, 153, 154, 155, 133, 173, 157, 158, 159, 160, 161, 246, 263, 249, 390, 373, 374, 380, 381, 382, 362, 398, 384, 385, 386, 387, 388, 466},
	"nose":          {168, 6, 1, 2, 98, 327},
	"lips":          {61, 185, 40, 39, 37, 0, 267, 269, 270, 409, 291, 146, 91, 181, 84, 17, 314, 405, 321, 375, 13, 14},
}

var regionsByLandmark = func() map[int][]string {
	m := map[int][]string{}
	for _, name := range RegionNames() {
		for _, i := range regions[name] {
			m[i] = append(m[i], name)
		}
	}
	return m
}()

// RegionNames returns the worker's region names, sorted.
func RegionNames() []string {
	names := make([]string, 0, len(regions))
	for n := range regions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// RegionLandmarks returns a copy of the landmarks in region name (nil for
// an unknown name), in anchors.py order.
func RegionLandmarks(name string) []int {
	r, ok := regions[name]
	if !ok {
		return nil
	}
	return append([]int(nil), r...)
}

// RegionsOf returns the sorted names of the regions containing landmark i;
// empty when i belongs to none.
func RegionsOf(i int) []string {
	return append([]string(nil), regionsByLandmark[i]...)
}
