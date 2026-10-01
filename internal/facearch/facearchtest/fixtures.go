// Package facearchtest holds recorded ai-worker payloads for facearch tests.
//
// Both files were produced on 2026-09-29 by the as-built Plan 1 ai-worker
// (branch feat/face-measure, head 47bdf8f; FastAPI TestClient on
// create_app(["vision"])) with the real MediaPipe model (manifest SHA-256
// 64184e22...) on the CC0 photo
// apps/workers/face/tests/fixtures/sample_face.jpg (the face worker's copy), with landmarks=true
// and FACE_MEASURE_OCCLUSION_DELTA_E unset. Measurement and quality values
// are rounded to 4 decimals, landmark pixels to 2.
//
//   - catalogue_fa-measure-1.json: GET /api/v1/face-measure/catalogue.
//   - measure_fixture_face.json: POST /api/v1/face-measure for the photo.
//     The photo crops the chin, so the jaw region is occluded "(out of
//     frame)", every jaw-dependent measurement is null, and regionDeltaE
//     has no jaw entry. Bands are value-centred and conservative (Plan 1's
//     noise model). It is ONE face: a test input, never a population
//     reference.
//
// Regenerate both from the ai-worker (Task 1 of Plan 2 has the command)
// and re-run the facearch tests.
//
// Re-recorded 2026-10-01 from the face worker (worker_common create_app on
// worker_face, same model, photo and settings) after
// eye_height_to_brow_distance and nose_aspect_ratio were appended to
// fa-measure/1; every pre-existing catalogue entry and measurement was
// checked unchanged (docs/superpowers/plans/2026-10-01-face-measure-vendor-comparison.md, Task 1).
package facearchtest

import (
	_ "embed"
	"encoding/json"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

//go:embed testdata/catalogue_fa-measure-1.json
var catalogueJSON []byte

//go:embed testdata/measure_fixture_face.json
var fixtureFaceJSON []byte

// CatalogueJSON returns the recorded catalogue body.
func CatalogueJSON() []byte { return append([]byte(nil), catalogueJSON...) }

// FixtureFaceJSON returns the recorded face-measure body.
func FixtureFaceJSON() []byte { return append([]byte(nil), fixtureFaceJSON...) }

// Catalogue decodes the recorded catalogue.
func Catalogue() *measureclient.Catalogue {
	var c measureclient.Catalogue
	if err := json.Unmarshal(catalogueJSON, &c); err != nil {
		panic("facearchtest: catalogue fixture: " + err.Error())
	}
	return &c
}

// FixtureFace decodes the recorded face-measure response.
func FixtureFace() *measureclient.Response {
	var r measureclient.Response
	if err := json.Unmarshal(fixtureFaceJSON, &r); err != nil {
		panic("facearchtest: face fixture: " + err.Error())
	}
	return &r
}
