# Face Architecture (core-engine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Verified:** every code block in this plan was compiled and its tests run on 2026-09-29 — re-run after the Plan 1 contract update below, with the re-recorded payloads — in a scratch copy of `apps/core-engine` + `packages/goplatform` + `go.work` (all `internal/facearch/...`, `cmd/facearch-profile` and `internal/config` tests pass; `go vet` and `gofmt -l` clean on the new files; the full `./apps/core-engine/...` suite still passes). Not run: the two opt-in tests that need a scratch Postgres (`FACEARCH_TEST_DATABASE_URL`) or a running ai-worker (`FACEARCH_TEST_AI_WORKER_URL`). Follow the code as written.

**Goal:** Ship `POST /api/v1/vision/face-architecture/:brandId/:applicationId` in core-engine: call the ai-worker's `/api/v1/face-measure` (Plan 1), look up the brand's versioned face-architecture profile, and return a face-shape classification, Perfect Corp-compatible traits, the profile's shown measurements and provenance (including calibration status). Plus the `facearch-profile` CLI and the uncalibrated seed profile, so the endpoint works end to end.

**Architecture:** New packages under `apps/core-engine/internal/facearch/`: `measureclient` (typed HTTP client mirroring Plan 1's JSON exactly), `profile` (GORM entity for the new core-engine-owned table `face_architecture_profiles`, document types, validation, brand+app → brand+`*` → `*`+`*` lookup), `classify` (`Classifier` interface, `PrototypeClassifier`), `traits` (ordered rule lists), `facearchtest` (recorded ai-worker payloads for tests), and `facearch` itself (`Interpret` + the Echo handler). `cmd/facearch-profile` validates and upserts profile JSON. Guidance polygons (Plan 3) and the benchmark (Plan 4) are out of scope; the document carries `guidance` opaquely and responses return `"guidance": null`.

**Tech Stack:** Go 1.25 (`go.work` at the repo root), Echo v4, GORM + Postgres, testify, `net/http/httptest`, `embed`.

**Spec:** `docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md` (the copy on branch `feat/face-measure` is newer than `main`'s; read that one) — sections "Interpretation layer (core-engine)", "Seed profile", "Open literals", "Testing", "Build order" step 2, "Lanes". This is Plan 2 of 4. It consumes Plan 1's API without changing it. The authority for that API is the as-built code on branch `feat/face-measure` (`apps/ai-worker/ai_worker/vision/schemas/face_measure.py`, `routers/face_measure.py`, `docs/FACE-MEASURE.md`), not the text of `docs/superpowers/plans/2026-09-29-face-measure.md`, which changed in its final review.

## Changes after Plan 1 review

Plan 1's final fix wave (`.superpowers/sdd/2026-09-29-face-measure/final-fix-wave.md` on `feat/face-measure`) changed the contract. This plan was updated to the as-built code:

1. **Catalogue `shape`.** Every catalogue entry now carries `shape` (`"scalar"` | `"list"`). The hardcoded per-version list-valued-key table (`measureclient/shape.go` `listValuedKeys`) is gone: `Catalogue.Shape` reads the entry's `shape`, and an entry without a recognised shape is refused (`ErrShapeUnknown`). `CatalogueEntry.Shape` is a required field in its as-built position, no longer `omitempty`.
2. **Noise bands are value-centred and conservative.** A band is `[value + (P5 − P50), value + (P95 − P50)]`, so it always contains the value; it models independent noise at the model card's worst regional error, so it is deliberately wide and `boundaryUncertain` will be common. A band is `null` when bands are disabled or fewer than 90% of perturbed samples were finite: traits treat a null band as "no uncertainty information" (never uncertain, never a crash). New tests pin both (`TestRecordedBandsContainTheirValue`, `TestObserve_NullBandIsUsableWithoutBand`, `TestNullBandMeansNoUncertaintyInformation`).
3. **Null values.** A value can be null with visibility `unknown` and reason `degenerate geometry` (→ `StateNoValue`, not usable). When the `eyes` region is occluded every supported measurement is withheld (`occluded`, reason `canonical frame requires region eyes`); new test `TestInterpret_EyesOccludedWithholdsEverything`.
4. **Quality block.** `regionDeltaE` omits out-of-frame regions (the fixture now has no `jaw` entry). `warnings` may include `perspective_not_corrected` (always), `visibility_detector_unconfigured`, `visibility_reference_unavailable`, `face_partly_out_of_frame`; core-engine passes them through untouched.
5. **Alar anchors are `ALAR_BASE_*` proxies.** `alar_width_to_intercanthal_ratio` and `alar_width_to_mouth_ratio` are now `proxy: true` (keys unchanged). The seed's noseWidth and lipShape Wide/Narrow source notes say so. Other `proxy` flags changed too (`jaw_contour_angles`, `chin_taper`, `oval_fit_residual`, `eye_to_face_width_ratio` are now `proxy: true`), and the `asymmetry_*` landmark lists now include the alignment points.
6. **Recorded payloads re-recorded** from the as-built worker (TestClient on `create_app(["vision"])`, real model, `landmarks=true`, `FACE_MEASURE_OCCLUSION_DELTA_E` unset). Values and bands changed; the seed-table fixture readings and every test expectation that depends on them were re-checked (all expected labels are unchanged; 8 of 9 assessable traits still straddle). The measure fixture now carries the 478 landmarks.
7. **Go structs re-checked field by field** against the Pydantic models: `Measurement`, `Model`, `Quality`, `Response` unchanged; `CatalogueEntry` gains `shape`; `band` stays `*[2]Value` (`[lo, hi]`, each a number or a list, or the whole band `null`).

## Global Constraints

- **Lanes.** Before Task 1, add one row to `docs/LANES.md` → Active claims: lane `core-facearch` owning `apps/core-engine/internal/facearch/` and `apps/core-engine/cmd/facearch-profile/`; plus, outside that lane, `apps/core-engine/cmd/server/main.go` (the import block, one AutoMigrate entry, the handler construction and one route line — nothing else), `apps/core-engine/internal/config/config.go` + new `apps/core-engine/internal/config/config_facemeasure_test.go` (the `FaceMeasureTimeout` setting only), and `docs/FACE-ARCHITECTURE.md` (new). `cmd/face-benchmark/` belongs to the same lane in the spec but to Plan 4: do not create it. Remove your row in Task 12. Never edit another session's row. Do not `git add` `docs/LANES.md`.
- **Plan 1's contract is fixed.** `measureclient` mirrors the as-built `ai_worker/vision/schemas/face_measure.py` (branch `feat/face-measure`) field for field, in the same order, with the same nullability. Never add a field the worker does not send. Worker error bodies sit under FastAPI's `detail`.
- **No hardcoded policy.** The only new tunable is `FACE_MEASURE_TIMEOUT` (config, flagged under Open literals). Every classifier and trait number lives in profile JSON; the seed's numbers live only in the seed file and its table below. Whether a measurement is a number or a list is read from the catalogue's `shape`; core-engine keeps no table of its own.
- **Placeholders announce themselves.** Responses always carry `provenance.calibration.status`; the seed is `uncalibrated`; every seed number without a source is cited as `placeholder pending benchmark` in `calibration.sources`; `guidance` is JSON `null` until Plan 3; a classifier or trait that cannot run says `unavailable` / `not_assessable` / `insufficient_evidence` with a reason, never a guessed label.
- **Shared database.** `face_architecture_profiles` is new and core-engine-owned; it is added to core-engine's `AutoMigrate` list only. Touch no other table. Index name `uq_facearch_profile_version` is prefixed so it cannot collide in the shared schema (`docs/SHARED-DATABASE.md`).
- **Do not modify** the ai-worker, Plan 1's files, or any core-engine package other than those named above. No Go imports from Seagull-gateway or Seagull-web.
- **Tests** run from the repo root: `go test ./apps/core-engine/internal/facearch/<pkg>/ -v`. Whole feature: `go test ./apps/core-engine/internal/facearch/... ./apps/core-engine/cmd/facearch-profile/ ./apps/core-engine/internal/config/`.
- **Commits:** one per task as written. Never push.

## Decisions this plan makes where the spec is silent

Each is documented in code where it lives.

1. **List-valued measurements.** Profiles may reference only scalar measurements in prototypes, `requires` and trait conditions; validation rejects keys whose catalogue entry has `shape: "list"` (in `fa-measure/1`: `fifths`, `jaw_contour_angles`). `measurementsShown` may list list-valued keys. An entry without a recognised `shape` is refused, not guessed.
2. **"Observed" in the classifier formula means "has a float value"** (visibility `observed` or `unknown`). v1 reports every feature region `unknown`, and every region `unknown` while `FACE_MEASURE_OCCLUSION_DELTA_E` is unset; reading "observed" literally would make every classifier `insufficient_evidence` forever. A null value is never usable, whatever its visibility (`occluded`, the eyes-occluded withholding, or `unknown` with `degenerate geometry`).
3. **Observed weight share** = observed prototype weight / total prototype weight, summed over every class's prototype terms.
4. **A class with no usable prototype term is `not_assessable`** even if its `requires` are met: a score over nothing is exp(0) = 1, a fabricated certainty.
5. **Scores are normalised in log space** (identical result, no 0/0 for far-off faces). `scores` is returned for every status, including `insufficient_evidence`.
6. **Status vocabulary.** Classifier: `single`, `blend`, `insufficient_evidence`, `unavailable`. Trait: `assessed`, `not_assessable`, `unavailable`.
7. **Trait not_assessable** when any condition in any rule references an unusable measurement (the default label is only meaningful if every earlier rule could be tested). **Boundary:** a condition straddles when `lo <= threshold <= hi`; conditions are tried in rule order, and the first whose flip changes the label sets `boundaryUncertain` and `alternative`. Bands are value-centred and deliberately conservative (independent noise at the model card's worst regional error), so `boundaryUncertain` is expected often; it is kept because it is honest, not tuned down here. A `null` band means no uncertainty information: that condition never straddles.
8. **Load-time drift** is judged per request from the measure response (which lists every catalogue key with its visibility), so no per-request catalogue fetch and no catalogue cache TTL literal: version mismatch → every classifier and trait `unavailable`; a referenced key missing, `not_supported` or list-valued → that classifier or trait `unavailable`.
9. **Error bodies.** Every face-architecture error is `{"detail": {"code": ...}}`, the same shape as the ai-worker's, so clients read `detail.code` whether core-engine or the worker produced it. Worker 400 (unreadable image) is passed through like 422 and 503. Any other worker failure (connection, timeout, 5xx other than 503, bad JSON) is 503 `face_measure_unreachable`. (FastAPI's own request-validation 422 carries `detail` as a list, not an object; core-engine always sends the `image` field, so it should not occur, and if it does it is passed through verbatim like any worker 422.)
10. **The profile is looked up before the ai-worker is called**, so a scope without a profile (409) costs no inference.
11. **One active profile per brand+app scope.** Lookup is by scope, not by code, so `upsert` deactivates every other active profile in the same scope (any code). Versions number per (brand, app, code).
12. **The route** is added inside `mountVisionRoutes`, like `/config/:brandId/:applicationId`, so it is also served under `/core/vision-engine/...` and `/api/vision/...`.
13. **Source coverage.** An uncalibrated document must cite a source for each classifier's `blendMargin`, `minObservedWeight` and every class, and for every trait. A source covers path P when its `for` is P or a path under P (so a vocabulary-only source on `traits` covers nothing). Sources gain an optional `note`.
14. **Added response fields** (beyond the spec example, all additive): `measurementsMissing`, per-classifier `missingMeasurements`, `observedWeightShare`, `reason`; per-trait `status`, `missingMeasurements`, `reason`; `provenance.profile.catalogueVersion`.
15. **Deferred to Plan 3:** guidance template-index validation (0-477), template resolution, and the seed's contour / highlight / blush / brow templates and rules. This plan stores `guidance` untouched and never reads it.

## Open literals (confirm with the user before Tasks 9 and 11)

AGENTS.md: a literal with no validated source is flagged before it is written.

| Value | Where | Proposed | Why no better source exists | What would replace it |
|---|---|---|---|---|
| `FACE_MEASURE_TIMEOUT` default | `internal/config/config.go` | 20 s | No face-measure latency has been measured (Plan 1 is built on `feat/face-measure` but not benchmarked). Same bound as `VISION_DISPATCH_TIMEOUT`, the other synchronous ai-worker model call. | p99 latency from the Plan 4 benchmark run. Already overridable by env. |
| 126 seed numbers | `cmd/facearch-profile/seed/seagull-default-uncalibrated.json` | table below | No validated face-shape or trait thresholds exist for MediaPipe landmarks (spec Research basis). | Plan 4's `calibrate` writes a calibrated profile. |

## Seed profile literals

`seagull-default-uncalibrated`, brand `*`, app `*`. **Every number below is uncalibrated.** "Sourced" means a public heuristic states the number itself; "ours" means a qualitative public statement translated to a number by this plan; "placeholder" means no source at all (cited in the file as `placeholder pending benchmark`). The fixture column is the CC0 photo `apps/ai-worker/tests/tryon/fixtures/sample_face.jpg` measured with Plan 1's code and the real model: **one face**, shown for scale only and used for **no** centre or threshold. Its chin is out of frame, so jaw-dependent values are withheld.

### Classifier `faceShape` (98 numbers)

| # | Literal | Value | Status | Source |
|---|---|---|---|---|
| 1 | `blendMargin` | 0.6 | sourced | Zafar & Popa 2016 (top-two blend, 60/40) |
| 2 | `minObservedWeight` | 0.5 | placeholder (policy) | spec example value |

Centres (32 numbers). Columns are `width_length_ratio` (W), `upper_to_mid_width_ratio` (U), `jaw_to_mid_width_ratio` (J), `chin_taper` (C). Fixture reading: U 0.8822 (band 0.8401–0.9215); W, J, C withheld (jaw out of frame).

| Class | W | U | J | C | `requires` | Status |
|---|---|---|---|---|---|---|
| Triangle | 0.70 | 0.80 | 1.00 | 0.60 | U, J | ours (jaw widest, forehead narrowest); W neutral |
| Diamond | 0.70 | 0.80 | 0.75 | 0.40 | U, J | ours (cheekbones widest, pointed chin); W neutral |
| Heart | 0.70 | 1.00 | 0.75 | 0.35 | U, J | ours (forehead widest, pointed chin); W neutral |
| InvTriangle | 0.70 | 1.00 | 0.80 | 0.50 | U, J | ours (forehead widest, less pointed chin); W neutral |
| Oblong | 0.60 | 1.00 | 1.00 | 0.50 | W | ours ("longer than 1.5x"; widths similar) |
| Oval | **0.67** | 0.90 | 0.85 | 0.45 | W | W **sourced** (length ≈ 1.5 × width); rest ours |
| Round | **1.00** | 0.90 | 0.90 | 0.55 | W | W **sourced** (length ≈ width); rest ours |
| Square | **1.00** | 1.00 | 1.00 | 0.60 | W, J | W **sourced** (length ≈ width); rest ours |

Known bias, stated in the seed's Oval source note: the heuristics measure hairline to chin, `face_length_mesh` stops at mesh point 10 below the hairline, so our W reads higher than theirs and the seed leans towards Round and Square until calibrated.

Spreads (32 numbers) and weights (32 numbers), identical in every class, all **placeholder**: spread W 0.12, U 0.10, J 0.10, C 0.15 (deliberately wide: about half the gap between neighbouring centres); weight 1.0 for every term (no source prefers any measurement).

### Traits (28 numbers)

| # | Trait → label | Condition | Status | Source | Fixture reading (band) |
|---|---|---|---|---|---|
| 1 | eyeSize → Big | `eye_to_face_width_ratio > 0.22` | centre 0.20 sourced, ±0.02 placeholder | rule of fifths | 0.2094 (0.1878–0.2335) |
| 2 | eyeSize → Small | `eye_to_face_width_ratio < 0.18` | as above | rule of fifths | |
| 3 | eyeAngle → Downturned | `canthal_tilt_deg < -3` | centre 0° sourced, ±3° placeholder | consumer definition (outer corner below inner) | 1.3963 (−5.7334–7.4424) |
| 4 | eyeAngle → Upturned | `canthal_tilt_deg > 3` | as above | as above | |
| 5 | eyeDistance → Wide-Set | `intercanthal_ratio > 1.1` | centre 1.0 sourced, ±0.1 placeholder | Farkas et al. 1985 canon; "one eye-width apart" | 1.1374 (0.9301–1.4653) |
| 6 | eyeDistance → Close-set | `intercanthal_ratio < 0.9` | as above | as above | |
| 7 | eyeShape → Round | `eye_aperture_ratio > 0.33` | placeholder (order from heuristics) | none | 0.2544 (0.1543–0.3542) |
| 8 | eyeShape → Narrow | `eye_aperture_ratio < 0.22` | placeholder | none | |
| 9 | eyebrowShape → Straight | `brow_arch_ratio < 0.08` | placeholder | none (qualitative definitions only) | 0.1486 (0.0913–0.2166) |
| 10 | eyebrowShape → Hard Angled | `brow_tail_tilt_deg > 35` | placeholder | none | 32.2378 (20.0762–50.9508) |
| 11 | eyebrowShape → Soft Angled | `brow_tail_tilt_deg > 20` | placeholder | none | |
| 12 | eyebrowDistance → Far-Apart | `interbrow_ratio > 1.1` | centre 1.0 sourced, ±0.1 placeholder | brow-mapping rule (brow starts above inner corner) | 0.9296 (0.7348–1.1452) |
| 13 | eyebrowDistance → Close | `interbrow_ratio < 0.9` | as above | as above | |
| 14 | eyebrowShortness → Short | `brow_length_to_eye_ratio < 1.0` | ours (derived from a qualitative rule) | brow-mapping rule (inner corner to nostril–outer-corner line) | 1.5442 (1.3667–1.7792) |
| 15 | lipShape → Downturned | `mouth_corner_tilt_deg > 5` | placeholder | none | 3.7334 (−1.9508–9.1365) |
| 16 | lipShape → Heavy Upper Lip | `upper_lower_lip_ratio > 1.0` | ours (from the words) | none | 0.6478 (0.3782–1.06) |
| 17 | lipShape → Heavy Lower Lip | `upper_lower_lip_ratio < 0.5` | placeholder | none | |
| 18 | lipShape → Round (a) | `lip_fullness_ratio > 0.45` | placeholder | none | 0.4237 (0.326–0.5429) |
| 19 | lipShape → Round (b) | `alar_width_to_mouth_ratio > 0.75` | placeholder | none | 0.629 (0.5382–0.6854) |
| 20 | lipShape → Full | `lip_fullness_ratio > 0.5` | placeholder | none | |
| 21 | lipShape → Thin | `lip_fullness_ratio < 0.3` | placeholder | none | |
| 22 | lipShape → Wide | `alar_width_to_mouth_ratio < 0.6` | centre 0.667 sourced, edge placeholder | Farkas et al. 1985 canon (mouth = 1.5 × nose width) | |
| 23 | lipShape → Narrow | `alar_width_to_mouth_ratio > 0.75` | as above | as above | |
| 24 | lipShape → Bow | `cupid_bow_depth > 0.05` | placeholder | none | 0.0317 (−0.0175–0.0941) |
| 25 | noseWidth → Broad | `alar_width_to_intercanthal_ratio > 1.1` | centre 1.0 sourced, ±0.1 placeholder | Farkas et al. 1985 canon (nasal width = intercanthal) | 1.1773 (0.9439–1.3768) |
| 26 | noseWidth → Narrow | `alar_width_to_intercanthal_ratio < 0.9` | as above | as above | |
| 27 | noseLength → Long | `nose_length_ratio > 0.53` | centre 0.5 sourced, ±0.03 placeholder | equal-thirds canon (a heuristic midpoint: met by 0% of 103 adults, Jayaratne et al. 2012) | withheld (jaw out of frame) |
| 28 | noseLength → Short | `nose_length_ratio < 0.47` | as above | as above | |

Rule order within lipShape (Downturned, Heavy Upper, Heavy Lower, Round, Full, Thin, Wide, Narrow, Bow; default Average) is itself unsourced. Defaults: Average (eyeSize, eyeAngle, eyeDistance, eyebrowDistance, lipShape, noseWidth, noseLength), Almond (eyeShape), Rounded (eyebrowShape), Normal (eyebrowShortness). Perfect Corp labels not emitted by the seed: eyebrowShape "Obscured" (hidden brows are `not_assessable` instead); every eyelid, eyebrow-thickness and cheekbone label (`not_supported` in `fa-measure/1`). On the fixture face, the fixture's noise bands straddle a threshold for 8 of the 9 assessable traits — expect `boundaryUncertain` often. Bands are value-centred (`value + P5 − P50` to `value + P95 − P50`) and conservative by construction (independent noise at the model card's worst regional error; real landmark error is correlated), so calibration moving thresholds will not by itself make the flag rare.

The alar ratios (rows 19, 22, 23, 25, 26) use `ALAR_BASE_R/L` (mesh 98/327, nostril base), a proxy for alare; the worker marks them `proxy: true`. The Farkas canon measures true alar width, so those canon centres are applied to a proxy and must be recalibrated like the placeholders.

---

### Task 1: Recorded ai-worker payloads and the measure types

**Files:**
- Create: `apps/core-engine/internal/facearch/facearchtest/testdata/catalogue_fa-measure-1.json`
- Create: `apps/core-engine/internal/facearch/facearchtest/testdata/measure_fixture_face.json`
- Create: `apps/core-engine/internal/facearch/facearchtest/fixtures.go`
- Create: `apps/core-engine/internal/facearch/measureclient/types.go`
- Create: `apps/core-engine/internal/facearch/measureclient/shape.go`
- Create: `apps/core-engine/internal/facearch/measureclient/observe.go`
- Test: `apps/core-engine/internal/facearch/measureclient/types_test.go`
- Test: `apps/core-engine/internal/facearch/measureclient/observe_test.go`

**Interfaces:**
- Consumes: Plan 1 JSON (`FaceMeasureResponse`, `CatalogueOut`).
- Produces (`measureclient`): `const MeasurePath, CataloguePath`; `const VisibilityObserved, VisibilityUnknown, VisibilityOccluded, VisibilityNotSupported`; `type Value struct{ Scalar *float64; List []float64 }` with `IsNull() bool`, `IsList() bool`, JSON (un)marshal; `type Measurement struct{ Key string; Value Value; Unit string; Band *[2]Value; Visibility string; Proxy bool; Landmarks []int; Reason *string }`; `type Model`, `type Quality`, `type Response struct{ CatalogueVersion; Model; Quality; Regions map[string]string; Measurements []Measurement; Landmarks [][]float64 }`; `type CatalogueEntry struct{ Key, Unit, Shape string; Regions []string; Landmarks []int; Proxy, Supported bool; Reason *string }`, `type Catalogue` with `Entry(key string) (CatalogueEntry, bool)` and `Shape(key string) (string, error)` (reads the entry's `shape`); `const ShapeScalar, ShapeList`; `var ErrShapeUnknown` (entry has no recognised shape); `const StateUsable, StateOccluded, StateNotSupported, StateMissing, StateNotScalar, StateNoValue`; `type Observation struct{ Key, State string; Value float64; Band *[2]float64 }` with `Usable() bool`; `type Index`; `func (r *Response) Index() Index`; `func (ix Index) Observe(key string) Observation`.
- Produces (`facearchtest`): `CatalogueJSON() []byte`, `FixtureFaceJSON() []byte`, `Catalogue() *measureclient.Catalogue`, `FixtureFace() *measureclient.Response`.

- [ ] **Step 1: Claim the lane.** Add your row to `docs/LANES.md` → Active claims exactly as described in Global Constraints (lane, session name, `[ref]`, paths, time). Do not edit other rows.

- [ ] **Step 2: Add the recorded payloads.** These are real outputs of the as-built ai-worker (branch `feat/face-measure`, head `47bdf8f`) recorded on 2026-09-29 (see the `facearchtest` package comment). Copy them byte for byte. To regenerate, from the root of a checkout of that branch with the model provisioned and `FACE_MEASURE_OCCLUSION_DELTA_E` unset, run `PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=apps/ai-worker python /tmp/record.py /tmp/recorded.json` where `/tmp/record.py` (outside the repository) is:
```python
import json
import sys

from fastapi.testclient import TestClient
from ai_worker.main import create_app

with TestClient(create_app(["vision"])) as c:
    cat = c.get("/api/v1/face-measure/catalogue").json()
    with open("apps/ai-worker/tests/tryon/fixtures/sample_face.jpg", "rb") as f:
        r = c.post("/api/v1/face-measure", files={"image": ("sample_face.jpg", f.read(), "image/jpeg")},
                   data={"landmarks": "true"})
    assert r.status_code == 200, r.text
    json.dump({"catalogue": cat, "measure": r.json()}, open(sys.argv[1], "w"))
```
then round measurement and quality floats to 4 decimals and landmark pixels to 2, one catalogue entry or measurement per line.

`apps/core-engine/internal/facearch/facearchtest/testdata/catalogue_fa-measure-1.json`:
```json
{
  "catalogueVersion": "fa-measure/1",
  "measurements": [
    {"key": "face_length_mesh", "unit": "iod", "shape": "scalar", "regions": ["forehead", "jaw"], "landmarks": [10, 152], "proxy": true, "supported": true, "reason": null},
    {"key": "face_width_oval", "unit": "iod", "shape": "scalar", "regions": ["contour_right", "contour_left"], "landmarks": [234, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "upper_width_proxy", "unit": "iod", "shape": "scalar", "regions": ["forehead"], "landmarks": [54, 284], "proxy": true, "supported": true, "reason": null},
    {"key": "jaw_width_proxy", "unit": "iod", "shape": "scalar", "regions": ["jaw"], "landmarks": [172, 397], "proxy": true, "supported": true, "reason": null},
    {"key": "width_length_ratio", "unit": "ratio", "shape": "scalar", "regions": ["forehead", "jaw", "contour_right", "contour_left"], "landmarks": [10, 152, 234, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "upper_to_mid_width_ratio", "unit": "ratio", "shape": "scalar", "regions": ["forehead", "contour_right", "contour_left"], "landmarks": [54, 284, 234, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "jaw_to_mid_width_ratio", "unit": "ratio", "shape": "scalar", "regions": ["jaw", "contour_right", "contour_left"], "landmarks": [172, 397, 234, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "jaw_contour_angles", "unit": "deg", "shape": "list", "regions": ["jaw"], "landmarks": [172, 136, 150, 149, 176, 148, 152, 377, 400, 378, 379, 365, 397], "proxy": true, "supported": true, "reason": null},
    {"key": "jaw_taper_angle_deg", "unit": "deg", "shape": "scalar", "regions": ["jaw"], "landmarks": [172, 152, 397], "proxy": true, "supported": true, "reason": null},
    {"key": "chin_taper", "unit": "ratio", "shape": "scalar", "regions": ["jaw"], "landmarks": [176, 400, 172, 397], "proxy": true, "supported": true, "reason": null},
    {"key": "oval_fit_residual", "unit": "ratio", "shape": "scalar", "regions": ["forehead", "jaw", "contour_right", "contour_left"], "landmarks": [10, 338, 297, 332, 284, 251, 389, 356, 454, 323, 361, 288, 397, 365, 379, 378, 400, 377, 152, 148, 176, 149, 150, 136, 172, 58, 132, 93, 234, 127, 162, 21, 54, 103, 67, 109], "proxy": true, "supported": true, "reason": null},
    {"key": "thirds_upper_proxy", "unit": "iod", "shape": "scalar", "regions": ["forehead", "brows"], "landmarks": [10, 9, 8], "proxy": true, "supported": true, "reason": null},
    {"key": "thirds_mid", "unit": "iod", "shape": "scalar", "regions": ["brows", "nose"], "landmarks": [9, 8, 2], "proxy": false, "supported": true, "reason": null},
    {"key": "thirds_lower", "unit": "iod", "shape": "scalar", "regions": ["nose", "jaw"], "landmarks": [2, 152], "proxy": false, "supported": true, "reason": null},
    {"key": "thirds_lower_to_mid_ratio", "unit": "ratio", "shape": "scalar", "regions": ["brows", "nose", "jaw"], "landmarks": [9, 8, 2, 152], "proxy": false, "supported": true, "reason": null},
    {"key": "lower_face_split", "unit": "ratio", "shape": "scalar", "regions": ["nose", "lips", "jaw"], "landmarks": [2, 13, 14, 152], "proxy": false, "supported": true, "reason": null},
    {"key": "fifths", "unit": "ratio", "shape": "list", "regions": ["contour_right", "contour_left", "eyes"], "landmarks": [234, 33, 133, 362, 263, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "eye_width", "unit": "iod", "shape": "scalar", "regions": ["eyes"], "landmarks": [33, 133, 263, 362], "proxy": false, "supported": true, "reason": null},
    {"key": "eye_aperture_ratio", "unit": "ratio", "shape": "scalar", "regions": ["eyes"], "landmarks": [33, 133, 263, 362, 159, 145, 386, 374], "proxy": false, "supported": true, "reason": null},
    {"key": "canthal_tilt_deg", "unit": "deg", "shape": "scalar", "regions": ["eyes"], "landmarks": [33, 133, 263, 362], "proxy": false, "supported": true, "reason": null},
    {"key": "intercanthal_ratio", "unit": "ratio", "shape": "scalar", "regions": ["eyes"], "landmarks": [33, 133, 263, 362], "proxy": false, "supported": true, "reason": null},
    {"key": "eye_to_face_width_ratio", "unit": "ratio", "shape": "scalar", "regions": ["eyes", "contour_right", "contour_left"], "landmarks": [33, 133, 263, 362, 234, 454], "proxy": true, "supported": true, "reason": null},
    {"key": "brow_length", "unit": "iod", "shape": "scalar", "regions": ["brows"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_arch_ratio", "unit": "ratio", "shape": "scalar", "regions": ["brows"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_peak_position", "unit": "ratio", "shape": "scalar", "regions": ["brows"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_tail_tilt_deg", "unit": "deg", "shape": "scalar", "regions": ["brows"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_band_height", "unit": "iod", "shape": "scalar", "regions": ["brows"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "proxy": true, "supported": true, "reason": null},
    {"key": "interbrow_ratio", "unit": "ratio", "shape": "scalar", "regions": ["brows", "eyes"], "landmarks": [107, 336, 133, 362], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_length_to_eye_ratio", "unit": "ratio", "shape": "scalar", "regions": ["brows", "eyes"], "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285, 33, 133, 263, 362], "proxy": false, "supported": true, "reason": null},
    {"key": "brow_eye_gap", "unit": "iod", "shape": "scalar", "regions": ["brows", "eyes"], "landmarks": [159, 386, 52, 282], "proxy": false, "supported": true, "reason": null},
    {"key": "alar_width_to_intercanthal_ratio", "unit": "ratio", "shape": "scalar", "regions": ["nose", "eyes"], "landmarks": [98, 327, 133, 362], "proxy": true, "supported": true, "reason": null},
    {"key": "alar_width_to_mouth_ratio", "unit": "ratio", "shape": "scalar", "regions": ["nose", "lips"], "landmarks": [98, 327, 61, 291], "proxy": true, "supported": true, "reason": null},
    {"key": "nose_length_ratio", "unit": "ratio", "shape": "scalar", "regions": ["brows", "nose", "jaw"], "landmarks": [9, 8, 2, 152], "proxy": false, "supported": true, "reason": null},
    {"key": "mouth_width", "unit": "iod", "shape": "scalar", "regions": ["lips"], "landmarks": [61, 291], "proxy": false, "supported": true, "reason": null},
    {"key": "upper_lower_lip_ratio", "unit": "ratio", "shape": "scalar", "regions": ["lips"], "landmarks": [0, 13, 14, 17], "proxy": false, "supported": true, "reason": null},
    {"key": "lip_fullness_ratio", "unit": "ratio", "shape": "scalar", "regions": ["lips"], "landmarks": [0, 13, 14, 17, 61, 291], "proxy": false, "supported": true, "reason": null},
    {"key": "cupid_bow_depth", "unit": "iod", "shape": "scalar", "regions": ["lips"], "landmarks": [0, 37, 267], "proxy": false, "supported": true, "reason": null},
    {"key": "mouth_corner_tilt_deg", "unit": "deg", "shape": "scalar", "regions": ["lips"], "landmarks": [61, 291, 13, 14], "proxy": false, "supported": true, "reason": null},
    {"key": "cheek_widest_height", "unit": "ratio", "shape": "scalar", "regions": ["contour_right", "contour_left", "nose"], "landmarks": [162, 127, 234, 93, 132, 21, 389, 356, 454, 323, 361, 251, 2], "proxy": true, "supported": true, "reason": null},
    {"key": "asymmetry_total", "unit": "iod", "shape": "scalar", "regions": ["forehead", "contour_right", "contour_left", "jaw", "brows", "eyes", "nose", "lips"], "landmarks": [0, 1, 2, 6, 7, 8, 9, 10, 13, 14, 17, 21, 33, 37, 39, 40, 46, 52, 53, 54, 55, 58, 61, 63, 65, 66, 67, 70, 84, 91, 93, 98, 103, 105, 107, 109, 127, 132, 133, 136, 144, 145, 146, 148, 149, 150, 151, 152, 153, 154, 155, 157, 158, 159, 160, 161, 162, 163, 168, 172, 173, 176, 181, 185, 234, 246, 249, 251, 263, 267, 269, 270, 276, 282, 283, 284, 285, 288, 291, 293, 295, 296, 297, 300, 314, 321, 323, 327, 332, 334, 336, 338, 356, 361, 362, 365, 373, 374, 375, 377, 378, 379, 380, 381, 382, 384, 385, 386, 387, 388, 389, 390, 397, 398, 400, 405, 409, 454, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_eyes", "unit": "iod", "shape": "scalar", "regions": ["eyes", "nose", "lips"], "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_brows", "unit": "iod", "shape": "scalar", "regions": ["brows", "eyes", "nose", "lips"], "landmarks": [0, 1, 2, 6, 7, 8, 9, 13, 14, 17, 33, 37, 39, 40, 46, 52, 53, 55, 61, 63, 65, 66, 70, 84, 91, 98, 105, 107, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 276, 282, 283, 285, 291, 293, 295, 296, 300, 314, 321, 327, 334, 336, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_nose", "unit": "iod", "shape": "scalar", "regions": ["nose", "eyes", "lips"], "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_lips", "unit": "iod", "shape": "scalar", "regions": ["lips", "eyes", "nose"], "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_contour", "unit": "iod", "shape": "scalar", "regions": ["contour_right", "contour_left", "eyes", "nose", "lips"], "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 21, 33, 37, 39, 40, 61, 84, 91, 93, 98, 127, 132, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 162, 163, 168, 173, 181, 185, 234, 246, 249, 251, 263, 267, 269, 270, 291, 314, 321, 323, 327, 356, 361, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 389, 390, 398, 405, 409, 454, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "asymmetry_jaw", "unit": "iod", "shape": "scalar", "regions": ["jaw", "eyes", "nose", "lips"], "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 58, 61, 84, 91, 98, 133, 136, 144, 145, 146, 148, 149, 150, 152, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 172, 173, 176, 181, 185, 246, 249, 263, 267, 269, 270, 288, 291, 314, 321, 327, 362, 365, 373, 374, 375, 377, 378, 379, 380, 381, 382, 384, 385, 386, 387, 388, 390, 397, 398, 400, 405, 409, 466], "proxy": false, "supported": true, "reason": null},
    {"key": "eyelid_type", "unit": "ratio", "shape": "scalar", "regions": ["eyes"], "landmarks": [], "proxy": false, "supported": false, "reason": "the eyelid crease is not in the landmark mesh; needs an image-crop classifier"},
    {"key": "brow_density", "unit": "ratio", "shape": "scalar", "regions": ["brows"], "landmarks": [], "proxy": false, "supported": false, "reason": "hair density needs pixels, not landmarks"},
    {"key": "cheekbone_prominence", "unit": "ratio", "shape": "scalar", "regions": ["contour_right", "contour_left"], "landmarks": [], "proxy": false, "supported": false, "reason": "needs depth or shading; MediaPipe z comes from synthetic training data"}
  ]
}
```

`apps/core-engine/internal/facearch/facearchtest/testdata/measure_fixture_face.json`:
```json
{
  "catalogueVersion": "fa-measure/1",
  "model": {"name": "face_landmarker", "version": "float16/1", "manifestSha256": "64184e229b263107bc2b804c6625db1341ff2bb731874b0bcc2fe6544e0bc9ff"},
  "quality": {"rollDeg": -1.2945, "yawDeg": -0.5116, "pitchDeg": -1.0657, "iodPx": 228.2242, "gatesPassed": ["roll", "yaw", "pitch", "iod_px"], "warnings": ["perspective_not_corrected", "visibility_detector_unconfigured", "face_partly_out_of_frame"], "regionDeltaE": {"forehead": 8.3556, "contour_right": 6.5918, "contour_left": 11.3366}},
  "regions": {"forehead": "unknown", "contour_right": "unknown", "contour_left": "unknown", "jaw": "occluded", "brows": "unknown", "eyes": "unknown", "nose": "unknown", "lips": "unknown"},
  "measurements": [
    {"key": "face_length_mesh", "value": null, "unit": "iod", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [10, 152], "reason": "requires region jaw (out of frame)"},
    {"key": "face_width_oval", "value": 2.2344, "unit": "iod", "band": [2.1034, 2.3789], "visibility": "unknown", "proxy": true, "landmarks": [234, 454], "reason": null},
    {"key": "upper_width_proxy", "value": 1.9712, "unit": "iod", "band": [1.8574, 2.0779], "visibility": "unknown", "proxy": true, "landmarks": [54, 284], "reason": null},
    {"key": "jaw_width_proxy", "value": null, "unit": "iod", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [172, 397], "reason": "requires region jaw (out of frame)"},
    {"key": "width_length_ratio", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [10, 152, 234, 454], "reason": "requires region jaw (out of frame)"},
    {"key": "upper_to_mid_width_ratio", "value": 0.8822, "unit": "ratio", "band": [0.8401, 0.9215], "visibility": "unknown", "proxy": true, "landmarks": [54, 284, 234, 454], "reason": null},
    {"key": "jaw_to_mid_width_ratio", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [172, 397, 234, 454], "reason": "requires region jaw (out of frame)"},
    {"key": "jaw_contour_angles", "value": null, "unit": "deg", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [172, 136, 150, 149, 176, 148, 152, 377, 400, 378, 379, 365, 397], "reason": "requires region jaw (out of frame)"},
    {"key": "jaw_taper_angle_deg", "value": null, "unit": "deg", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [172, 152, 397], "reason": "requires region jaw (out of frame)"},
    {"key": "chin_taper", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [176, 400, 172, 397], "reason": "requires region jaw (out of frame)"},
    {"key": "oval_fit_residual", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": true, "landmarks": [10, 338, 297, 332, 284, 251, 389, 356, 454, 323, 361, 288, 397, 365, 379, 378, 400, 377, 152, 148, 176, 149, 150, 136, 172, 58, 132, 93, 234, 127, 162, 21, 54, 103, 67, 109], "reason": "requires region jaw (out of frame)"},
    {"key": "thirds_upper_proxy", "value": 0.5414, "unit": "iod", "band": [0.4867, 0.6035], "visibility": "unknown", "proxy": true, "landmarks": [10, 9, 8], "reason": null},
    {"key": "thirds_mid", "value": 0.9821, "unit": "iod", "band": [0.904, 1.0637], "visibility": "unknown", "proxy": false, "landmarks": [9, 8, 2], "reason": null},
    {"key": "thirds_lower", "value": null, "unit": "iod", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [2, 152], "reason": "requires region jaw (out of frame)"},
    {"key": "thirds_lower_to_mid_ratio", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [9, 8, 2, 152], "reason": "requires region jaw (out of frame)"},
    {"key": "lower_face_split", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [2, 13, 14, 152], "reason": "requires region jaw (out of frame)"},
    {"key": "fifths", "value": [0.162, 0.2062, 0.2382, 0.2126, 0.1811], "unit": "ratio", "band": [[0.1359, 0.1746, 0.2091, 0.1822, 0.1509], [0.191, 0.2323, 0.2822, 0.2424, 0.2073]], "visibility": "unknown", "proxy": true, "landmarks": [234, 33, 133, 362, 263, 454], "reason": null},
    {"key": "eye_width", "value": 0.4679, "unit": "iod", "band": [0.3996, 0.5169], "visibility": "unknown", "proxy": false, "landmarks": [33, 133, 263, 362], "reason": null},
    {"key": "eye_aperture_ratio", "value": 0.2544, "unit": "ratio", "band": [0.1543, 0.3542], "visibility": "unknown", "proxy": false, "landmarks": [33, 133, 263, 362, 159, 145, 386, 374], "reason": null},
    {"key": "canthal_tilt_deg", "value": 1.3963, "unit": "deg", "band": [-5.7334, 7.4424], "visibility": "unknown", "proxy": false, "landmarks": [33, 133, 263, 362], "reason": null},
    {"key": "intercanthal_ratio", "value": 1.1374, "unit": "ratio", "band": [0.9301, 1.4653], "visibility": "unknown", "proxy": false, "landmarks": [33, 133, 263, 362], "reason": null},
    {"key": "eye_to_face_width_ratio", "value": 0.2094, "unit": "ratio", "band": [0.1878, 0.2335], "visibility": "unknown", "proxy": true, "landmarks": [33, 133, 263, 362, 234, 454], "reason": null},
    {"key": "brow_length", "value": 0.7226, "unit": "iod", "band": [0.6702, 0.7813], "visibility": "unknown", "proxy": false, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "reason": null},
    {"key": "brow_arch_ratio", "value": 0.1486, "unit": "ratio", "band": [0.0913, 0.2166], "visibility": "unknown", "proxy": false, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "reason": null},
    {"key": "brow_peak_position", "value": 0.6189, "unit": "ratio", "band": [0.4888, 0.7631], "visibility": "unknown", "proxy": false, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "reason": null},
    {"key": "brow_tail_tilt_deg", "value": 32.2378, "unit": "deg", "band": [20.0762, 50.9508], "visibility": "unknown", "proxy": false, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "reason": null},
    {"key": "brow_band_height", "value": 0.0925, "unit": "iod", "band": [0.0729, 0.1138], "visibility": "unknown", "proxy": true, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285], "reason": null},
    {"key": "interbrow_ratio", "value": 0.9296, "unit": "ratio", "band": [0.7348, 1.1452], "visibility": "unknown", "proxy": false, "landmarks": [107, 336, 133, 362], "reason": null},
    {"key": "brow_length_to_eye_ratio", "value": 1.5442, "unit": "ratio", "band": [1.3667, 1.7792], "visibility": "unknown", "proxy": false, "landmarks": [70, 63, 105, 66, 107, 46, 53, 52, 65, 55, 300, 293, 334, 296, 336, 276, 283, 282, 295, 285, 33, 133, 263, 362], "reason": null},
    {"key": "brow_eye_gap", "value": 0.1848, "unit": "iod", "band": [0.1363, 0.2269], "visibility": "unknown", "proxy": false, "landmarks": [159, 386, 52, 282], "reason": null},
    {"key": "alar_width_to_intercanthal_ratio", "value": 1.1773, "unit": "ratio", "band": [0.9439, 1.3768], "visibility": "unknown", "proxy": true, "landmarks": [98, 327, 133, 362], "reason": null},
    {"key": "alar_width_to_mouth_ratio", "value": 0.629, "unit": "ratio", "band": [0.5382, 0.6854], "visibility": "unknown", "proxy": true, "landmarks": [98, 327, 61, 291], "reason": null},
    {"key": "nose_length_ratio", "value": null, "unit": "ratio", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [9, 8, 2, 152], "reason": "requires region jaw (out of frame)"},
    {"key": "mouth_width", "value": 0.9962, "unit": "iod", "band": [0.9078, 1.0671], "visibility": "unknown", "proxy": false, "landmarks": [61, 291], "reason": null},
    {"key": "upper_lower_lip_ratio", "value": 0.6478, "unit": "ratio", "band": [0.3782, 1.06], "visibility": "unknown", "proxy": false, "landmarks": [0, 13, 14, 17], "reason": null},
    {"key": "lip_fullness_ratio", "value": 0.4237, "unit": "ratio", "band": [0.326, 0.5429], "visibility": "unknown", "proxy": false, "landmarks": [0, 13, 14, 17, 61, 291], "reason": null},
    {"key": "cupid_bow_depth", "value": 0.0317, "unit": "iod", "band": [-0.0175, 0.0941], "visibility": "unknown", "proxy": false, "landmarks": [0, 37, 267], "reason": null},
    {"key": "mouth_corner_tilt_deg", "value": 3.7334, "unit": "deg", "band": [-1.9508, 9.1365], "visibility": "unknown", "proxy": false, "landmarks": [61, 291, 13, 14], "reason": null},
    {"key": "cheek_widest_height", "value": 0.246, "unit": "ratio", "band": [-0.0738, 0.5818], "visibility": "unknown", "proxy": true, "landmarks": [162, 127, 234, 93, 132, 21, 389, 356, 454, 323, 361, 251, 2], "reason": null},
    {"key": "asymmetry_total", "value": null, "unit": "iod", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 8, 9, 10, 13, 14, 17, 21, 33, 37, 39, 40, 46, 52, 53, 54, 55, 58, 61, 63, 65, 66, 67, 70, 84, 91, 93, 98, 103, 105, 107, 109, 127, 132, 133, 136, 144, 145, 146, 148, 149, 150, 151, 152, 153, 154, 155, 157, 158, 159, 160, 161, 162, 163, 168, 172, 173, 176, 181, 185, 234, 246, 249, 251, 263, 267, 269, 270, 276, 282, 283, 284, 285, 288, 291, 293, 295, 296, 297, 300, 314, 321, 323, 327, 332, 334, 336, 338, 356, 361, 362, 365, 373, 374, 375, 377, 378, 379, 380, 381, 382, 384, 385, 386, 387, 388, 389, 390, 397, 398, 400, 405, 409, 454, 466], "reason": "requires region jaw (out of frame)"},
    {"key": "asymmetry_eyes", "value": 0.0075, "unit": "iod", "band": [0.002, 0.0132], "visibility": "unknown", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "reason": null},
    {"key": "asymmetry_brows", "value": 0.0132, "unit": "iod", "band": [0.0036, 0.0224], "visibility": "unknown", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 8, 9, 13, 14, 17, 33, 37, 39, 40, 46, 52, 53, 55, 61, 63, 65, 66, 70, 84, 91, 98, 105, 107, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 276, 282, 283, 285, 291, 293, 295, 296, 300, 314, 321, 327, 334, 336, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "reason": null},
    {"key": "asymmetry_nose", "value": 0.0188, "unit": "iod", "band": [0.0043, 0.0303], "visibility": "unknown", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "reason": null},
    {"key": "asymmetry_lips", "value": 0.0098, "unit": "iod", "band": [0.0022, 0.0168], "visibility": "unknown", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 61, 84, 91, 98, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 173, 181, 185, 246, 249, 263, 267, 269, 270, 291, 314, 321, 327, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 390, 398, 405, 409, 466], "reason": null},
    {"key": "asymmetry_contour", "value": 0.0183, "unit": "iod", "band": [0.0115, 0.0313], "visibility": "unknown", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 21, 33, 37, 39, 40, 61, 84, 91, 93, 98, 127, 132, 133, 144, 145, 146, 153, 154, 155, 157, 158, 159, 160, 161, 162, 163, 168, 173, 181, 185, 234, 246, 249, 251, 263, 267, 269, 270, 291, 314, 321, 323, 327, 356, 361, 362, 373, 374, 375, 380, 381, 382, 384, 385, 386, 387, 388, 389, 390, 398, 405, 409, 454, 466], "reason": null},
    {"key": "asymmetry_jaw", "value": null, "unit": "iod", "band": null, "visibility": "occluded", "proxy": false, "landmarks": [0, 1, 2, 6, 7, 13, 14, 17, 33, 37, 39, 40, 58, 61, 84, 91, 98, 133, 136, 144, 145, 146, 148, 149, 150, 152, 153, 154, 155, 157, 158, 159, 160, 161, 163, 168, 172, 173, 176, 181, 185, 246, 249, 263, 267, 269, 270, 288, 291, 314, 321, 327, 362, 365, 373, 374, 375, 377, 378, 379, 380, 381, 382, 384, 385, 386, 387, 388, 390, 397, 398, 400, 405, 409, 466], "reason": "requires region jaw (out of frame)"},
    {"key": "eyelid_type", "value": null, "unit": "ratio", "band": null, "visibility": "not_supported", "proxy": false, "landmarks": [], "reason": "the eyelid crease is not in the landmark mesh; needs an image-crop classifier"},
    {"key": "brow_density", "value": null, "unit": "ratio", "band": null, "visibility": "not_supported", "proxy": false, "landmarks": [], "reason": "hair density needs pixels, not landmarks"},
    {"key": "cheekbone_prominence", "value": null, "unit": "ratio", "band": null, "visibility": "not_supported", "proxy": false, "landmarks": [], "reason": "needs depth or shading; MediaPipe z comes from synthetic training data"}
  ],
  "landmarks": [
    [448.02, 463.61], [436.83, 374.86], [440.22, 403.91], [413.29, 302.79], [435.57, 352.01], [434.66, 322.83], [433.55, 250.62], [279.54, 245.3],
    [431.83, 194.91], [431.33, 164.91], [428.84, 56.39], [448.89, 476.12], [449.62, 490.36], [449.28, 501.47], [449.54, 502.45], [450.24, 519.53],
    [451.07, 540.74], [450.71, 560.9], [449.24, 570.09], [437.99, 388.39], [414.41, 391.57], [195.02, 168.79], [341.43, 259.92], [318.66, 262.85],
    [296.54, 264.64], [268.4, 256.79], [361.51, 255.47], [302.55, 201.96], [331.18, 202.21], [277.66, 208.45], [262.66, 218.6], [248.81, 274.81],
    [354.52, 593.27], [270.71, 241.77], [191.07, 264.39], [223.02, 262.58], [320.98, 365.4], [413.7, 457.84], [419.35, 490.82], [379.53, 471.15],
    [358.23, 485.61], [392.77, 493.92], [372.82, 499.37], [329.56, 525.74], [415.84, 375.13], [411.08, 353.24], [231.16, 207.62], [365.6, 303.1],
    [356.89, 374.32], [354.66, 362.79], [258.11, 365.64], [411.68, 325.86], [281.93, 179.96], [250.86, 190.01], [210.76, 128.77], [384.45, 192.73],
    [356.49, 210.69], [315.17, 503.31], [214.5, 473.79], [375.71, 387.03], [397.97, 394.9], [335.38, 512.23], [342.67, 510.66], [238.17, 176.77],
    [357.71, 386.86], [325.56, 179.57], [320.04, 159.7], [295.36, 73.04], [224.08, 150.81], [308.88, 114.46], [218.79, 199.22], [205.99, 183.45],
    [415.71, 474.73], [386.33, 483.14], [365.44, 492.21], [383.15, 392.66], [339.09, 511.55], [348.4, 516.78], [346.82, 509.95], [385.87, 375.73],
    [378.25, 504.06], [397.81, 501.1], [422.1, 500.69], [410.98, 568.77], [411.53, 558.95], [414.22, 538.84], [417.46, 517.71], [421.21, 501.09],
    [377.34, 503.97], [369.72, 512.93], [361.61, 524.0], [354.95, 534.7], [342.21, 455.06], [192.11, 359.8], [438.88, 397.16], [363.0, 506.78],
    [355.86, 512.16], [406.11, 404.26], [369.18, 402.09], [402.21, 400.05], [342.14, 318.45], [307.16, 336.43], [352.31, 375.19], [242.02, 96.2],
    [256.47, 127.43], [272.41, 161.58], [349.95, 541.17], [375.05, 163.52], [365.37, 111.19], [355.79, 60.91], [279.29, 262.78], [226.29, 293.13],
    [374.98, 249.74], [241.84, 231.96], [381.61, 287.42], [370.32, 363.19], [204.34, 307.22], [240.22, 307.86], [267.96, 314.44], [311.59, 307.61],
    [343.8, 295.42], [367.71, 283.82], [411.03, 256.88], [209.7, 355.55], [225.11, 232.29], [425.26, 388.0], [365.87, 326.4], [187.3, 262.0],
    [385.71, 270.49], [352.04, 377.32], [257.13, 245.06], [369.38, 349.17], [199.71, 413.95], [375.87, 241.81], [389.97, 334.73], [258.71, 535.45],
    [259.45, 559.19], [194.48, 357.77], [234.15, 501.67], [198.0, 220.51], [349.06, 617.24], [428.85, 396.92], [346.47, 346.23], [206.91, 267.9],
    [300.11, 248.29], [318.31, 248.0], [342.23, 522.57], [216.85, 401.27], [395.47, 651.23], [319.36, 615.82], [290.54, 592.47], [430.01, 109.95],
    [451.02, 654.47], [337.28, 246.04], [356.1, 243.87], [369.48, 243.24], [211.35, 229.46], [354.56, 227.41], [333.54, 221.72], [313.09, 221.49],
    [294.0, 226.13], [282.52, 232.03], [188.95, 210.94], [288.27, 247.04], [442.56, 424.92], [362.02, 438.71], [374.92, 381.55], [407.29, 426.91],
    [432.8, 222.39], [285.56, 567.99], [313.94, 592.97], [394.32, 632.84], [235.53, 522.53], [369.29, 235.95], [395.92, 293.52], [450.57, 637.78],
    [353.29, 636.39], [201.66, 407.98], [397.0, 501.93], [390.13, 514.79], [383.41, 531.67], [378.28, 549.69], [375.82, 557.65], [357.18, 504.76],
    [349.93, 502.52], [343.58, 499.36], [326.86, 477.84], [245.72, 414.59], [396.46, 270.53], [389.12, 225.4], [377.41, 227.84], [363.72, 506.92],
    [243.99, 470.39], [405.36, 225.17], [364.75, 572.99], [434.42, 298.54], [411.87, 280.95], [433.73, 275.89], [378.32, 330.12], [449.43, 614.27],
    [449.19, 589.61], [404.88, 586.13], [309.87, 530.13], [335.4, 397.18], [333.51, 554.58], [290.28, 394.85], [319.27, 422.38], [274.38, 433.58],
    [397.74, 609.76], [363.64, 344.26], [291.73, 544.28], [320.93, 571.39], [293.31, 501.08], [223.83, 440.51], [268.01, 499.45], [212.8, 456.3],
    [304.91, 456.72], [381.32, 309.36], [378.26, 371.9], [364.51, 380.77], [389.4, 356.37], [369.74, 205.51], [329.16, 193.85], [293.65, 193.59],
    [265.81, 200.85], [248.27, 213.49], [241.97, 253.55], [190.14, 310.14], [261.09, 282.65], [283.63, 287.23], [314.12, 284.12], [343.27, 276.92],
    [366.16, 268.87], [381.73, 261.53], [189.6, 310.01], [366.58, 389.13], [395.31, 313.93], [396.94, 372.09], [412.11, 385.17], [397.2, 376.41],
    [375.41, 397.66], [416.87, 387.06], [420.45, 394.64], [383.35, 242.82], [393.2, 248.29], [398.36, 252.17], [275.81, 237.06], [254.44, 230.69],
    [455.96, 301.67], [596.66, 238.45], [463.3, 390.23], [680.34, 154.36], [534.37, 255.72], [557.52, 257.78], [580.73, 258.34], [609.12, 249.19],
    [512.72, 251.9], [571.2, 195.42], [541.44, 197.19], [596.7, 200.96], [612.5, 210.54], [631.81, 265.46], [540.47, 589.61], [605.61, 234.01],
    [692.13, 251.05], [658.25, 251.89], [559.3, 359.51], [481.63, 454.94], [478.65, 488.59], [516.82, 465.5], [539.49, 479.05], [504.5, 490.35],
    [524.85, 493.89], [566.3, 521.3], [457.46, 373.84], [460.43, 352.35], [643.43, 197.73], [509.31, 299.64], [520.88, 368.94], [522.27, 357.53],
    [625.49, 356.56], [457.92, 324.2], [586.81, 172.99], [620.66, 181.63], [660.45, 115.27], [480.49, 191.6], [515.56, 206.58], [578.67, 496.28],
    [680.64, 462.61], [502.46, 382.45], [481.67, 391.66], [562.66, 506.54], [554.99, 505.33], [632.28, 167.45], [522.1, 381.11], [540.78, 175.22],
    [545.25, 154.82], [567.66, 64.84], [645.67, 138.88], [554.5, 107.3], [655.39, 188.28], [667.94, 170.53], [481.33, 472.39], [510.96, 478.53],
    [532.61, 486.77], [496.38, 388.1], [559.68, 506.0], [551.51, 514.04], [551.19, 504.97], [490.41, 371.93], [518.68, 499.12], [499.36, 497.87],
    [475.8, 499.31], [484.71, 567.05], [489.05, 558.66], [486.15, 538.7], [482.01, 516.88], [476.55, 501.62], [520.42, 502.39], [528.92, 511.19],
    [538.39, 523.01], [546.51, 533.86], [545.59, 448.86], [700.25, 348.22], [535.3, 503.52], [542.73, 509.08], [474.49, 401.79], [512.1, 397.03],
    [477.85, 397.55], [534.85, 313.11], [572.49, 329.89], [526.61, 369.55], [625.01, 84.69], [609.92, 117.32], [594.91, 154.19], [545.0, 538.26],
    [487.95, 161.75], [496.43, 107.38], [504.21, 56.17], [598.56, 255.25], [657.23, 282.84], [498.67, 247.15], [634.99, 222.78], [490.8, 284.87],
    [504.68, 359.19], [680.99, 295.31], [642.74, 298.08], [613.46, 306.29], [567.81, 301.41], [533.3, 291.06], [506.98, 280.51], [456.78, 255.63],
    [677.5, 344.63], [652.19, 221.94], [450.2, 386.98], [510.28, 322.53], [697.17, 248.07], [487.33, 268.3], [527.96, 371.68], [619.86, 236.92],
    [505.48, 345.03], [694.91, 402.73], [497.3, 239.26], [481.79, 332.22], [635.33, 526.84], [636.58, 550.28], [696.79, 345.91], [659.2, 491.43],
    [680.64, 207.45], [548.41, 612.94], [448.93, 395.91], [532.0, 341.1], [675.6, 255.69], [575.08, 241.78], [557.07, 242.38], [558.84, 518.82],
    [672.48, 390.43], [504.93, 648.79], [578.77, 610.66], [606.76, 585.42], [537.01, 241.63], [517.83, 240.42], [504.48, 240.01], [667.14, 217.84],
    [519.76, 223.42], [541.45, 216.66], [562.74, 215.68], [581.32, 219.31], [593.36, 225.05], [691.1, 196.04], [587.41, 240.51], [523.87, 433.37],
    [502.39, 377.04], [477.58, 424.17], [609.23, 560.21], [581.91, 587.3], [503.87, 630.86], [659.99, 512.44], [504.56, 232.65], [474.93, 291.85],
    [545.76, 632.62], [691.36, 397.08], [500.99, 501.13], [508.53, 514.75], [516.93, 532.02], [522.86, 549.55], [519.35, 555.84], [541.04, 499.81],
    [548.05, 496.59], [554.69, 493.24], [564.21, 471.63], [641.75, 405.18], [473.37, 268.69], [480.69, 223.66], [494.43, 225.24], [533.32, 501.8],
    [646.58, 461.02], [460.75, 224.49], [529.49, 569.92], [456.89, 279.92], [495.92, 326.47], [490.63, 585.07], [584.53, 524.28], [546.92, 391.17],
    [561.05, 549.41], [593.58, 387.63], [566.2, 415.83], [612.6, 425.14], [498.73, 607.98], [512.93, 339.73], [601.48, 537.2], [574.09, 565.56],
    [598.9, 493.1], [666.76, 429.89], [623.86, 491.09], [680.74, 445.5], [583.25, 449.77], [492.18, 306.43], [496.78, 367.85], [512.96, 375.99],
    [483.58, 353.51], [499.88, 203.19], [541.43, 188.96], [578.12, 187.14], [607.1, 193.09], [626.2, 204.59], [636.8, 244.4], [697.02, 297.75],
    [619.26, 274.55], [595.59, 279.81], [563.59, 278.39], [532.89, 272.56], [508.75, 265.69], [491.09, 259.28], [699.38, 297.22], [512.49, 383.8],
    [476.02, 311.47], [477.2, 369.35], [464.16, 383.52], [478.01, 373.85], [505.15, 392.89], [458.53, 385.53], [457.39, 394.04], [489.0, 240.43],
    [478.13, 246.71], [472.33, 251.16], [600.35, 229.87], [622.08, 222.39], [321.54, 234.11], [346.22, 232.93], [320.03, 214.34], [296.73, 235.28],
    [322.92, 254.55], [558.46, 228.73], [583.0, 228.32], [557.85, 208.64], [533.13, 228.66], [558.09, 248.73]
  ]
}
```

- [ ] **Step 3: Write the failing tests**

`apps/core-engine/internal/facearch/measureclient/types_test.go`:
```go
package measureclient_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestDecodesScalarListAndNullValues(t *testing.T) {
	resp := facearchtest.FixtureFace()
	byKey := map[string]mc.Measurement{}
	for _, m := range resp.Measurements {
		byKey[m.Key] = m
	}
	eye := byKey["eye_width"]
	require.NotNil(t, eye.Value.Scalar)
	assert.InDelta(t, 0.4679, *eye.Value.Scalar, 1e-9)
	require.NotNil(t, eye.Band)
	assert.InDelta(t, 0.3996, *eye.Band[0].Scalar, 1e-9)
	assert.Nil(t, eye.Reason)

	fifths := byKey["fifths"]
	assert.True(t, fifths.Value.IsList())
	assert.Len(t, fifths.Value.List, 5)
	assert.Len(t, fifths.Band[0].List, 5)

	jaw := byKey["jaw_width_proxy"]
	assert.True(t, jaw.Value.IsNull())
	assert.Nil(t, jaw.Band)
	require.NotNil(t, jaw.Reason)
	assert.Equal(t, "requires region jaw (out of frame)", *jaw.Reason)

	assert.True(t, byKey["alar_width_to_intercanthal_ratio"].Proxy, "alar anchors are nostril-base proxies")
	assert.Len(t, resp.Landmarks, 478)
	assert.Len(t, resp.Landmarks[0], 2)
	_, hasJaw := resp.Quality.RegionDeltaE["jaw"]
	assert.False(t, hasJaw, "regionDeltaE omits out-of-frame regions")
}

func TestMeasurement_RoundTripsJSON(t *testing.T) {
	var outer struct {
		Measurements []json.RawMessage `json:"measurements"`
	}
	require.NoError(t, json.Unmarshal(facearchtest.FixtureFaceJSON(), &outer))
	for _, raw := range outer.Measurements {
		var m mc.Measurement
		require.NoError(t, json.Unmarshal(raw, &m))
		again, err := json.Marshal(m)
		require.NoError(t, err)
		assert.JSONEq(t, string(raw), string(again))
	}
}

// Every catalogue field the worker sends, and only those, survives a
// decode/encode: CatalogueEntry mirrors CatalogueEntryOut exactly.
func TestCatalogueEntry_RoundTripsJSON(t *testing.T) {
	var outer struct {
		Measurements []json.RawMessage `json:"measurements"`
	}
	require.NoError(t, json.Unmarshal(facearchtest.CatalogueJSON(), &outer))
	require.NotEmpty(t, outer.Measurements)
	for _, raw := range outer.Measurements {
		var e mc.CatalogueEntry
		require.NoError(t, json.Unmarshal(raw, &e))
		again, err := json.Marshal(e)
		require.NoError(t, err)
		assert.JSONEq(t, string(raw), string(again))
	}
}
```

`apps/core-engine/internal/facearch/measureclient/observe_test.go`:
```go
package measureclient_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestObserve_States(t *testing.T) {
	ix := facearchtest.FixtureFace().Index()

	o := ix.Observe("intercanthal_ratio")
	assert.Equal(t, mc.StateUsable, o.State)
	assert.True(t, o.Usable())
	assert.InDelta(t, 1.1374, o.Value, 1e-9)
	require.NotNil(t, o.Band)
	assert.InDelta(t, 0.9301, o.Band[0], 1e-9)
	assert.InDelta(t, 1.4653, o.Band[1], 1e-9)

	assert.Equal(t, mc.StateOccluded, ix.Observe("nose_length_ratio").State)
	assert.Equal(t, mc.StateNotSupported, ix.Observe("eyelid_type").State)
	assert.Equal(t, mc.StateNotScalar, ix.Observe("fifths").State)
	assert.Equal(t, mc.StateMissing, ix.Observe("no_such_key").State)
}

// The worker's "degenerate geometry": value null, visibility unknown.
func TestObserve_NullVisibleValueIsNoValue(t *testing.T) {
	reason := "degenerate geometry"
	r := &mc.Response{Measurements: []mc.Measurement{{Key: "k", Visibility: mc.VisibilityUnknown, Reason: &reason}}}
	o := r.Index().Observe("k")
	assert.Equal(t, mc.StateNoValue, o.State)
	assert.False(t, o.Usable())
}

// A band is null when bands are disabled or too few perturbed samples were
// finite. The value is still usable; there is just no band.
func TestObserve_NullBandIsUsableWithoutBand(t *testing.T) {
	v := 0.5
	r := &mc.Response{Measurements: []mc.Measurement{{Key: "k", Value: mc.Value{Scalar: &v}, Visibility: mc.VisibilityUnknown}}}
	o := r.Index().Observe("k")
	assert.True(t, o.Usable())
	assert.Nil(t, o.Band)
}

func TestCatalogueShape(t *testing.T) {
	cat := facearchtest.Catalogue()
	s, err := cat.Shape("fifths")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeList, s)
	s, err = cat.Shape("jaw_contour_angles")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeList, s)
	s, err = cat.Shape("width_length_ratio")
	require.NoError(t, err)
	assert.Equal(t, mc.ShapeScalar, s)

	_, err = cat.Shape("no_such_key")
	assert.Error(t, err)

	other := &mc.Catalogue{CatalogueVersion: "fa-measure/2", Measurements: []mc.CatalogueEntry{{Key: "a"}, {Key: "b", Shape: "matrix"}}}
	_, err = other.Shape("a")
	assert.ErrorIs(t, err, mc.ErrShapeUnknown, "no shape is refused, not guessed")
	_, err = other.Shape("b")
	assert.ErrorIs(t, err, mc.ErrShapeUnknown, "an unrecognised shape is refused")
}

// Every non-null recorded value is a list exactly when the catalogue says so.
func TestCatalogueShapeMatchesRecordedValues(t *testing.T) {
	cat := facearchtest.Catalogue()
	for _, m := range facearchtest.FixtureFace().Measurements {
		if m.Value.IsNull() {
			continue
		}
		s, err := cat.Shape(m.Key)
		require.NoError(t, err, m.Key)
		assert.Equal(t, m.Value.IsList(), s == mc.ShapeList, m.Key)
	}
}

// Plan 1's bands are value-centred (value + P5-P50, value + P95-P50), so
// every recorded band contains its value, element by element for lists.
func TestRecordedBandsContainTheirValue(t *testing.T) {
	for _, m := range facearchtest.FixtureFace().Measurements {
		if m.Band == nil {
			continue
		}
		lo, hi := m.Band[0], m.Band[1]
		if m.Value.IsList() {
			require.Len(t, lo.List, len(m.Value.List), m.Key)
			require.Len(t, hi.List, len(m.Value.List), m.Key)
			for i, v := range m.Value.List {
				assert.LessOrEqual(t, lo.List[i], v, m.Key)
				assert.LessOrEqual(t, v, hi.List[i], m.Key)
			}
			continue
		}
		require.NotNil(t, m.Value.Scalar, m.Key)
		assert.LessOrEqual(t, *lo.Scalar, *m.Value.Scalar, m.Key)
		assert.LessOrEqual(t, *m.Value.Scalar, *hi.Scalar, m.Key)
	}
}
```

- [ ] **Step 4: Run; expect FAIL** (build failure: the `facearchtest` and `measureclient` packages do not exist yet).

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 5: Implement the fixture loader**

`apps/core-engine/internal/facearch/facearchtest/fixtures.go`:
```go
// Package facearchtest holds recorded ai-worker payloads for facearch tests.
//
// Both files were produced on 2026-09-29 by the as-built Plan 1 ai-worker
// (branch feat/face-measure, head 47bdf8f; FastAPI TestClient on
// create_app(["vision"])) with the real MediaPipe model (manifest SHA-256
// 64184e22...) on the CC0 photo
// apps/ai-worker/tests/tryon/fixtures/sample_face.jpg, with landmarks=true
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
```

- [ ] **Step 6: Implement the types, the shape lookup and observations**

`apps/core-engine/internal/facearch/measureclient/types.go`:
```go
// Package measureclient calls the ai-worker's face-measure endpoints
// (vision profile) and mirrors their JSON. The contract is the ai-worker's
// ai_worker/vision/schemas/face_measure.py and routers/face_measure.py
// (docs/FACE-MEASURE.md). Field names, order and nullability here follow
// those Pydantic models exactly; do not add fields the worker does not send.
package measureclient

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Paths on the ai-worker (vision profile).
const (
	MeasurePath   = "/api/v1/face-measure"
	CataloguePath = "/api/v1/face-measure/catalogue"
)

// Visibility values a measurement can carry (Plan 1, pipeline.MeasurementResult).
const (
	VisibilityObserved     = "observed"
	VisibilityUnknown      = "unknown"
	VisibilityOccluded     = "occluded"
	VisibilityNotSupported = "not_supported"
)

// Value is a measurement value: a float, a list of floats, or null.
// The zero Value is null.
type Value struct {
	Scalar *float64
	List   []float64
}

// IsNull reports whether the value is JSON null.
func (v Value) IsNull() bool { return v.Scalar == nil && v.List == nil }

// IsList reports whether the value is a JSON array.
func (v Value) IsList() bool { return v.List != nil }

func (v Value) MarshalJSON() ([]byte, error) {
	switch {
	case v.List != nil:
		return json.Marshal(v.List)
	case v.Scalar != nil:
		return json.Marshal(*v.Scalar)
	default:
		return []byte("null"), nil
	}
}

func (v *Value) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*v = Value{}
	switch {
	case bytes.Equal(b, []byte("null")):
		return nil
	case len(b) > 0 && b[0] == '[':
		list := []float64{}
		if err := json.Unmarshal(b, &list); err != nil {
			return fmt.Errorf("measurement value list: %w", err)
		}
		v.List = list
		return nil
	default:
		var f float64
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("measurement value: %w", err)
		}
		v.Scalar = &f
		return nil
	}
}

// Measurement mirrors MeasurementOut. Value is null when the measurement is
// occluded or out of frame (visibility "occluded"; every measurement when
// the eyes are occluded, reason "canonical frame requires region eyes"),
// not supported, or undefined for this face (visibility "unknown", reason
// "degenerate geometry"). Band is [lo, hi], each a number or a list like
// Value, re-centred on the value so it always contains it; it is null
// whenever Value is, when bands are disabled, or when too few perturbed
// samples were finite.
type Measurement struct {
	Key        string    `json:"key"`
	Value      Value     `json:"value"`
	Unit       string    `json:"unit"`
	Band       *[2]Value `json:"band"`
	Visibility string    `json:"visibility"`
	Proxy      bool      `json:"proxy"`
	Landmarks  []int     `json:"landmarks"`
	Reason     *string   `json:"reason"`
}

// Model mirrors ModelOut.
type Model struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	ManifestSha256 string `json:"manifestSha256"`
}

// Quality mirrors QualityOut. RegionDeltaE omits regions that are out of
// frame. Warnings may include perspective_not_corrected (always),
// visibility_detector_unconfigured, visibility_reference_unavailable and
// face_partly_out_of_frame; core-engine passes them through unread.
type Quality struct {
	RollDeg      float64            `json:"rollDeg"`
	YawDeg       float64            `json:"yawDeg"`
	PitchDeg     float64            `json:"pitchDeg"`
	IodPx        float64            `json:"iodPx"`
	GatesPassed  []string           `json:"gatesPassed"`
	Warnings     []string           `json:"warnings"`
	RegionDeltaE map[string]float64 `json:"regionDeltaE"`
}

// Response mirrors FaceMeasureResponse. Landmarks is 478 x [x, y] in image
// pixels; core-engine always requests it (landmarks=true) for Plan 3's
// guidance polygons.
type Response struct {
	CatalogueVersion string            `json:"catalogueVersion"`
	Model            Model             `json:"model"`
	Quality          Quality           `json:"quality"`
	Regions          map[string]string `json:"regions"`
	Measurements     []Measurement     `json:"measurements"`
	Landmarks        [][]float64       `json:"landmarks"`
}

// CatalogueEntry mirrors CatalogueEntryOut. Shape is "scalar" (value is a
// number) or "list" (a list of numbers); see Catalogue.Shape.
type CatalogueEntry struct {
	Key       string   `json:"key"`
	Unit      string   `json:"unit"`
	Shape     string   `json:"shape"`
	Regions   []string `json:"regions"`
	Landmarks []int    `json:"landmarks"`
	Proxy     bool     `json:"proxy"`
	Supported bool     `json:"supported"`
	Reason    *string  `json:"reason"`
}

// Catalogue mirrors CatalogueOut.
type Catalogue struct {
	CatalogueVersion string           `json:"catalogueVersion"`
	Measurements     []CatalogueEntry `json:"measurements"`
}

// Entry returns the catalogue entry for key.
func (c *Catalogue) Entry(key string) (CatalogueEntry, bool) {
	for _, e := range c.Measurements {
		if e.Key == key {
			return e, true
		}
	}
	return CatalogueEntry{}, false
}
```

`apps/core-engine/internal/facearch/measureclient/shape.go`:
```go
package measureclient

import (
	"errors"
	"fmt"
)

// Shapes a measurement value can have.
const (
	ShapeScalar = "scalar"
	ShapeList   = "list"
)

// ErrShapeUnknown means the catalogue entry does not say whether the
// measurement is a number or a list.
var ErrShapeUnknown = errors.New("measurement shape unknown")

// Shape returns ShapeScalar or ShapeList for key, as the catalogue entry
// declares it. The unit cannot tell ("fifths" is a "ratio" and
// "jaw_contour_angles" a "deg", like scalar keys), so core-engine keeps no
// table of its own: an entry without a recognised shape is refused
// (ErrShapeUnknown) rather than guessed.
func (c *Catalogue) Shape(key string) (string, error) {
	e, ok := c.Entry(key)
	if !ok {
		return "", fmt.Errorf("measurement %q is not in catalogue %s", key, c.CatalogueVersion)
	}
	switch e.Shape {
	case ShapeScalar, ShapeList:
		return e.Shape, nil
	}
	return "", fmt.Errorf("%w: measurement %q in catalogue %s has shape %q", ErrShapeUnknown, key, c.CatalogueVersion, e.Shape)
}
```

`apps/core-engine/internal/facearch/measureclient/observe.go`:
```go
package measureclient

// Observation states: whether a measurement can be used by a classifier
// or trait rule.
const (
	// StateUsable: the value is a float. Visibility is "observed" or
	// "unknown"; "unknown" values are computed, only their region could not
	// be judged (in v1 feature regions are always unknown).
	StateUsable = "usable"
	// StateOccluded: a region the measurement needs is occluded or out of
	// frame (or the eyes are, which withholds every measurement).
	StateOccluded = "occluded"
	// StateNotSupported: the catalogue lists the key but cannot measure it.
	StateNotSupported = "not_supported"
	// StateMissing: the key is not in the response at all (catalogue drift).
	StateMissing = "missing"
	// StateNotScalar: the value is a list; profiles reference scalars only.
	StateNotScalar = "not_scalar"
	// StateNoValue: visible but null (the worker's "degenerate geometry").
	StateNoValue = "no_value"
)

// Observation is one measurement as a classifier or trait sees it.
type Observation struct {
	Key   string
	State string
	Value float64
	// Band is the worker's noise band re-centred on Value
	// (Value + P5 - P50, Value + P95 - P50), so it contains Value. It is
	// conservative (independent noise at the model card's worst regional
	// error). Nil when the worker sent none (bands disabled, or too few
	// finite perturbed samples): no uncertainty information, not "certain".
	Band *[2]float64
}

// Usable reports whether the observation carries a float value.
func (o Observation) Usable() bool { return o.State == StateUsable }

// Index looks measurements up by key.
type Index struct {
	byKey map[string]Measurement
}

// Index builds an Index over the response's measurements.
func (r *Response) Index() Index {
	ix := Index{byKey: make(map[string]Measurement, len(r.Measurements))}
	for _, m := range r.Measurements {
		ix.byKey[m.Key] = m
	}
	return ix
}

// Observe classifies the measurement named key.
func (ix Index) Observe(key string) Observation {
	m, ok := ix.byKey[key]
	switch {
	case !ok:
		return Observation{Key: key, State: StateMissing}
	case m.Visibility == VisibilityNotSupported:
		return Observation{Key: key, State: StateNotSupported}
	case m.Visibility == VisibilityOccluded:
		return Observation{Key: key, State: StateOccluded}
	case m.Value.IsList():
		return Observation{Key: key, State: StateNotScalar}
	case m.Value.Scalar == nil:
		return Observation{Key: key, State: StateNoValue}
	}
	o := Observation{Key: key, State: StateUsable, Value: *m.Value.Scalar}
	if m.Band != nil && m.Band[0].Scalar != nil && m.Band[1].Scalar != nil {
		o.Band = &[2]float64{*m.Band[0].Scalar, *m.Band[1].Scalar}
	}
	return o
}
```

- [ ] **Step 7: Run; expect PASS** (9 tests). `TestMeasurement_RoundTripsJSON` and `TestCatalogueEntry_RoundTripsJSON` prove the Go types re-encode every recorded measurement and catalogue entry to identical JSON, including `"reason": null`, `"shape"` and list bands.

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 8: Commit**
```bash
git add apps/core-engine/internal/facearch/facearchtest apps/core-engine/internal/facearch/measureclient/types.go apps/core-engine/internal/facearch/measureclient/shape.go apps/core-engine/internal/facearch/measureclient/observe.go apps/core-engine/internal/facearch/measureclient/types_test.go apps/core-engine/internal/facearch/measureclient/observe_test.go
git commit -m "feat(core-engine): face-measure types and recorded ai-worker payloads"
```

---

### Task 2: measureclient HTTP client

**Files:**
- Create: `apps/core-engine/internal/facearch/measureclient/client.go`
- Test: `apps/core-engine/internal/facearch/measureclient/client_test.go`
- Test: `apps/core-engine/internal/facearch/measureclient/live_test.go` (opt-in)

**Interfaces:**
- Consumes: Task 1 types; ai-worker `POST /api/v1/face-measure` (multipart `image`, form `landmarks=true`), `GET /api/v1/face-measure/catalogue`.
- Produces: `var ErrUnavailable error`; `type UpstreamError struct{ Status int; Body []byte }` (`Error() string`); `type Client`; `func New(baseURL string, timeout time.Duration) *Client`; `func (c *Client) Measure(ctx context.Context, image []byte, filename string) (*Response, error)`; `func (c *Client) Catalogue(ctx context.Context) (*Catalogue, error)`. 400/422/503 → `*UpstreamError` with the body verbatim; any other failure → wraps `ErrUnavailable`.

- [ ] **Step 1: Write the failing tests**

`apps/core-engine/internal/facearch/measureclient/client_test.go`:
```go
package measureclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

func TestMeasure_SendsImageWithLandmarksAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/face-measure", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, "true", r.FormValue("landmarks"))
		f, hdr, err := r.FormFile("image")
		require.NoError(t, err)
		data, _ := io.ReadAll(f)
		assert.Equal(t, []byte("jpeg-bytes"), data)
		assert.Equal(t, "face.jpg", hdr.Filename)
		_, _ = w.Write(facearchtest.FixtureFaceJSON())
	}))
	defer srv.Close()

	resp, err := mc.New(srv.URL+"/", 5*time.Second).Measure(context.Background(), []byte("jpeg-bytes"), "face.jpg")
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", resp.CatalogueVersion)
	assert.Equal(t, "face_landmarker", resp.Model.Name)
	assert.Equal(t, []string{"roll", "yaw", "pitch", "iod_px"}, resp.Quality.GatesPassed)
	assert.Contains(t, resp.Quality.Warnings, "face_partly_out_of_frame")
	assert.Equal(t, "occluded", resp.Regions["jaw"])
	assert.Len(t, resp.Measurements, 49)
}

func TestMeasure_PassesThroughClientAndAvailabilityErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusServiceUnavailable} {
		body := `{"detail":{"code":"quality_gates_failed","failedGates":[{"gate":"roll","value":20.0,"limit":8.0,"passed":false}]}}`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		_, err := mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
		srv.Close()
		var up *mc.UpstreamError
		require.True(t, errors.As(err, &up), "status %d", status)
		assert.Equal(t, status, up.Status)
		assert.JSONEq(t, body, string(up.Body))
	}
}

func TestMeasure_OtherFailuresAreUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_, err := mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
	srv.Close()
	assert.ErrorIs(t, err, mc.ErrUnavailable)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	_, err = mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg")
	srv.Close()
	assert.ErrorIs(t, err, mc.ErrUnavailable)

	_, err = mc.New(srv.URL, 5*time.Second).Measure(context.Background(), []byte("x"), "x.jpg") // server closed
	assert.ErrorIs(t, err, mc.ErrUnavailable)
}

func TestCatalogue_Decodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/face-measure/catalogue", r.URL.Path)
		_, _ = w.Write(facearchtest.CatalogueJSON())
	}))
	defer srv.Close()

	cat, err := mc.New(srv.URL, 5*time.Second).Catalogue(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", cat.CatalogueVersion)
	e, ok := cat.Entry("eyelid_type")
	require.True(t, ok)
	assert.False(t, e.Supported)
	e, ok = cat.Entry("width_length_ratio")
	require.True(t, ok)
	assert.Equal(t, []string{"forehead", "jaw", "contour_right", "contour_left"}, e.Regions)
}
```

`apps/core-engine/internal/facearch/measureclient/live_test.go`:
```go
package measureclient_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// liveTimeout bounds the opt-in call below; test-only.
const liveTimeout = 30 * time.Second

// TestRecordedCatalogueMatchesLiveWorker checks the recorded catalogue
// against a running ai-worker (Plan 1 landed). Opt-in: set
// FACEARCH_TEST_AI_WORKER_URL, e.g. http://127.0.0.1:8088.
func TestRecordedCatalogueMatchesLiveWorker(t *testing.T) {
	url := os.Getenv("FACEARCH_TEST_AI_WORKER_URL")
	if url == "" {
		t.Skip("FACEARCH_TEST_AI_WORKER_URL not set")
	}
	live, err := mc.New(url, liveTimeout).Catalogue(context.Background())
	require.NoError(t, err)
	rec := facearchtest.Catalogue()
	assert.Equal(t, rec.CatalogueVersion, live.CatalogueVersion)
	require.Equal(t, len(rec.Measurements), len(live.Measurements))
	for i, want := range rec.Measurements {
		got := live.Measurements[i]
		assert.Equal(t, want.Key, got.Key)
		assert.Equal(t, want.Unit, got.Unit, want.Key)
		assert.Equal(t, want.Regions, got.Regions, want.Key)
		assert.Equal(t, want.Shape, got.Shape, want.Key)
		assert.Equal(t, want.Landmarks, got.Landmarks, want.Key)
		assert.Equal(t, want.Supported, got.Supported, want.Key)
		assert.Equal(t, want.Proxy, got.Proxy, want.Key)
	}
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: mc.New`, `mc.UpstreamError`, `mc.ErrUnavailable`).

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 3: Implement the client**

`apps/core-engine/internal/facearch/measureclient/client.go`:
```go
package measureclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable wraps every failure to get an answer from the ai-worker
// other than the statuses it passes through (see UpstreamError):
// connection errors, timeouts, undecodable bodies and unexpected statuses.
var ErrUnavailable = errors.New("face-measure unavailable")

// passThrough are the ai-worker statuses whose bodies core-engine returns
// to its caller unchanged (spec: "Passes 422 and 503 through with their
// bodies"; 400 is the worker's unreadable-image answer and is the caller's
// fault in the same way).
var passThrough = map[int]bool{
	http.StatusBadRequest:          true,
	http.StatusUnprocessableEntity: true,
	http.StatusServiceUnavailable:  true,
}

// UpstreamError is a 400, 422 or 503 from the ai-worker, with its body verbatim.
type UpstreamError struct {
	Status int
	Body   []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("face-measure returned %d: %s", e.Status, strings.TrimSpace(string(e.Body)))
}

// Client calls the ai-worker's face-measure endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New returns a Client for the ai-worker at baseURL (VISION_AI_WORKER_URL).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Measure posts image as multipart field "image" with landmarks=true.
func (c *Client) Measure(ctx context.Context, image []byte, filename string) (*Response, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile("image", filename)
	if err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if _, err := part.Write(image); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if err := w.WriteField("landmarks", "true"); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("encode face-measure request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+MeasurePath, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	var out Response
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Catalogue fetches GET /api/v1/face-measure/catalogue.
func (c *Client) Catalogue(ctx context.Context) (*Catalogue, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+CataloguePath, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var out Catalogue
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	if passThrough[resp.StatusCode] {
		return &UpstreamError{Status: resp.StatusCode, Body: raw}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s returned status %d", ErrUnavailable, req.URL.Path, resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrUnavailable, req.URL.Path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run; expect PASS** (`TestRecordedCatalogueMatchesLiveWorker` SKIPs unless `FACEARCH_TEST_AI_WORKER_URL` is set).

Run: `go test ./apps/core-engine/internal/facearch/measureclient/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/measureclient/client.go apps/core-engine/internal/facearch/measureclient/client_test.go apps/core-engine/internal/facearch/measureclient/live_test.go
git commit -m "feat(core-engine): face-measure HTTP client with 400/422/503 pass-through"
```

---

### Task 3: Profile entity and document types

**Files:**
- Create: `apps/core-engine/internal/facearch/profile/entity.go`
- Create: `apps/core-engine/internal/facearch/profile/document.go`
- Test: `apps/core-engine/internal/facearch/profile/document_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `const StatusActive, StatusInactive, Wildcard`; `func NormalizeStatus(status string) string`; `type FaceArchitectureProfile struct{ ID, BrandID, ApplicationID, Code, Title, Status string; Version int; Document string; CreatedAt time.Time; UpdatedAt *time.Time }` with `TableName() == "face_architecture_profiles"`; `const SchemaVersion = 1`; `const CalibrationUncalibrated, CalibrationCalibrated`; `type Document struct{ SchemaVersion int; CatalogueVersion string; Calibration Calibration; MeasurementsShown []string; Classifiers []ClassifierSpec; Traits []TraitSpec; Guidance json.RawMessage }`; `type Calibration struct{ Status string; BenchmarkRunID, Target *string; HeldOutMetrics json.RawMessage; Sources []Source }`; `type Source struct{ Cite, For, Note string }`; `type ClassifierSpec struct{ Key string; BlendMargin, MinObservedWeight float64; Classes []ClassSpec }` with `Keys() []string`; `type ClassSpec struct{ Label string; Requires []string; Prototype map[string]PrototypeTerm }` with `SortedKeys() []string`; `type PrototypeTerm struct{ Centre, Spread, Weight float64 }`; `type TraitSpec struct{ Key string; Rules []RuleSpec; Default string }` with `Keys() []string`; `type RuleSpec struct{ Label string; When []Condition }`; `type Condition struct{ Measurement, Op string; Value float64 }` with `Holds(x float64) bool`; `const OpLess, OpLessEqual, OpGreater, OpGreaterEqual`; `func Parse(raw []byte) (*Document, error)` (strict: unknown fields rejected).

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/profile/document_test.go`:
```go
package profile_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// validDoc is a small, complete document. Its numbers are test inputs, not
// proposed values.
const validDoc = `{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {
    "status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide.prototype.width_length_ratio"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]
  },
  "measurementsShown": ["width_length_ratio", "fifths"],
  "classifiers": [{
    "key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5,
    "classes": [
      {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
      {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}
    ]
  }],
  "traits": [{
    "key": "noseWidth",
    "rules": [{"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]}],
    "default": "Average"
  }]
}`

func parseValid(t *testing.T) *profile.Document {
	t.Helper()
	d, err := profile.Parse([]byte(validDoc))
	require.NoError(t, err)
	return d
}

func TestNormalizeStatus(t *testing.T) {
	for in, want := range map[string]string{"": "ACTIVE", "active": "ACTIVE", " Inactive ": "INACTIVE"} {
		assert.Equal(t, want, profile.NormalizeStatus(in), in)
	}
	assert.Equal(t, "face_architecture_profiles", profile.FaceArchitectureProfile{}.TableName())
}

func TestParse_StrictAndGuidanceOpaque(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, "faceShape", d.Classifiers[0].Key)
	assert.Nil(t, d.Guidance)

	_, err := profile.Parse([]byte(`{"schemaVersion": 1, "blendMarign": 0.6}`))
	assert.Error(t, err, "unknown fields are rejected")

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(validDoc), &raw))
	raw["guidance"] = map[string]any{"templates": map[string]any{"x": map[string]any{"role": "contour"}}}
	b, _ := json.Marshal(raw)
	d, err = profile.Parse(b)
	require.NoError(t, err)
	assert.JSONEq(t, `{"templates":{"x":{"role":"contour"}}}`, string(d.Guidance))
}

func TestConditionHolds(t *testing.T) {
	c := profile.Condition{Measurement: "m", Op: ">", Value: 1}
	assert.True(t, c.Holds(1.5))
	assert.False(t, c.Holds(1))
	c.Op = ">="
	assert.True(t, c.Holds(1))
	c.Op = "<"
	assert.True(t, c.Holds(0.5))
	c.Op = "<="
	assert.True(t, c.Holds(1))
	c.Op = "=="
	assert.False(t, c.Holds(1))
}

func TestKeys(t *testing.T) {
	d := parseValid(t)
	assert.Equal(t, []string{"width_length_ratio"}, d.Classifiers[0].Keys())
	assert.Equal(t, []string{"alar_width_to_intercanthal_ratio"}, d.Traits[0].Keys())
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: package `profile` does not exist).

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 3: Implement the entity**

`apps/core-engine/internal/facearch/profile/entity.go`:
```go
// Package profile holds per-brand face-architecture profiles: the entity,
// the document inside it, validation, and lookup.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Interpretation layer (core-engine)".
package profile

import (
	"strings"
	"time"
)

// Profile statuses. Uppercase, like score_rulesets.
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
)

// Wildcard matches any brand or application in a profile's scope.
const Wildcard = "*"

// NormalizeStatus returns status trimmed and uppercase, defaulting to ACTIVE
// (same rule as score/entity.NormalizeRulesetStatus).
func NormalizeStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return StatusActive
	}
	return status
}

// FaceArchitectureProfile is one version of a brand's interpretation policy.
// Table face_architecture_profiles is new and owned by core-engine
// (docs/SHARED-DATABASE.md): nothing in Seagull-gateway reads or migrates it.
// The unique index name is prefixed so it cannot collide with another
// table's index in the shared schema.
type FaceArchitectureProfile struct {
	ID            string     `gorm:"type:uuid;primaryKey" json:"id"`
	BrandID       string     `gorm:"type:varchar(64);not null;default:'*';uniqueIndex:uq_facearch_profile_version" json:"brandId"`
	ApplicationID string     `gorm:"type:varchar(64);not null;default:'*';uniqueIndex:uq_facearch_profile_version" json:"applicationId"`
	Code          string     `gorm:"type:varchar(64);not null;uniqueIndex:uq_facearch_profile_version" json:"code"`
	Title         string     `gorm:"type:varchar(255);not null" json:"title"`
	Status        string     `gorm:"type:varchar(32);not null;default:'ACTIVE';index" json:"status"`
	Version       int        `gorm:"type:int;not null;default:1;uniqueIndex:uq_facearch_profile_version" json:"version"`
	Document      string     `gorm:"type:jsonb;not null" json:"document"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     *time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (FaceArchitectureProfile) TableName() string { return "face_architecture_profiles" }
```

- [ ] **Step 4: Implement the document types**

`apps/core-engine/internal/facearch/profile/document.go`:
```go
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// SchemaVersion is the document schema this code reads.
const SchemaVersion = 1

// Calibration statuses. Every response repeats the profile's status.
const (
	CalibrationUncalibrated = "uncalibrated"
	CalibrationCalibrated   = "calibrated"
)

// Document is the JSON stored in FaceArchitectureProfile.Document.
type Document struct {
	SchemaVersion     int              `json:"schemaVersion"`
	CatalogueVersion  string           `json:"catalogueVersion"`
	Calibration       Calibration      `json:"calibration"`
	MeasurementsShown []string         `json:"measurementsShown"`
	Classifiers       []ClassifierSpec `json:"classifiers"`
	Traits            []TraitSpec      `json:"traits"`
	// Guidance belongs to Plan 3 (templates and rules). It is carried
	// opaquely so a profile with guidance round-trips unchanged; nothing in
	// this plan reads it and responses return guidance as null.
	Guidance json.RawMessage `json:"guidance,omitempty"`
}

// Calibration says where the profile's numbers came from.
type Calibration struct {
	Status         string          `json:"status"`
	BenchmarkRunID *string         `json:"benchmarkRunId"`
	Target         *string         `json:"target"`
	HeldOutMetrics json.RawMessage `json:"heldOutMetrics"`
	Sources        []Source        `json:"sources"`
}

// Source cites where a value came from. For is a document path such as
// "classifiers.faceShape.blendMargin" or "traits.noseWidth"; see Validate.
type Source struct {
	Cite string `json:"cite"`
	For  string `json:"for"`
	Note string `json:"note,omitempty"`
}

// ClassifierSpec is one prototype classifier (e.g. faceShape).
type ClassifierSpec struct {
	Key               string      `json:"key"`
	BlendMargin       float64     `json:"blendMargin"`
	MinObservedWeight float64     `json:"minObservedWeight"`
	Classes           []ClassSpec `json:"classes"`
}

// ClassSpec is one class and its prototype.
type ClassSpec struct {
	Label     string                   `json:"label"`
	Requires  []string                 `json:"requires"`
	Prototype map[string]PrototypeTerm `json:"prototype"`
}

// PrototypeTerm is one measurement's centre, spread and weight in a prototype.
type PrototypeTerm struct {
	Centre float64 `json:"centre"`
	Spread float64 `json:"spread"`
	Weight float64 `json:"weight"`
}

// TraitSpec is an ordered rule list; the first rule whose conditions all
// hold wins, otherwise Default.
type TraitSpec struct {
	Key     string     `json:"key"`
	Rules   []RuleSpec `json:"rules"`
	Default string     `json:"default"`
}

// RuleSpec assigns Label when every condition holds.
type RuleSpec struct {
	Label string      `json:"label"`
	When  []Condition `json:"when"`
}

// Condition compares one measurement with a threshold.
type Condition struct {
	Measurement string  `json:"measurement"`
	Op          string  `json:"op"`
	Value       float64 `json:"value"`
}

// Comparison operators a Condition may use.
const (
	OpLess         = "<"
	OpLessEqual    = "<="
	OpGreater      = ">"
	OpGreaterEqual = ">="
)

// Holds reports whether x satisfies the condition. Unknown operators never
// hold; Validate rejects them before a document is stored.
func (c Condition) Holds(x float64) bool {
	switch c.Op {
	case OpLess:
		return x < c.Value
	case OpLessEqual:
		return x <= c.Value
	case OpGreater:
		return x > c.Value
	case OpGreaterEqual:
		return x >= c.Value
	}
	return false
}

// SortedKeys returns the prototype's measurement keys in sorted order, so
// sums over a prototype are deterministic.
func (c ClassSpec) SortedKeys() []string {
	keys := make([]string, 0, len(c.Prototype))
	for k := range c.Prototype {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Keys returns every measurement key the classifier references, sorted.
func (s ClassifierSpec) Keys() []string {
	set := map[string]bool{}
	for _, c := range s.Classes {
		for _, k := range c.Requires {
			set[k] = true
		}
		for k := range c.Prototype {
			set[k] = true
		}
	}
	return sortedSet(set)
}

// Keys returns every measurement key the trait's conditions reference, sorted.
func (s TraitSpec) Keys() []string {
	set := map[string]bool{}
	for _, r := range s.Rules {
		for _, c := range r.When {
			set[c.Measurement] = true
		}
	}
	return sortedSet(set)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Parse decodes a document strictly: unknown fields are an error, so a
// misspelt key cannot silently drop a threshold.
func Parse(raw []byte) (*Document, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("face-architecture profile document: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("face-architecture profile document: trailing data after the JSON object")
	}
	return &d, nil
}
```

- [ ] **Step 5: Run; expect PASS** (4 tests).

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 6: Commit**
```bash
git add apps/core-engine/internal/facearch/profile/entity.go apps/core-engine/internal/facearch/profile/document.go apps/core-engine/internal/facearch/profile/document_test.go
git commit -m "feat(core-engine): face-architecture profile entity and document types"
```

---

### Task 4: Profile validation

**Files:**
- Create: `apps/core-engine/internal/facearch/profile/validate.go`
- Test: `apps/core-engine/internal/facearch/profile/validate_test.go`

**Interfaces:**
- Consumes: Task 3 types; `measureclient.Catalogue` (`Entry`, `Shape`), `measureclient.ShapeScalar` (Task 1); `facearchtest.Catalogue()` in tests.
- Produces: `type Problem struct{ Path, Message string }`; `type ValidationError struct{ Problems []Problem }` (`Error() string`); `func Validate(d *Document, cat *measureclient.Catalogue) error` (nil catalogue = structure only); `func Load(p *FaceArchitectureProfile) (*Document, error)` (Parse + structural Validate).

- [ ] **Step 1: Write the failing test** (uses `validDoc` and `parseValid` from Task 3's `document_test.go`)

`apps/core-engine/internal/facearch/profile/validate_test.go`:
```go
package profile_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func problems(t *testing.T, err error) string {
	t.Helper()
	var ve *profile.ValidationError
	require.True(t, errors.As(err, &ve), "want *ValidationError, got %v", err)
	return ve.Error()
}

func TestValidate_ValidDocument(t *testing.T) {
	assert.NoError(t, profile.Validate(parseValid(t), facearchtest.Catalogue()))
	assert.NoError(t, profile.Validate(parseValid(t), nil))
}

func TestValidate_Rules(t *testing.T) {
	cases := map[string]struct {
		mutate func(d *profile.Document)
		want   string
	}{
		"schema version": {func(d *profile.Document) { d.SchemaVersion = 2 }, "schemaVersion: got 2"},
		"catalogue version": {func(d *profile.Document) { d.CatalogueVersion = "fa-measure/0" },
			"document targets fa-measure/0, ai-worker catalogue is fa-measure/1"},
		"unknown key": {func(d *profile.Document) { d.MeasurementsShown = append(d.MeasurementsShown, "golden_ratio") },
			`"golden_ratio" is not in catalogue`},
		"unsupported key": {func(d *profile.Document) { d.MeasurementsShown = append(d.MeasurementsShown, "eyelid_type") },
			`"eyelid_type" is not supported`},
		"list-valued prototype": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["fifths"] = profile.PrototypeTerm{Centre: 0.2, Spread: 0.1, Weight: 1}
		}, `"fifths" is list-valued`},
		"list-valued rule": {func(d *profile.Document) { d.Traits[0].Rules[0].When[0].Measurement = "jaw_contour_angles" },
			`"jaw_contour_angles" is list-valued`},
		"zero spread": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["width_length_ratio"] = profile.PrototypeTerm{Centre: 0.6, Spread: 0, Weight: 1}
		}, "spread: must be positive"},
		"zero weight": {func(d *profile.Document) {
			d.Classifiers[0].Classes[0].Prototype["width_length_ratio"] = profile.PrototypeTerm{Centre: 0.6, Spread: 0.1, Weight: 0}
		}, "weight: must be positive"},
		"blend margin 0.5":        {func(d *profile.Document) { d.Classifiers[0].BlendMargin = 0.5 }, "blendMargin: must be in (0.5, 1]"},
		"blend margin above 1":    {func(d *profile.Document) { d.Classifiers[0].BlendMargin = 1.01 }, "blendMargin: must be in (0.5, 1]"},
		"min observed weight":     {func(d *profile.Document) { d.Classifiers[0].MinObservedWeight = -0.1 }, "minObservedWeight: must be in [0, 1]"},
		"one class":               {func(d *profile.Document) { d.Classifiers[0].Classes = d.Classifiers[0].Classes[:1] }, "needs at least 2 classes"},
		"duplicate class":         {func(d *profile.Document) { d.Classifiers[0].Classes[1].Label = "Long" }, `duplicate class "Long"`},
		"bad op":                  {func(d *profile.Document) { d.Traits[0].Rules[0].When[0].Op = "==" }, "must be one of < <= > >="},
		"empty default":           {func(d *profile.Document) { d.Traits[0].Default = "" }, "default: is empty"},
		"bad calibration status":  {func(d *profile.Document) { d.Calibration.Status = "guessed" }, "calibration.status"},
		"calibrated needs run id": {func(d *profile.Document) { d.Calibration.Status = "calibrated" }, "benchmarkRunId: required"},
		"uncited class": {func(d *profile.Document) { d.Calibration.Sources = d.Calibration.Sources[:2] },
			"no source cited for classifiers.faceShape.classes.Long"},
		"uncited trait": {func(d *profile.Document) { d.Calibration.Sources = d.Calibration.Sources[:4] },
			"no source cited for traits.noseWidth"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := parseValid(t)
			tc.mutate(d)
			msg := problems(t, profile.Validate(d, facearchtest.Catalogue()))
			assert.Contains(t, msg, tc.want)
		})
	}
}

func TestValidate_ParentSourceDoesNotCoverChildren(t *testing.T) {
	d := parseValid(t)
	d.Calibration.Sources = []profile.Source{{Cite: "vocabulary only", For: "traits"}, {Cite: "x", For: "classifiers"}}
	msg := problems(t, profile.Validate(d, nil))
	assert.Contains(t, msg, "traits.noseWidth")
	assert.Contains(t, msg, "classifiers.faceShape.blendMargin")
}

func TestValidate_ReportsEveryProblem(t *testing.T) {
	d := parseValid(t)
	d.SchemaVersion = 0
	d.Classifiers[0].BlendMargin = 2
	d.Traits[0].Default = ""
	var ve *profile.ValidationError
	require.True(t, errors.As(profile.Validate(d, nil), &ve))
	paths := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		paths[i] = p.Path
	}
	assert.Equal(t, "schemaVersion,classifiers[0].blendMargin,traits[0].default", strings.Join(paths, ","))
}

func TestLoad(t *testing.T) {
	d, err := profile.Load(&profile.FaceArchitectureProfile{Document: validDoc})
	require.NoError(t, err)
	assert.Equal(t, "fa-measure/1", d.CatalogueVersion)

	_, err = profile.Load(&profile.FaceArchitectureProfile{Document: `{"schemaVersion": 7}`})
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: profile.Validate`, `profile.ValidationError`, `profile.Load`).

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 3: Implement validation**

`apps/core-engine/internal/facearch/profile/validate.go`:
```go
package profile

import (
	"fmt"
	"math"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
)

// Problem is one validation failure at a document path.
type Problem struct {
	Path    string
	Message string
}

// ValidationError lists every problem found, not just the first.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.Path + ": " + p.Message
	}
	return "invalid face-architecture profile:\n  " + strings.Join(lines, "\n  ")
}

type checker struct {
	cat      *measureclient.Catalogue
	problems []Problem
}

func (c *checker) addf(path, format string, args ...any) {
	c.problems = append(c.problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

// measurement checks a key against the catalogue. scalar requires a float
// value: classifiers and trait rules compare numbers, so a list-valued
// measurement (fifths, jaw_contour_angles) cannot be referenced by them.
// Without a catalogue only emptiness is checked.
func (c *checker) measurement(path, key string, scalar bool) {
	if key == "" {
		c.addf(path, "measurement key is empty")
		return
	}
	if c.cat == nil {
		return
	}
	e, ok := c.cat.Entry(key)
	if !ok {
		c.addf(path, "measurement %q is not in catalogue %s", key, c.cat.CatalogueVersion)
		return
	}
	if !e.Supported {
		c.addf(path, "measurement %q is not supported by catalogue %s", key, c.cat.CatalogueVersion)
		return
	}
	if !scalar {
		return
	}
	shape, err := c.cat.Shape(key)
	if err != nil {
		c.addf(path, "%v", err)
		return
	}
	if shape != measureclient.ShapeScalar {
		c.addf(path, "measurement %q is list-valued; classifiers and trait rules take scalar measurements only", key)
	}
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Validate checks a document. With a catalogue (upsert, CLI validate) it
// also checks every measurement key against it; with nil (load) it checks
// structure only. It returns a *ValidationError listing every problem.
//
// Rules (spec "Validation"): keys exist and are supported; prototype and
// rule keys are scalar; spreads and weights are positive; blendMargin is in
// (0.5, 1]; minObservedWeight is in [0, 1]. An uncalibrated document must
// cite a source for each classifier's blendMargin, minObservedWeight and
// every class, and for every trait: a source covers path P when its "for"
// is P or a path under P.
func Validate(d *Document, cat *measureclient.Catalogue) error {
	c := &checker{cat: cat}
	if d.SchemaVersion != SchemaVersion {
		c.addf("schemaVersion", "got %d, this code reads %d", d.SchemaVersion, SchemaVersion)
	}
	switch {
	case d.CatalogueVersion == "":
		c.addf("catalogueVersion", "is empty")
	case cat != nil && d.CatalogueVersion != cat.CatalogueVersion:
		c.addf("catalogueVersion", "document targets %s, ai-worker catalogue is %s", d.CatalogueVersion, cat.CatalogueVersion)
	}
	c.calibration(d)

	seen := map[string]bool{}
	for i, k := range d.MeasurementsShown {
		path := fmt.Sprintf("measurementsShown[%d]", i)
		if seen[k] {
			c.addf(path, "duplicate %q", k)
		}
		seen[k] = true
		c.measurement(path, k, false)
	}

	keys := map[string]bool{}
	for i, s := range d.Classifiers {
		c.classifier(fmt.Sprintf("classifiers[%d]", i), s, keys)
	}
	keys = map[string]bool{}
	for i, t := range d.Traits {
		c.trait(fmt.Sprintf("traits[%d]", i), t, keys)
	}

	if d.Calibration.Status == CalibrationUncalibrated {
		c.coverage(d)
	}
	if len(c.problems) > 0 {
		return &ValidationError{Problems: c.problems}
	}
	return nil
}

func (c *checker) calibration(d *Document) {
	cal := d.Calibration
	switch cal.Status {
	case CalibrationUncalibrated:
	case CalibrationCalibrated:
		if cal.BenchmarkRunID == nil || *cal.BenchmarkRunID == "" {
			c.addf("calibration.benchmarkRunId", "required when status is calibrated")
		}
		if cal.Target == nil || *cal.Target == "" {
			c.addf("calibration.target", "required when status is calibrated")
		}
	default:
		c.addf("calibration.status", "must be %q or %q, got %q", CalibrationUncalibrated, CalibrationCalibrated, cal.Status)
	}
	for i, s := range cal.Sources {
		if strings.TrimSpace(s.Cite) == "" {
			c.addf(fmt.Sprintf("calibration.sources[%d].cite", i), "is empty")
		}
		if strings.TrimSpace(s.For) == "" {
			c.addf(fmt.Sprintf("calibration.sources[%d].for", i), "is empty")
		}
	}
}

func (c *checker) classifier(path string, s ClassifierSpec, keys map[string]bool) {
	if s.Key == "" {
		c.addf(path+".key", "is empty")
	} else if keys[s.Key] {
		c.addf(path+".key", "duplicate classifier %q", s.Key)
	}
	keys[s.Key] = true
	if !(s.BlendMargin > 0.5 && s.BlendMargin <= 1) {
		c.addf(path+".blendMargin", "must be in (0.5, 1], got %v", s.BlendMargin)
	}
	if !(s.MinObservedWeight >= 0 && s.MinObservedWeight <= 1) {
		c.addf(path+".minObservedWeight", "must be in [0, 1], got %v", s.MinObservedWeight)
	}
	if len(s.Classes) < 2 {
		c.addf(path+".classes", "needs at least 2 classes, got %d", len(s.Classes))
	}
	labels := map[string]bool{}
	for j, cl := range s.Classes {
		cp := fmt.Sprintf("%s.classes[%d]", path, j)
		if cl.Label == "" {
			c.addf(cp+".label", "is empty")
		} else if labels[cl.Label] {
			c.addf(cp+".label", "duplicate class %q", cl.Label)
		}
		labels[cl.Label] = true
		if len(cl.Prototype) == 0 {
			c.addf(cp+".prototype", "is empty")
		}
		for _, k := range cl.SortedKeys() {
			t := cl.Prototype[k]
			tp := cp + ".prototype." + k
			c.measurement(tp, k, true)
			if !finite(t.Centre) {
				c.addf(tp+".centre", "must be finite")
			}
			if !(t.Spread > 0) || !finite(t.Spread) {
				c.addf(tp+".spread", "must be positive and finite, got %v", t.Spread)
			}
			if !(t.Weight > 0) || !finite(t.Weight) {
				c.addf(tp+".weight", "must be positive and finite, got %v", t.Weight)
			}
		}
		req := map[string]bool{}
		for r, k := range cl.Requires {
			rp := fmt.Sprintf("%s.requires[%d]", cp, r)
			if req[k] {
				c.addf(rp, "duplicate %q", k)
			}
			req[k] = true
			c.measurement(rp, k, true)
		}
	}
}

func (c *checker) trait(path string, t TraitSpec, keys map[string]bool) {
	if t.Key == "" {
		c.addf(path+".key", "is empty")
	} else if keys[t.Key] {
		c.addf(path+".key", "duplicate trait %q", t.Key)
	}
	keys[t.Key] = true
	if t.Default == "" {
		c.addf(path+".default", "is empty")
	}
	if len(t.Rules) == 0 {
		c.addf(path+".rules", "needs at least 1 rule")
	}
	for i, r := range t.Rules {
		rp := fmt.Sprintf("%s.rules[%d]", path, i)
		if r.Label == "" {
			c.addf(rp+".label", "is empty")
		}
		if len(r.When) == 0 {
			c.addf(rp+".when", "needs at least 1 condition")
		}
		for j, cond := range r.When {
			cp := fmt.Sprintf("%s.when[%d]", rp, j)
			c.measurement(cp+".measurement", cond.Measurement, true)
			switch cond.Op {
			case OpLess, OpLessEqual, OpGreater, OpGreaterEqual:
			default:
				c.addf(cp+".op", "must be one of < <= > >=, got %q", cond.Op)
			}
			if !finite(cond.Value) {
				c.addf(cp+".value", "must be finite")
			}
		}
	}
}

func (c *checker) coverage(d *Document) {
	var required []string
	for _, s := range d.Classifiers {
		base := "classifiers." + s.Key
		required = append(required, base+".blendMargin", base+".minObservedWeight")
		for _, cl := range s.Classes {
			required = append(required, base+".classes."+cl.Label)
		}
	}
	for _, t := range d.Traits {
		required = append(required, "traits."+t.Key)
	}
	for _, p := range required {
		if !covered(d.Calibration.Sources, p) {
			c.addf("calibration.sources", "no source cited for %s (uncalibrated profiles cite every value)", p)
		}
	}
}

func covered(sources []Source, path string) bool {
	for _, s := range sources {
		if s.For == path || strings.HasPrefix(s.For, path+".") {
			return true
		}
	}
	return false
}

// Load parses a stored profile's document and checks its structure. The
// catalogue is not consulted here: drift against the live catalogue is
// judged per request from the measure response (facearch.Interpret).
func Load(p *FaceArchitectureProfile) (*Document, error) {
	d, err := Parse([]byte(p.Document))
	if err != nil {
		return nil, err
	}
	if err := Validate(d, nil); err != nil {
		return nil, err
	}
	return d, nil
}
```

- [ ] **Step 4: Run; expect PASS.**

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/profile/validate.go apps/core-engine/internal/facearch/profile/validate_test.go
git commit -m "feat(core-engine): validate face-architecture profiles against the catalogue"
```

---

### Task 5: Profile stores and lookup

**Files:**
- Create: `apps/core-engine/internal/facearch/profile/store.go`
- Create: `apps/core-engine/internal/facearch/profile/memory_store.go`
- Create: `apps/core-engine/internal/facearch/profile/gorm_store.go`
- Test: `apps/core-engine/internal/facearch/profile/store_test.go`

**Interfaces:**
- Consumes: Task 3 entity; `gorm.io/gorm`, `github.com/google/uuid` (already in `apps/core-engine/go.mod`).
- Produces: `var ErrNoActiveProfile error`; `type UpsertInput struct{ BrandID, ApplicationID, Code, Title string; Document []byte }`; `type Store interface{ Active(ctx context.Context, brandID, appID string) (*FaceArchitectureProfile, error); Upsert(ctx context.Context, in UpsertInput) (*FaceArchitectureProfile, error) }`; `func Scopes(brandID, appID string) [][2]string`; `type MemoryStore` (`NewMemoryStore() *MemoryStore`, `Put(p FaceArchitectureProfile)`, `Active`, `Upsert`; tests only); `type GormStore` (`NewGormStore(db *gorm.DB) *GormStore`, `Active`, `Upsert`).

- [ ] **Step 1: Write the failing test.** `storeContract` runs against `MemoryStore` always and against Postgres only when `FACEARCH_TEST_DATABASE_URL` names a **disposable** database (it deletes every row in `face_architecture_profiles`). Never point it at the shared database.

`apps/core-engine/internal/facearch/profile/store_test.go`:
```go
package profile_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func TestScopes(t *testing.T) {
	assert.Equal(t, [][2]string{{"wardah", "app1"}, {"wardah", "*"}, {"*", "*"}}, profile.Scopes("wardah", "app1"))
	assert.Equal(t, [][2]string{{"wardah", "*"}, {"*", "*"}}, profile.Scopes("wardah", "*"))
	assert.Equal(t, [][2]string{{"*", "*"}}, profile.Scopes("", ""))
}

// storeContract is the behaviour every Store must have. It runs against
// MemoryStore always and against Postgres when FACEARCH_TEST_DATABASE_URL
// names a scratch database.
func storeContract(t *testing.T, s profile.Store) {
	ctx := context.Background()

	_, err := s.Active(ctx, "brand-a", "app-1")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)

	global, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "base", Document: []byte(`{"n":1}`)})
	require.NoError(t, err)
	assert.Equal(t, 1, global.Version)
	assert.Equal(t, "base", global.Title, "title defaults to code")

	got, err := s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, global.ID, got.ID, "falls back to *+*")

	brand, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "brand-a", ApplicationID: "*", Code: "brand", Title: "Brand A", Document: []byte(`{"n":2}`)})
	require.NoError(t, err)
	got, err = s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, brand.ID, got.ID, "brand+* beats *+*")

	app, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "brand-a", ApplicationID: "app-1", Code: "app", Document: []byte(`{"n":3}`)})
	require.NoError(t, err)
	got, err = s.Active(ctx, "brand-a", "app-1")
	require.NoError(t, err)
	assert.Equal(t, app.ID, got.ID, "brand+app beats brand+*")

	got, err = s.Active(ctx, "brand-b", "app-1")
	require.NoError(t, err)
	assert.Equal(t, global.ID, got.ID, "other brands still get *+*")

	v2, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "base", Document: []byte(`{"n":4}`)})
	require.NoError(t, err)
	assert.Equal(t, 2, v2.Version, "next version of the same code")
	got, err = s.Active(ctx, "brand-b", "")
	require.NoError(t, err)
	assert.Equal(t, v2.ID, got.ID)
	assert.JSONEq(t, `{"n":4}`, got.Document)

	other, err := s.Upsert(ctx, profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "other", Document: []byte(`{"n":5}`)})
	require.NoError(t, err)
	assert.Equal(t, 1, other.Version)
	got, err = s.Active(ctx, "brand-b", "")
	require.NoError(t, err)
	assert.Equal(t, other.ID, got.ID, "an upsert deactivates every other profile in its scope")

	_, err = s.Upsert(ctx, profile.UpsertInput{BrandID: "", ApplicationID: "*", Code: "x", Document: []byte(`{}`)})
	assert.Error(t, err, "scope is explicit")
}

func TestMemoryStoreContract(t *testing.T) {
	storeContract(t, profile.NewMemoryStore())
}

func TestMemoryStore_IgnoresInactiveRows(t *testing.T) {
	s := profile.NewMemoryStore()
	s.Put(profile.FaceArchitectureProfile{BrandID: "*", ApplicationID: "*", Code: "a", Version: 9, Status: "inactive", Document: `{}`})
	_, err := s.Active(context.Background(), "x", "y")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)
}

// TestGormStoreContract needs a disposable Postgres database: it creates
// face_architecture_profiles there and deletes every row in it. Never point
// it at a shared database.
func TestGormStoreContract(t *testing.T) {
	url := os.Getenv("FACEARCH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FACEARCH_TEST_DATABASE_URL not set (scratch Postgres only)")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&profile.FaceArchitectureProfile{}))
	require.NoError(t, db.Exec("DELETE FROM face_architecture_profiles").Error)
	t.Cleanup(func() { db.Exec("DELETE FROM face_architecture_profiles") })
	storeContract(t, profile.NewGormStore(db))
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: profile.Scopes`, `profile.NewMemoryStore`, `profile.NewGormStore`, ...).

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 3: Implement the store interface and lookup order**

`apps/core-engine/internal/facearch/profile/store.go`:
```go
package profile

import (
	"context"
	"errors"
)

// ErrNoActiveProfile means no active profile matches brand+app, brand+*,
// or *+*. There are no built-in defaults; the handler answers 409.
var ErrNoActiveProfile = errors.New("no_active_face_architecture_profile")

// UpsertInput is a validated document and the scope to store it under.
type UpsertInput struct {
	BrandID       string
	ApplicationID string
	Code          string
	Title         string
	Document      []byte
}

// Store finds and writes profiles.
type Store interface {
	// Active returns the highest active version for the first scope in
	// Scopes(brandID, appID) that has one, or ErrNoActiveProfile.
	Active(ctx context.Context, brandID, appID string) (*FaceArchitectureProfile, error)
	// Upsert inserts the document as the next version of (brand, app,
	// code), makes it ACTIVE, and marks every other active profile in the
	// same brand+app scope INACTIVE, so each scope has one active profile.
	Upsert(ctx context.Context, in UpsertInput) (*FaceArchitectureProfile, error)
}

// Scopes is the lookup order: brand+app, brand+*, *+*. Empty values mean
// Wildcard; a scope that repeats an earlier one is left out.
func Scopes(brandID, appID string) [][2]string {
	if brandID == "" {
		brandID = Wildcard
	}
	if appID == "" {
		appID = Wildcard
	}
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, s := range [][2]string{{brandID, appID}, {brandID, Wildcard}, {Wildcard, Wildcard}} {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func normaliseInput(in UpsertInput) (UpsertInput, error) {
	if in.BrandID == "" || in.ApplicationID == "" || in.Code == "" {
		return in, errors.New("brand, application and code are required (use * for any brand or application)")
	}
	if in.Title == "" {
		in.Title = in.Code
	}
	return in, nil
}
```

- [ ] **Step 4: Implement the in-memory store (tests only)**

`apps/core-engine/internal/facearch/profile/memory_store.go`:
```go
package profile

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-process Store for tests. It is not wired into the
// server: core-engine serves profiles from Postgres only (GormStore).
type MemoryStore struct {
	mu   sync.Mutex
	rows []FaceArchitectureProfile
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Put stores a row as given (tests use it to seed inactive or competing rows).
func (s *MemoryStore) Put(p FaceArchitectureProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	p.Status = NormalizeStatus(p.Status)
	s.rows = append(s.rows, p)
}

func (s *MemoryStore) Active(_ context.Context, brandID, appID string) (*FaceArchitectureProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sc := range Scopes(brandID, appID) {
		var best *FaceArchitectureProfile
		for i := range s.rows {
			r := &s.rows[i]
			if r.BrandID != sc[0] || r.ApplicationID != sc[1] || NormalizeStatus(r.Status) != StatusActive {
				continue
			}
			if best == nil || r.Version > best.Version || (r.Version == best.Version && r.CreatedAt.After(best.CreatedAt)) {
				best = r
			}
		}
		if best != nil {
			out := *best
			return &out, nil
		}
	}
	return nil, ErrNoActiveProfile
}

func (s *MemoryStore) Upsert(_ context.Context, in UpsertInput) (*FaceArchitectureProfile, error) {
	in, err := normaliseInput(in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := 1
	now := time.Now()
	for i := range s.rows {
		r := &s.rows[i]
		if r.BrandID == in.BrandID && r.ApplicationID == in.ApplicationID {
			if r.Code == in.Code && r.Version >= next {
				next = r.Version + 1
			}
			if NormalizeStatus(r.Status) == StatusActive {
				r.Status = StatusInactive
				r.UpdatedAt = &now
			}
		}
	}
	p := FaceArchitectureProfile{
		ID: uuid.NewString(), BrandID: in.BrandID, ApplicationID: in.ApplicationID, Code: in.Code,
		Title: in.Title, Status: StatusActive, Version: next, Document: string(in.Document),
		CreatedAt: now, UpdatedAt: &now,
	}
	s.rows = append(s.rows, p)
	return &p, nil
}
```

- [ ] **Step 5: Implement the GORM store**

`apps/core-engine/internal/facearch/profile/gorm_store.go`:
```go
package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GormStore keeps profiles in face_architecture_profiles.
type GormStore struct {
	db *gorm.DB
}

// NewGormStore returns a Store over db.
func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

func (s *GormStore) Active(ctx context.Context, brandID, appID string) (*FaceArchitectureProfile, error) {
	for _, sc := range Scopes(brandID, appID) {
		var p FaceArchitectureProfile
		err := s.db.WithContext(ctx).
			Where("brand_id = ? AND application_id = ? AND UPPER(status) = ?", sc[0], sc[1], StatusActive).
			Order("version desc").Order("created_at desc").
			First(&p).Error
		if err == nil {
			return &p, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("face-architecture profile lookup: %w", err)
		}
	}
	return nil, ErrNoActiveProfile
}

func (s *GormStore) Upsert(ctx context.Context, in UpsertInput) (*FaceArchitectureProfile, error) {
	in, err := normaliseInput(in)
	if err != nil {
		return nil, err
	}
	var out FaceArchitectureProfile
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&FaceArchitectureProfile{}).
			Where("brand_id = ? AND application_id = ? AND code = ?", in.BrandID, in.ApplicationID, in.Code).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&FaceArchitectureProfile{}).
			Where("brand_id = ? AND application_id = ? AND UPPER(status) = ?", in.BrandID, in.ApplicationID, StatusActive).
			Updates(map[string]any{"status": StatusInactive, "updated_at": now}).Error; err != nil {
			return err
		}
		out = FaceArchitectureProfile{
			ID: uuid.NewString(), BrandID: in.BrandID, ApplicationID: in.ApplicationID, Code: in.Code,
			Title: in.Title, Status: StatusActive, Version: maxVersion + 1, Document: string(in.Document),
		}
		return tx.Create(&out).Error
	})
	if err != nil {
		return nil, fmt.Errorf("face-architecture profile upsert: %w", err)
	}
	return &out, nil
}
```

- [ ] **Step 6: Run; expect PASS** (`TestGormStoreContract` SKIPs without `FACEARCH_TEST_DATABASE_URL`). If you have a disposable Postgres, run it too: `FACEARCH_TEST_DATABASE_URL=postgres://... go test ./apps/core-engine/internal/facearch/profile/ -run TestGormStoreContract -v`.

Run: `go test ./apps/core-engine/internal/facearch/profile/ -v`

- [ ] **Step 7: Commit**
```bash
git add apps/core-engine/internal/facearch/profile/store.go apps/core-engine/internal/facearch/profile/memory_store.go apps/core-engine/internal/facearch/profile/gorm_store.go apps/core-engine/internal/facearch/profile/store_test.go
git commit -m "feat(core-engine): face-architecture profile lookup with brand/app fallback"
```

---

### Task 6: Prototype classifier

**Files:**
- Create: `apps/core-engine/internal/facearch/classify/classify.go`
- Test: `apps/core-engine/internal/facearch/classify/classify_test.go`

**Interfaces:**
- Consumes: `profile.ClassifierSpec`, `profile.ClassSpec.SortedKeys()` (Task 3); `measureclient.Index.Observe`, `Observation.Usable()` (Task 1).
- Produces: `const StatusSingle, StatusBlend, StatusInsufficientEvidence, StatusUnavailable`; `type Result struct{ Status, Primary, Secondary string; Scores map[string]float64; NotAssessable, MissingMeasurements []string; ObservedWeightShare *float64; Reason string }`; `type Classifier interface{ Classify(obs measureclient.Index) Result }`; `func Unavailable(reason string) Result`; `type PrototypeClassifier`; `func NewPrototype(spec profile.ClassifierSpec) *PrototypeClassifier`.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/classify/classify_test.go`:
```go
package classify_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

func f(x float64) *float64 { return &x }

// obs builds a response: a float value is usable, "occluded" is occluded.
func obs(values map[string]any) mc.Index {
	r := &mc.Response{}
	for k, v := range values {
		switch v := v.(type) {
		case float64:
			r.Measurements = append(r.Measurements, mc.Measurement{Key: k, Value: mc.Value{Scalar: f(v)}, Visibility: mc.VisibilityUnknown})
		case string:
			r.Measurements = append(r.Measurements, mc.Measurement{Key: k, Visibility: v})
		}
	}
	return r.Index()
}

func term(c, s, w float64) profile.PrototypeTerm {
	return profile.PrototypeTerm{Centre: c, Spread: s, Weight: w}
}

// Two classes on one measurement: A at 0, B at 2, spread 1.
func twoClass() profile.ClassifierSpec {
	return profile.ClassifierSpec{
		Key: "shape", BlendMargin: 0.6, MinObservedWeight: 0.5,
		Classes: []profile.ClassSpec{
			{Label: "A", Requires: []string{"m"}, Prototype: map[string]profile.PrototypeTerm{"m": term(0, 1, 1)}},
			{Label: "B", Requires: []string{"m"}, Prototype: map[string]profile.PrototypeTerm{"m": term(2, 1, 1)}},
		},
	}
}

func TestSingleWhenTopReachesMargin(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 0.0}))
	// log scores 0 and -2: A = 1 / (1 + e^-2).
	want := 1 / (1 + math.Exp(-2))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Empty(t, r.Secondary)
	assert.InDelta(t, want, r.Scores["A"], 1e-12)
	assert.InDelta(t, 1-want, r.Scores["B"], 1e-12)
	assert.Empty(t, r.NotAssessable)
	require.NotNil(t, r.ObservedWeightShare)
	assert.Equal(t, 1.0, *r.ObservedWeightShare)
}

func TestBlendBelowMarginTieKeepsSpecOrder(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 1.0}))
	assert.Equal(t, classify.StatusBlend, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Equal(t, "B", r.Secondary)
	assert.InDelta(t, 0.5, r.Scores["A"], 1e-12)
}

func TestWeightsAndSpreadsEnterTheFormula(t *testing.T) {
	spec := twoClass()
	spec.Classes[0].Prototype["m"] = term(0, 2, 3) // z = 1/2, w = 3 -> log = -3/8
	spec.Classes[1].Prototype["m"] = term(2, 1, 1) // z = -1, w = 1 -> log = -1/2
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 1.0}))
	a, b := math.Exp(-3.0/8), math.Exp(-0.5)
	assert.InDelta(t, a/(a+b), r.Scores["A"], 1e-12)
}

func TestFarOffFaceDoesNotUnderflow(t *testing.T) {
	r := classify.NewPrototype(twoClass()).Classify(obs(map[string]any{"m": 1e4}))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "B", r.Primary)
	assert.InDelta(t, 1.0, r.Scores["A"]+r.Scores["B"], 1e-12)
}

func TestRequiresOccludedMakesClassNotAssessable(t *testing.T) {
	spec := twoClass()
	spec.Classes = append(spec.Classes, profile.ClassSpec{
		Label: "C", Requires: []string{"hidden"},
		Prototype: map[string]profile.PrototypeTerm{"m": term(1, 1, 1), "hidden": term(0, 1, 1)},
	})
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0, "hidden": mc.VisibilityOccluded}))
	assert.Equal(t, []string{"C"}, r.NotAssessable)
	assert.NotContains(t, r.Scores, "C")
	assert.InDelta(t, 1.0, r.Scores["A"]+r.Scores["B"], 1e-12)
	assert.Equal(t, []string{"hidden"}, r.MissingMeasurements)
	assert.InDelta(t, 3.0/4.0, *r.ObservedWeightShare, 1e-12)
}

func TestNonRequiredMissingTermIsSkipped(t *testing.T) {
	spec := twoClass()
	spec.Classes[0].Prototype["extra"] = term(5, 1, 1)
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0}))
	assert.Equal(t, classify.StatusSingle, r.Status)
	assert.Equal(t, "A", r.Primary)
	assert.Equal(t, []string{"extra"}, r.MissingMeasurements)
}

func TestClassWithNoUsableTermIsNotAssessable(t *testing.T) {
	spec := twoClass()
	spec.Classes[1].Requires = nil
	spec.Classes[1].Prototype = map[string]profile.PrototypeTerm{"gone": term(0, 1, 1)}
	spec.MinObservedWeight = 0
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0}))
	assert.Equal(t, []string{"B"}, r.NotAssessable)
	assert.Equal(t, classify.StatusInsufficientEvidence, r.Status, "fewer than two assessable classes")
	assert.Empty(t, r.Primary)
	assert.Equal(t, map[string]float64{"A": 1}, r.Scores, "assessable scores are still reported")
}

func TestInsufficientWhenObservedShareBelowMinimum(t *testing.T) {
	spec := twoClass()
	for i := range spec.Classes {
		spec.Classes[i].Requires = nil
		spec.Classes[i].Prototype["n"] = term(0, 1, 3)
	}
	r := classify.NewPrototype(spec).Classify(obs(map[string]any{"m": 0.0, "n": mc.VisibilityOccluded}))
	assert.InDelta(t, 0.25, *r.ObservedWeightShare, 1e-12)
	assert.Equal(t, classify.StatusInsufficientEvidence, r.Status)
	assert.Equal(t, []string{"n"}, r.MissingMeasurements)
	assert.Len(t, r.Scores, 2)
}

func TestNotSupportedAndListValuesAreNotUsable(t *testing.T) {
	r := &mc.Response{Measurements: []mc.Measurement{
		{Key: "m", Visibility: mc.VisibilityNotSupported},
	}}
	res := classify.NewPrototype(twoClass()).Classify(r.Index())
	assert.Equal(t, classify.StatusInsufficientEvidence, res.Status)
	assert.Equal(t, []string{"A", "B"}, res.NotAssessable)

	r = &mc.Response{Measurements: []mc.Measurement{
		{Key: "m", Value: mc.Value{List: []float64{1, 2}}, Visibility: mc.VisibilityUnknown},
	}}
	res = classify.NewPrototype(twoClass()).Classify(r.Index())
	assert.Equal(t, []string{"A", "B"}, res.NotAssessable)
}

func TestUnavailable(t *testing.T) {
	r := classify.Unavailable("catalogue drift")
	assert.Equal(t, classify.StatusUnavailable, r.Status)
	assert.Equal(t, "catalogue drift", r.Reason)
	assert.NotNil(t, r.Scores)
	assert.NotNil(t, r.NotAssessable)
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: package `classify` does not exist).

Run: `go test ./apps/core-engine/internal/facearch/classify/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/classify/classify.go`:
```go
// Package classify turns measurements into class scores (face shape, and any
// other prototype classifier a profile defines).
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Classifiers (prototype scoring)".
package classify

import (
	"math"
	"sort"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Result statuses.
const (
	// StatusSingle: the top class's normalised score reached blendMargin.
	StatusSingle = "single"
	// StatusBlend: it did not; primary and secondary are the top two.
	StatusBlend = "blend"
	// StatusInsufficientEvidence: too little of the prototype weight was
	// observed, or fewer than two classes could be scored.
	StatusInsufficientEvidence = "insufficient_evidence"
	// StatusUnavailable: the profile no longer matches the ai-worker's
	// catalogue (set by the caller, see Unavailable).
	StatusUnavailable = "unavailable"
)

// Result is one classifier's answer. Scores always holds every assessable
// class's normalised score (they sum to 1), whatever the status.
type Result struct {
	Status              string             `json:"status"`
	Primary             string             `json:"primary,omitempty"`
	Secondary           string             `json:"secondary,omitempty"`
	Scores              map[string]float64 `json:"scores"`
	NotAssessable       []string           `json:"notAssessable"`
	MissingMeasurements []string           `json:"missingMeasurements,omitempty"`
	ObservedWeightShare *float64           `json:"observedWeightShare,omitempty"`
	Reason              string             `json:"reason,omitempty"`
}

// Classifier scores one face. PrototypeClassifier is the v1 implementation;
// a learned classifier can implement the same interface later.
type Classifier interface {
	Classify(obs measureclient.Index) Result
}

// Unavailable is the result for a classifier the caller cannot run.
func Unavailable(reason string) Result {
	return Result{Status: StatusUnavailable, Scores: map[string]float64{}, NotAssessable: []string{}, Reason: reason}
}

// PrototypeClassifier scores each class by distance to its prototype:
//
//	score_c = exp(-1/2 * sum_m w_m * ((x_m - centre_m) / spread_m)^2)
//
// over the class's usable measurements, normalised across assessable
// classes. "Usable" means the worker returned a float: visibility
// "observed" or "unknown" (v1 feature regions are always "unknown").
//
// A class is not assessable when a "requires" measurement is not usable, or
// when none of its prototype measurements is usable (a score over nothing
// would be exp(0) = 1, a fabricated certainty).
type PrototypeClassifier struct {
	spec profile.ClassifierSpec
}

// NewPrototype returns a classifier for a validated spec.
func NewPrototype(spec profile.ClassifierSpec) *PrototypeClassifier {
	return &PrototypeClassifier{spec: spec}
}

type scored struct {
	label string
	order int
	log   float64
}

func (p *PrototypeClassifier) Classify(obs measureclient.Index) Result {
	res := Result{Scores: map[string]float64{}, NotAssessable: []string{}}
	missing := map[string]bool{}
	var totalW, observedW float64
	var ok []scored

	for i, cl := range p.spec.Classes {
		assessable := true
		for _, k := range cl.Requires {
			if !obs.Observe(k).Usable() {
				missing[k] = true
				assessable = false
			}
		}
		var logScore float64
		used := 0
		for _, k := range cl.SortedKeys() {
			term := cl.Prototype[k]
			totalW += term.Weight
			o := obs.Observe(k)
			if !o.Usable() {
				missing[k] = true
				continue
			}
			observedW += term.Weight
			z := (o.Value - term.Centre) / term.Spread
			logScore -= 0.5 * term.Weight * z * z
			used++
		}
		if !assessable || used == 0 {
			res.NotAssessable = append(res.NotAssessable, cl.Label)
			continue
		}
		ok = append(ok, scored{label: cl.Label, order: i, log: logScore})
	}

	// Normalise in log space so far-off faces do not underflow to 0/0.
	if len(ok) > 0 {
		maxLog := math.Inf(-1)
		for _, s := range ok {
			maxLog = math.Max(maxLog, s.log)
		}
		var sum float64
		for _, s := range ok {
			sum += math.Exp(s.log - maxLog)
		}
		for _, s := range ok {
			res.Scores[s.label] = math.Exp(s.log-maxLog) / sum
		}
	}
	if len(missing) > 0 {
		res.MissingMeasurements = make([]string, 0, len(missing))
		for k := range missing {
			res.MissingMeasurements = append(res.MissingMeasurements, k)
		}
		sort.Strings(res.MissingMeasurements)
	}
	share := 0.0
	if totalW > 0 {
		share = observedW / totalW
	}
	res.ObservedWeightShare = &share

	if share < p.spec.MinObservedWeight || len(ok) < 2 {
		res.Status = StatusInsufficientEvidence
		return res
	}
	sort.SliceStable(ok, func(i, j int) bool {
		si, sj := res.Scores[ok[i].label], res.Scores[ok[j].label]
		if si != sj {
			return si > sj
		}
		return ok[i].order < ok[j].order
	})
	res.Primary = ok[0].label
	if res.Scores[ok[0].label] < p.spec.BlendMargin {
		res.Status = StatusBlend
		res.Secondary = ok[1].label
		return res
	}
	res.Status = StatusSingle
	return res
}
```

- [ ] **Step 4: Run; expect PASS** (10 tests).

Run: `go test ./apps/core-engine/internal/facearch/classify/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/classify
git commit -m "feat(core-engine): prototype face-shape classifier with blend and evidence rules"
```

---

### Task 7: Trait rule lists

**Files:**
- Create: `apps/core-engine/internal/facearch/traits/traits.go`
- Test: `apps/core-engine/internal/facearch/traits/traits_test.go`

**Interfaces:**
- Consumes: `profile.TraitSpec`, `profile.Condition.Holds`, `TraitSpec.Keys()` (Task 3); `measureclient.Index.Observe` (Task 1).
- Produces: `const StatusAssessed, StatusNotAssessable, StatusUnavailable`; `type Result struct{ Status, Label string; BoundaryUncertain bool; Alternative string; MissingMeasurements []string; Reason string }`; `func Evaluate(spec profile.TraitSpec, obs measureclient.Index) Result`; `func Unavailable(reason string) Result`.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/internal/facearch/traits/traits_test.go`:
```go
package traits_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mc "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

func f(x float64) *float64 { return &x }

func m(key string, value, lo, hi float64) mc.Measurement {
	return mc.Measurement{
		Key: key, Visibility: mc.VisibilityUnknown,
		Value: mc.Value{Scalar: f(value)},
		Band:  &[2]mc.Value{{Scalar: f(lo)}, {Scalar: f(hi)}},
	}
}

func index(ms ...mc.Measurement) mc.Index {
	return (&mc.Response{Measurements: ms}).Index()
}

func cond(key, op string, v float64) profile.Condition {
	return profile.Condition{Measurement: key, Op: op, Value: v}
}

// noseWidth-shaped trait: Broad above 1.1, Narrow below 0.9, else Average.
func width() profile.TraitSpec {
	return profile.TraitSpec{
		Key: "noseWidth", Default: "Average",
		Rules: []profile.RuleSpec{
			{Label: "Broad", When: []profile.Condition{cond("w", ">", 1.1)}},
			{Label: "Narrow", When: []profile.Condition{cond("w", "<", 0.9)}},
		},
	}
}

func TestFirstMatchWinsAndDefault(t *testing.T) {
	assert.Equal(t, "Broad", traits.Evaluate(width(), index(m("w", 1.3, 1.25, 1.35))).Label)
	assert.Equal(t, "Narrow", traits.Evaluate(width(), index(m("w", 0.8, 0.75, 0.85))).Label)
	r := traits.Evaluate(width(), index(m("w", 1.0, 0.95, 1.05)))
	assert.Equal(t, traits.StatusAssessed, r.Status)
	assert.Equal(t, "Average", r.Label)
	assert.False(t, r.BoundaryUncertain)
	assert.Empty(t, r.Alternative)
}

func TestRuleOrderMatters(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "First", When: []profile.Condition{cond("w", ">", 0)}},
		{Label: "Second", When: []profile.Condition{cond("w", ">", 1)}},
	}}
	assert.Equal(t, "First", traits.Evaluate(spec, index(m("w", 2, 1.9, 2.1))).Label)
}

func TestAllConditionsMustHold(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "Both", When: []profile.Condition{cond("a", ">", 1), cond("b", "<", 0)}},
	}}
	assert.Equal(t, "D", traits.Evaluate(spec, index(m("a", 2, 1.9, 2.1), m("b", 1, 0.9, 1.1))).Label)
	assert.Equal(t, "Both", traits.Evaluate(spec, index(m("a", 2, 1.9, 2.1), m("b", -1, -1.1, -0.9))).Label)
}

func TestBandStraddlingThresholdGivesAlternative(t *testing.T) {
	r := traits.Evaluate(width(), index(m("w", 1.15, 1.0, 1.3)))
	assert.Equal(t, "Broad", r.Label)
	assert.True(t, r.BoundaryUncertain)
	assert.Equal(t, "Average", r.Alternative)

	r = traits.Evaluate(width(), index(m("w", 1.0, 0.85, 1.05)))
	assert.Equal(t, "Average", r.Label)
	assert.True(t, r.BoundaryUncertain)
	assert.Equal(t, "Narrow", r.Alternative)
}

func TestStraddleThatCannotChangeTheLabelIsIgnored(t *testing.T) {
	spec := profile.TraitSpec{Key: "k", Default: "D", Rules: []profile.RuleSpec{
		{Label: "A", When: []profile.Condition{cond("a", ">", 1)}},
		{Label: "B", When: []profile.Condition{cond("b", ">", 0), cond("c", ">", 5)}},
	}}
	// a is clearly true; b straddles but rule B is never reached.
	r := traits.Evaluate(spec, index(m("a", 3, 2.9, 3.1), m("b", 0.1, -0.1, 0.2), m("c", 9, 8, 10)))
	assert.Equal(t, "A", r.Label)
	assert.False(t, r.BoundaryUncertain)
}

// The worker sends "band": null when bands are disabled or too few perturbed
// samples were finite. That is no uncertainty information: the trait is
// assessed from the value, never flagged, and nothing panics.
func TestNullBandMeansNoUncertaintyInformation(t *testing.T) {
	r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityObserved, Value: mc.Value{Scalar: f(1.1)}}))
	assert.Equal(t, traits.StatusAssessed, r.Status)
	assert.Equal(t, "Average", r.Label)
	assert.False(t, r.BoundaryUncertain)
	assert.Empty(t, r.Alternative)

	r = traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityUnknown, Value: mc.Value{Scalar: f(1.3)}}))
	assert.Equal(t, "Broad", r.Label)
	assert.False(t, r.BoundaryUncertain)
}

// The worker's "degenerate geometry" (value null, visibility unknown) is
// not usable: the trait cannot be assessed.
func TestDegenerateGeometryIsNotAssessable(t *testing.T) {
	reason := "degenerate geometry"
	r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: mc.VisibilityUnknown, Reason: &reason}))
	assert.Equal(t, traits.StatusNotAssessable, r.Status)
	assert.Equal(t, []string{"w"}, r.MissingMeasurements)
}

func TestOccludedOrUnsupportedConditionIsNotAssessable(t *testing.T) {
	for _, vis := range []string{mc.VisibilityOccluded, mc.VisibilityNotSupported} {
		r := traits.Evaluate(width(), index(mc.Measurement{Key: "w", Visibility: vis}))
		assert.Equal(t, traits.StatusNotAssessable, r.Status, vis)
		assert.Empty(t, r.Label)
		assert.Equal(t, []string{"w"}, r.MissingMeasurements)
		assert.Equal(t, "requires w", r.Reason)
	}
}

func TestLaterRuleOnHiddenMeasurementStillBlocks(t *testing.T) {
	spec := width()
	spec.Rules = append(spec.Rules, profile.RuleSpec{Label: "X", When: []profile.Condition{cond("hidden", ">", 0)}})
	r := traits.Evaluate(spec, index(m("w", 1.3, 1.25, 1.35), mc.Measurement{Key: "hidden", Visibility: mc.VisibilityOccluded}))
	assert.Equal(t, traits.StatusNotAssessable, r.Status)
}

func TestUnavailable(t *testing.T) {
	r := traits.Unavailable("drift")
	assert.Equal(t, traits.StatusUnavailable, r.Status)
	assert.Equal(t, "drift", r.Reason)
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: package `traits` does not exist).

Run: `go test ./apps/core-engine/internal/facearch/traits/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/traits/traits.go`:
```go
// Package traits evaluates a profile's ordered trait rule lists.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Semantics → Traits".
package traits

import (
	"sort"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// Result statuses.
const (
	StatusAssessed      = "assessed"
	StatusNotAssessable = "not_assessable"
	StatusUnavailable   = "unavailable"
)

// Result is one trait's answer.
type Result struct {
	Status              string   `json:"status"`
	Label               string   `json:"label,omitempty"`
	BoundaryUncertain   bool     `json:"boundaryUncertain"`
	Alternative         string   `json:"alternative,omitempty"`
	MissingMeasurements []string `json:"missingMeasurements,omitempty"`
	Reason              string   `json:"reason,omitempty"`
}

// Unavailable is the result for a trait the caller cannot run.
func Unavailable(reason string) Result {
	return Result{Status: StatusUnavailable, Reason: reason}
}

// Evaluate runs the rules in order; the first whose conditions all hold
// wins, otherwise the default label.
//
// Any condition, in any rule, on a measurement that is not usable (occluded,
// not supported, null) makes the trait not_assessable: the default is only
// meaningful when every earlier rule could be tested.
//
// A condition whose noise band straddles its threshold (lo <= value <= hi)
// could have gone the other way. Conditions are tried in rule order; the
// first one whose flip changes the winning label sets boundaryUncertain and
// alternative to that label. The worker's bands are value-centred and
// conservative (independent noise at the model card's worst regional
// error), so boundaryUncertain is expected often. A measurement without a
// band (null: bands disabled or too few finite samples) carries no
// uncertainty information, so its conditions never straddle.
func Evaluate(spec profile.TraitSpec, obs measureclient.Index) Result {
	keys := spec.Keys()
	values := make(map[string]measureclient.Observation, len(keys))
	var missing []string
	for _, k := range keys {
		o := obs.Observe(k)
		if !o.Usable() {
			missing = append(missing, k)
			continue
		}
		values[k] = o
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Result{
			Status:              StatusNotAssessable,
			MissingMeasurements: missing,
			Reason:              "requires " + strings.Join(missing, ", "),
		}
	}

	truth := make([][]bool, len(spec.Rules))
	for i, r := range spec.Rules {
		truth[i] = make([]bool, len(r.When))
		for j, c := range r.When {
			truth[i][j] = c.Holds(values[c.Measurement].Value)
		}
	}
	res := Result{Status: StatusAssessed, Label: pick(spec, truth)}

	for i, r := range spec.Rules {
		for j, c := range r.When {
			band := values[c.Measurement].Band
			if band == nil || !(band[0] <= c.Value && c.Value <= band[1]) {
				continue
			}
			truth[i][j] = !truth[i][j]
			alt := pick(spec, truth)
			truth[i][j] = !truth[i][j]
			if alt != res.Label {
				res.BoundaryUncertain = true
				res.Alternative = alt
				return res
			}
		}
	}
	return res
}

func pick(spec profile.TraitSpec, truth [][]bool) string {
	for i, r := range spec.Rules {
		all := true
		for j := range r.When {
			if !truth[i][j] {
				all = false
				break
			}
		}
		if all {
			return r.Label
		}
	}
	return spec.Default
}
```

- [ ] **Step 4: Run; expect PASS** (10 tests).

Run: `go test ./apps/core-engine/internal/facearch/traits/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/traits
git commit -m "feat(core-engine): ordered trait rules with boundary uncertainty"
```

---

### Task 8: Interpretation (profile × measurements → response)

**Files:**
- Create: `apps/core-engine/internal/facearch/interpret.go`
- Test: `apps/core-engine/internal/facearch/interpret_test.go`

**Interfaces:**
- Consumes: Tasks 1, 3, 4, 6, 7.
- Produces: `type ProfileRef struct{ ID, Code string; Version int; CatalogueVersion string }`; `type CalibrationRef struct{ Status string; BenchmarkRunID, Target *string }`; `type Provenance struct{ Model measureclient.Model; CatalogueVersion string; Profile ProfileRef; Calibration CalibrationRef }`; `type Response struct{ Quality measureclient.Quality; Regions map[string]string; Measurements []measureclient.Measurement; MeasurementsMissing []string; Classifications map[string]classify.Result; Traits map[string]traits.Result; Guidance json.RawMessage; Provenance Provenance }`; `func Interpret(p *profile.FaceArchitectureProfile, doc *profile.Document, m *measureclient.Response) *Response`. Plan 4's benchmark calls `Interpret` in-process against a profile file.

- [ ] **Step 1: Write the failing test** (`testDoc` is reused by Task 9's handler test in the same package)

`apps/core-engine/internal/facearch/interpret_test.go`:
```go
package facearch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// testDoc's numbers are test inputs, not proposed values.
const testDoc = `{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"},
      {"cite": "test", "for": "traits.noseWidth"}
    ]},
  "measurementsShown": ["eye_width", "fifths", "gone_key"],
  "classifiers": [{
    "key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5,
    "classes": [
      {"label": "Long", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
      {"label": "Wide", "requires": ["width_length_ratio"], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}
    ]
  }],
  "traits": [{
    "key": "noseWidth",
    "rules": [
      {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]},
      {"label": "Narrow", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": "<", "value": 0.9}]}
    ],
    "default": "Average"
  }]
}`

func testProfile(t *testing.T) (*profile.FaceArchitectureProfile, *profile.Document) {
	t.Helper()
	p := &profile.FaceArchitectureProfile{ID: "p-1", Code: "test", Version: 3, Document: testDoc}
	d, err := profile.Load(p)
	require.NoError(t, err)
	return p, d
}

func TestInterpret_FixtureFace(t *testing.T) {
	p, d := testProfile(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())

	keys := []string{}
	for _, m := range out.Measurements {
		keys = append(keys, m.Key)
	}
	assert.Equal(t, []string{"fifths", "eye_width"}, keys, "worker order, shown keys only")
	assert.Equal(t, []string{"gone_key"}, out.MeasurementsMissing)

	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status, "the fixture's chin is out of frame")
	assert.Equal(t, []string{"Long", "Wide"}, fs.NotAssessable)
	assert.Equal(t, []string{"width_length_ratio"}, fs.MissingMeasurements)

	nw := out.Traits["noseWidth"]
	assert.Equal(t, traits.StatusAssessed, nw.Status)
	assert.Equal(t, "Broad", nw.Label) // 1.1773, band 0.9439..1.3768
	assert.True(t, nw.BoundaryUncertain)
	assert.Equal(t, "Average", nw.Alternative)

	assert.Equal(t, "occluded", out.Regions["jaw"])
	assert.Equal(t, "fa-measure/1", out.Provenance.CatalogueVersion)
	assert.Equal(t, facearch.ProfileRef{ID: "p-1", Code: "test", Version: 3, CatalogueVersion: "fa-measure/1"}, out.Provenance.Profile)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	assert.Equal(t, "face_landmarker", out.Provenance.Model.Name)
}

func TestInterpret_JSONShape(t *testing.T) {
	p, d := testProfile(t)
	b, err := json.Marshal(facearch.Interpret(p, d, facearchtest.FixtureFace()))
	require.NoError(t, err)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &body))
	assert.Equal(t, "null", string(body["guidance"]), "guidance is null until Plan 3")
	for _, k := range []string{"quality", "regions", "measurements", "classifications", "traits", "provenance"} {
		assert.Contains(t, body, k)
	}
	assert.JSONEq(t, `{"status":"uncalibrated"}`, string(mustField(t, body["provenance"], "calibration")))
}

func mustField(t *testing.T, raw json.RawMessage, key string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	return m[key]
}

func TestInterpret_CatalogueVersionDriftMarksEverythingUnavailable(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	m.CatalogueVersion = "fa-measure/2"
	out := facearch.Interpret(p, d, m)
	assert.Equal(t, classify.StatusUnavailable, out.Classifications["faceShape"].Status)
	assert.Contains(t, out.Classifications["faceShape"].Reason, "profile targets fa-measure/1, ai-worker measured with fa-measure/2")
	assert.Equal(t, traits.StatusUnavailable, out.Traits["noseWidth"].Status)
	assert.Equal(t, "fa-measure/2", out.Provenance.CatalogueVersion)
}

func TestInterpret_KeyDriftIsPerClassifierOrTrait(t *testing.T) {
	p, d := testProfile(t)
	d.Traits = append(d.Traits,
		profile.TraitSpec{Key: "lid", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "eyelid_type", Op: ">", Value: 0}}}}},
		profile.TraitSpec{Key: "gone", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "removed_key", Op: ">", Value: 0}}}}},
		profile.TraitSpec{Key: "list", Default: "x", Rules: []profile.RuleSpec{{Label: "y", When: []profile.Condition{{Measurement: "fifths", Op: ">", Value: 0}}}}},
	)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())
	assert.Equal(t, traits.StatusAssessed, out.Traits["noseWidth"].Status)
	assert.Contains(t, out.Traits["lid"].Reason, "eyelid_type is not supported")
	assert.Contains(t, out.Traits["gone"].Reason, "removed_key is not in fa-measure/1")
	assert.Contains(t, out.Traits["list"].Reason, "fifths is list-valued")
}

// With the eyes occluded the worker withholds every supported measurement
// (the canonical frame is built from the eye corners). That is occlusion,
// not drift: classifiers lack evidence and traits are not assessable.
func TestInterpret_EyesOccludedWithholdsEverything(t *testing.T) {
	p, d := testProfile(t)
	m := facearchtest.FixtureFace()
	m.Regions["eyes"] = "occluded"
	reason := "canonical frame requires region eyes"
	for i := range m.Measurements {
		if m.Measurements[i].Visibility == measureclient.VisibilityNotSupported {
			continue
		}
		m.Measurements[i].Value = measureclient.Value{}
		m.Measurements[i].Band = nil
		m.Measurements[i].Visibility = measureclient.VisibilityOccluded
		m.Measurements[i].Reason = &reason
	}
	out := facearch.Interpret(p, d, m)
	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status)
	assert.Equal(t, []string{"Long", "Wide"}, fs.NotAssessable)
	nw := out.Traits["noseWidth"]
	assert.Equal(t, traits.StatusNotAssessable, nw.Status)
	assert.Equal(t, "requires alar_width_to_intercanthal_ratio", nw.Reason)
	for _, meas := range out.Measurements {
		assert.True(t, meas.Value.IsNull(), meas.Key)
	}
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: package `facearch` does not exist).

Run: `go test ./apps/core-engine/internal/facearch/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/internal/facearch/interpret.go`:
```go
// Package facearch is the face-architecture interpretation layer: it asks
// the ai-worker for measurements and applies a brand's profile to them.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md.
package facearch

import (
	"encoding/json"
	"fmt"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

// ProfileRef identifies the profile that produced a response.
type ProfileRef struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Version          int    `json:"version"`
	CatalogueVersion string `json:"catalogueVersion"`
}

// CalibrationRef repeats the profile's calibration status on every response.
type CalibrationRef struct {
	Status         string  `json:"status"`
	BenchmarkRunID *string `json:"benchmarkRunId,omitempty"`
	Target         *string `json:"target,omitempty"`
}

// Provenance says what produced the answer.
type Provenance struct {
	Model            measureclient.Model `json:"model"`
	CatalogueVersion string              `json:"catalogueVersion"`
	Profile          ProfileRef          `json:"profile"`
	Calibration      CalibrationRef      `json:"calibration"`
}

// Response is the body of POST /api/v1/vision/face-architecture/:brandId/:applicationId.
type Response struct {
	Quality measureclient.Quality `json:"quality"`
	Regions map[string]string     `json:"regions"`
	// Measurements are the profile's measurementsShown, as the ai-worker
	// returned them, in the worker's order.
	Measurements []measureclient.Measurement `json:"measurements"`
	// MeasurementsMissing lists measurementsShown keys the worker did not
	// return (catalogue drift); omitted when empty.
	MeasurementsMissing []string                   `json:"measurementsMissing,omitempty"`
	Classifications     map[string]classify.Result `json:"classifications"`
	Traits              map[string]traits.Result   `json:"traits"`
	// Guidance is filled by Plan 3 (guidance templates and rules). Until
	// then it is always JSON null: no polygons are invented.
	Guidance   json.RawMessage `json:"guidance"`
	Provenance Provenance      `json:"provenance"`
}

// Interpret applies a loaded profile document to one measure response.
// It never fails: drift between the profile and the worker's catalogue
// marks the affected classifier or trait "unavailable" with a reason.
func Interpret(p *profile.FaceArchitectureProfile, doc *profile.Document, m *measureclient.Response) *Response {
	idx := m.Index()
	out := &Response{
		Quality:         m.Quality,
		Regions:         m.Regions,
		Measurements:    []measureclient.Measurement{},
		Classifications: map[string]classify.Result{},
		Traits:          map[string]traits.Result{},
		Provenance: Provenance{
			Model:            m.Model,
			CatalogueVersion: m.CatalogueVersion,
			Profile:          ProfileRef{ID: p.ID, Code: p.Code, Version: p.Version, CatalogueVersion: doc.CatalogueVersion},
			Calibration: CalibrationRef{
				Status:         doc.Calibration.Status,
				BenchmarkRunID: doc.Calibration.BenchmarkRunID,
				Target:         doc.Calibration.Target,
			},
		},
	}

	shown := make(map[string]bool, len(doc.MeasurementsShown))
	for _, k := range doc.MeasurementsShown {
		shown[k] = true
	}
	returned := map[string]bool{}
	for _, meas := range m.Measurements {
		returned[meas.Key] = true
		if shown[meas.Key] {
			out.Measurements = append(out.Measurements, meas)
		}
	}
	for _, k := range doc.MeasurementsShown {
		if !returned[k] {
			out.MeasurementsMissing = append(out.MeasurementsMissing, k)
		}
	}

	for _, spec := range doc.Classifiers {
		if reason := drift(doc, m, idx, spec.Keys()); reason != "" {
			out.Classifications[spec.Key] = classify.Unavailable(reason)
			continue
		}
		out.Classifications[spec.Key] = classify.NewPrototype(spec).Classify(idx)
	}
	for _, spec := range doc.Traits {
		if reason := drift(doc, m, idx, spec.Keys()); reason != "" {
			out.Traits[spec.Key] = traits.Unavailable(reason)
			continue
		}
		out.Traits[spec.Key] = traits.Evaluate(spec, idx)
	}
	return out
}

// drift explains why keys cannot be used with this response, or returns "".
// Occlusion is not drift: an occluded key is present with visibility
// "occluded" and handled by the classifier or trait itself.
func drift(doc *profile.Document, m *measureclient.Response, idx measureclient.Index, keys []string) string {
	if doc.CatalogueVersion != m.CatalogueVersion {
		return fmt.Sprintf("catalogue drift: profile targets %s, ai-worker measured with %s", doc.CatalogueVersion, m.CatalogueVersion)
	}
	for _, k := range keys {
		switch idx.Observe(k).State {
		case measureclient.StateMissing:
			return fmt.Sprintf("catalogue drift: measurement %s is not in %s", k, m.CatalogueVersion)
		case measureclient.StateNotSupported:
			return fmt.Sprintf("catalogue drift: measurement %s is not supported in %s", k, m.CatalogueVersion)
		case measureclient.StateNotScalar:
			return fmt.Sprintf("catalogue drift: measurement %s is list-valued in %s", k, m.CatalogueVersion)
		}
	}
	return ""
}
```

- [ ] **Step 4: Run; expect PASS** (5 tests).

Run: `go test ./apps/core-engine/internal/facearch/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/internal/facearch/interpret.go apps/core-engine/internal/facearch/interpret_test.go
git commit -m "feat(core-engine): interpret face measurements with a brand profile"
```

---

### Task 9: Timeout setting, HTTP handler and server wiring

**Before starting:** confirm the `FACE_MEASURE_TIMEOUT` default (20 s) with the user (Open literals).

**Files:**
- Modify: `apps/core-engine/internal/config/config.go`
- Create: `apps/core-engine/internal/config/config_facemeasure_test.go`
- Create: `apps/core-engine/internal/facearch/handler.go`
- Test: `apps/core-engine/internal/facearch/handler_test.go`
- Modify: `apps/core-engine/cmd/server/main.go`

**Interfaces:**
- Consumes: Tasks 2, 4, 5, 8.
- Produces: `config.DefaultFaceMeasureTimeout`, `Config.FaceMeasureTimeout` (env `FACE_MEASURE_TIMEOUT`); `type Measurer interface{ Measure(ctx context.Context, image []byte, filename string) (*measureclient.Response, error) }`; `type ProfileFinder interface{ Active(ctx context.Context, brandID, appID string) (*profile.FaceArchitectureProfile, error) }`; `const CodeMissingImage, CodeNoActiveProfile, CodeProfileLookupFailed, CodeProfileDocumentInvalid, CodeFaceMeasureUnreachable`; `type Handler`; `func NewHandler(m Measurer, p ProfileFinder) *Handler`; `func (h *Handler) Analyze(c echo.Context) error`; route `POST /api/v1/vision/face-architecture/:brandId/:applicationId` (also under `/core/vision-engine` and `/api/vision`).

- [ ] **Step 1: Write the failing config test**

`apps/core-engine/internal/config/config_facemeasure_test.go`:
```go
package config_test

import (
	"testing"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/config"
)

func TestFromEnv_FaceMeasureTimeout(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("FACE_MEASURE_TIMEOUT", "")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FaceMeasureTimeout != config.DefaultFaceMeasureTimeout || config.DefaultFaceMeasureTimeout != 20*time.Second {
		t.Errorf("FaceMeasureTimeout = %v, want 20s default", cfg.FaceMeasureTimeout)
	}

	t.Setenv("FACE_MEASURE_TIMEOUT", "45s")
	if cfg, err = config.FromEnv(); err != nil || cfg.FaceMeasureTimeout != 45*time.Second {
		t.Errorf("override: got %v, %v", cfg, err)
	}

	t.Setenv("FACE_MEASURE_TIMEOUT", "0s")
	if _, err := config.FromEnv(); err == nil {
		t.Error("want error for FACE_MEASURE_TIMEOUT=0s")
	}
}
```

- [ ] **Step 2: Run; expect FAIL** (`undefined: config.DefaultFaceMeasureTimeout`).

Run: `go test ./apps/core-engine/internal/config/ -v`

- [ ] **Step 3: Add the setting.** In `apps/core-engine/internal/config/config.go`:

After the closing `)` of the existing `const (...)` block (the line after `DefaultParadevGatewayURL   = "http://127.0.0.1:9080/paradev/scoring"`), add a separate block — the existing block's comment says its values are moved pre-standardization literals, which this is not:
```go

// DefaultFaceMeasureTimeout bounds one synchronous ai-worker face-measure
// call (facearch/measureclient): landmarker plus FACE_MEASURE_NOISE_SAMPLES
// re-measurements. Unlike the defaults above it is new, not a moved
// literal, and no latency has been measured yet: it is the same bound as
// DefaultVisionDispatchTimeout, the other synchronous ai-worker model call,
// until benchmark timings replace it.
const DefaultFaceMeasureTimeout = 20 * time.Second
```

In `type Config struct`, after `SegmenterTimeout       time.Duration` add:
```go
	FaceMeasureTimeout     time.Duration
```

In `FromEnv`'s duration table, after `{"SEGMENTER_TIMEOUT", DefaultSegmenterTimeout, &cfg.SegmenterTimeout},` add:
```go
		{"FACE_MEASURE_TIMEOUT", DefaultFaceMeasureTimeout, &cfg.FaceMeasureTimeout},
```

- [ ] **Step 4: Run; expect PASS.**

Run: `go test ./apps/core-engine/internal/config/ -v`

- [ ] **Step 5: Write the failing handler test**

`apps/core-engine/internal/facearch/handler_test.go`:
```go
package facearch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

type fakeMeasurer struct {
	resp  *measureclient.Response
	err   error
	calls int
	got   []byte
}

func (f *fakeMeasurer) Measure(_ context.Context, image []byte, _ string) (*measureclient.Response, error) {
	f.calls++
	f.got = image
	return f.resp, f.err
}

type failingFinder struct{}

func (failingFinder) Active(context.Context, string, string) (*profile.FaceArchitectureProfile, error) {
	return nil, errors.New("connection refused")
}

const route = "/api/v1/vision/face-architecture/:brandId/:applicationId"

func serve(t *testing.T, h *facearch.Handler, withImage bool) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if withImage {
		part, err := w.CreateFormFile("image", "face.jpg")
		require.NoError(t, err)
		_, _ = part.Write([]byte("jpeg-bytes"))
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vision/face-architecture/wardah/app-1", body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.POST(route, h.Analyze)
	e.ServeHTTP(rec, req)
	return rec
}

func storeWithTestDoc(t *testing.T) *profile.MemoryStore {
	t.Helper()
	s := profile.NewMemoryStore()
	_, err := s.Upsert(context.Background(), profile.UpsertInput{BrandID: "*", ApplicationID: "*", Code: "test", Document: []byte(testDoc)})
	require.NoError(t, err)
	return s
}

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Detail struct {
			Code string `json:"code"`
		} `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b), rec.Body.String())
	return b.Detail.Code
}

func TestAnalyze_OK(t *testing.T) {
	m := &fakeMeasurer{resp: facearchtest.FixtureFace()}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []byte("jpeg-bytes"), m.got)

	var out facearch.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "test", out.Provenance.Profile.Code)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	assert.Equal(t, "Broad", out.Traits["noseWidth"].Label)
}

func TestAnalyze_MissingImage(t *testing.T) {
	m := &fakeMeasurer{}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), false)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, facearch.CodeMissingImage, code(t, rec))
	assert.Zero(t, m.calls)
}

func TestAnalyze_NoActiveProfileIs409WithoutCallingWorker(t *testing.T) {
	m := &fakeMeasurer{resp: facearchtest.FixtureFace()}
	rec := serve(t, facearch.NewHandler(m, profile.NewMemoryStore()), true)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, facearch.CodeNoActiveProfile, code(t, rec))
	assert.Contains(t, rec.Body.String(), `"brandId":"wardah"`)
	assert.Zero(t, m.calls)
}

func TestAnalyze_PassesWorkerErrorsThrough(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusServiceUnavailable} {
		body := fmt.Sprintf(`{"detail":{"code":"from_worker_%d"}}`, status)
		m := &fakeMeasurer{err: &measureclient.UpstreamError{Status: status, Body: []byte(body)}}
		rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
		assert.Equal(t, status, rec.Code)
		assert.JSONEq(t, body, rec.Body.String())
	}
}

func TestAnalyze_UnreachableWorkerIs503(t *testing.T) {
	m := &fakeMeasurer{err: fmt.Errorf("%w: dial tcp: refused", measureclient.ErrUnavailable)}
	rec := serve(t, facearch.NewHandler(m, storeWithTestDoc(t)), true)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, facearch.CodeFaceMeasureUnreachable, code(t, rec))
}

func TestAnalyze_ProfileFailures(t *testing.T) {
	rec := serve(t, facearch.NewHandler(&fakeMeasurer{}, failingFinder{}), true)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, facearch.CodeProfileLookupFailed, code(t, rec))
	assert.NotContains(t, rec.Body.String(), "connection refused", "database errors are logged, not returned")

	s := profile.NewMemoryStore()
	s.Put(profile.FaceArchitectureProfile{BrandID: "*", ApplicationID: "*", Code: "bad", Version: 1, Status: "ACTIVE", Document: `{"schemaVersion": 9}`})
	m := &fakeMeasurer{}
	rec = serve(t, facearch.NewHandler(m, s), true)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, facearch.CodeProfileDocumentInvalid, code(t, rec))
	assert.Zero(t, m.calls)
}
```

- [ ] **Step 6: Run; expect FAIL** (build failure: `undefined: facearch.NewHandler`, `facearch.CodeMissingImage`, ...).

Run: `go test ./apps/core-engine/internal/facearch/ -v`

- [ ] **Step 7: Implement the handler**

`apps/core-engine/internal/facearch/handler.go`:
```go
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
```

- [ ] **Step 8: Run; expect PASS** (11 tests in the package).

Run: `go test ./apps/core-engine/internal/facearch/ -v`

- [ ] **Step 9: Wire the server.** In `apps/core-engine/cmd/server/main.go` make exactly these four edits.

Import block — after the `visionWorkers ".../internal/vision/workers"` line, before the closing `)`:
```go

	// Face Architecture
	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	faceMeasure "github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	faceProfile "github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
```

AutoMigrate list — after `&matchEntity.ShadeAsset{},`:
```go
		&faceProfile.FaceArchitectureProfile{},
```

After `visionHandler := visionControllers.NewVisionController(visionOrch, visionReferenceCatalog)`:
```go

	// Face Architecture (measurements from the ai-worker, per-brand profile)
	faceArchHandler := facearch.NewHandler(
		faceMeasure.New(cfg.VisionAIWorkerURL, cfg.FaceMeasureTimeout),
		faceProfile.NewGormStore(db),
	)
```

Inside `mountVisionRoutes`, after `g.GET("/registry", visionHandler.GetRegistry)`:
```go
		g.POST("/face-architecture/:brandId/:applicationId", faceArchHandler.Analyze)
```

- [ ] **Step 10: Build, vet and run the whole core-engine suite.**

Run: `go build ./apps/core-engine/... && go vet ./apps/core-engine/cmd/server/ ./apps/core-engine/internal/facearch/... ./apps/core-engine/internal/config/ && go test ./apps/core-engine/...`
Expected: build succeeds, vet silent, every package `ok`.

- [ ] **Step 11: Commit**
```bash
git add apps/core-engine/internal/config/config.go apps/core-engine/internal/config/config_facemeasure_test.go apps/core-engine/internal/facearch/handler.go apps/core-engine/internal/facearch/handler_test.go apps/core-engine/cmd/server/main.go
git commit -m "feat(core-engine): POST /api/v1/vision/face-architecture endpoint"
```

---

### Task 10: facearch-profile CLI

**Files:**
- Create: `apps/core-engine/cmd/facearch-profile/main.go`
- Test: `apps/core-engine/cmd/facearch-profile/main_test.go`

**Interfaces:**
- Consumes: `measureclient.New(...).Catalogue` (Task 2), `profile.Parse/Validate/Store/NewGormStore/UpsertInput` (Tasks 3-5), `config.Load`, `config.DefaultVisionAIWorkerURL`, `config.DefaultFaceMeasureTimeout` (Task 9), `goplatform.OpenPostgres/First/Duration/LoadDotEnv`.
- Produces: binary `facearch-profile` with `validate <file.json>` and `upsert <file.json> --brand --app --code [--title]`; exit 0 ok, 1 invalid/failed, 2 usage. Testable core `func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int` with `type deps struct{ catalogue func(ctx context.Context) (*measureclient.Catalogue, error); store func(ctx context.Context) (profile.Store, func(), error) }`.

- [ ] **Step 1: Write the failing test**

`apps/core-engine/cmd/facearch-profile/main_test.go`:
```go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
)

// cliDoc is a small valid document; its numbers are test inputs.
const cliDoc = `{
  "schemaVersion": 1, "catalogueVersion": "fa-measure/1",
  "calibration": {"status": "uncalibrated", "benchmarkRunId": null, "target": null, "heldOutMetrics": null,
    "sources": [
      {"cite": "test", "for": "classifiers.faceShape.blendMargin"},
      {"cite": "test", "for": "classifiers.faceShape.minObservedWeight"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Long"},
      {"cite": "test", "for": "classifiers.faceShape.classes.Wide"}
    ]},
  "measurementsShown": ["width_length_ratio"],
  "classifiers": [{"key": "faceShape", "blendMargin": 0.6, "minObservedWeight": 0.5, "classes": [
    {"label": "Long", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.6, "spread": 0.1, "weight": 1}}},
    {"label": "Wide", "requires": [], "prototype": {"width_length_ratio": {"centre": 0.9, "spread": 0.1, "weight": 1}}}]}],
  "traits": []
}`

func writeDoc(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func testDeps(store *profile.MemoryStore) deps {
	return deps{
		catalogue: func(context.Context) (*measureclient.Catalogue, error) { return facearchtest.Catalogue(), nil },
		store: func(context.Context) (profile.Store, func(), error) {
			return store, func() {}, nil
		},
	}
}

func runCLI(d deps, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestValidate(t *testing.T) {
	code, out, errOut := runCLI(testDeps(nil), "validate", writeDoc(t, cliDoc))
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "valid against fa-measure/1 (1 classifiers, 0 traits, calibration uncalibrated)")
}

func TestValidateRejectsBadDocument(t *testing.T) {
	bad := writeDoc(t, strings.Replace(cliDoc, `"blendMargin": 0.6`, `"blendMargin": 0.4`, 1))
	code, _, errOut := runCLI(testDeps(nil), "validate", bad)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "classifiers[0].blendMargin: must be in (0.5, 1]")
}

func TestValidateReportsCatalogueFailure(t *testing.T) {
	d := testDeps(nil)
	d.catalogue = func(context.Context) (*measureclient.Catalogue, error) {
		return nil, errors.New("face-measure unavailable: connection refused")
	}
	code, _, errOut := runCLI(d, "validate", writeDoc(t, cliDoc))
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "catalogue: face-measure unavailable")
}

func TestUpsert(t *testing.T) {
	store := profile.NewMemoryStore()
	path := writeDoc(t, cliDoc)
	code, out, errOut := runCLI(testDeps(store), "upsert", path, "--brand", "wardah", "--app", "*", "--code", "wardah-v1", "--title", "Wardah")
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "stored wardah-v1 version 1 for brand wardah app *")

	p, err := store.Active(context.Background(), "wardah", "app-1")
	require.NoError(t, err)
	assert.Equal(t, "Wardah", p.Title)
	assert.JSONEq(t, cliDoc, p.Document)

	code, out, _ = runCLI(testDeps(store), "upsert", path, "--brand", "wardah", "--app", "*", "--code", "wardah-v1")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "version 2")
}

func TestUpsertInvalidDocumentWritesNothing(t *testing.T) {
	store := profile.NewMemoryStore()
	code, _, _ := runCLI(testDeps(store), "upsert", writeDoc(t, `{"schemaVersion": 1}`), "--brand", "*", "--app", "*", "--code", "x")
	assert.Equal(t, 1, code)
	_, err := store.Active(context.Background(), "*", "*")
	assert.ErrorIs(t, err, profile.ErrNoActiveProfile)
}

func TestUsage(t *testing.T) {
	path := writeDoc(t, cliDoc)
	for _, args := range [][]string{
		{},
		{"validate"},
		{"validate", path, "extra"},
		{"frobnicate", path},
		{"upsert", path, "--brand", "*", "--app", "*"},
		{"upsert", "--brand", "*", path},
	} {
		code, _, errOut := runCLI(testDeps(profile.NewMemoryStore()), args...)
		assert.Equal(t, 2, code, "%v", args)
		assert.Contains(t, errOut, "usage:", "%v", args)
	}
}
```

- [ ] **Step 2: Run; expect FAIL** (build failure: `undefined: run`, `undefined: deps`).

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 3: Implement**

`apps/core-engine/cmd/facearch-profile/main.go`:
```go
// Command facearch-profile validates and stores face-architecture profiles.
//
//	facearch-profile validate <file.json>
//	facearch-profile upsert <file.json> --brand <id|*> --app <id|*> --code <code> [--title <title>]
//
// Both fetch the ai-worker catalogue (VISION_AI_WORKER_URL, timeout
// FACE_MEASURE_TIMEOUT) and validate the document against it; validate
// never touches the database. upsert then stores the document as the next
// version of (brand, app, code), makes it active, and deactivates every
// other active profile in that brand+app scope. upsert reads DATABASE_URL
// like core-engine and migrates only its own table,
// face_architecture_profiles.
//
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md, "Profile CLI".
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/packages/goplatform"
)

// deps are the outside world; tests replace them.
type deps struct {
	catalogue func(ctx context.Context) (*measureclient.Catalogue, error)
	store     func(ctx context.Context) (profile.Store, func(), error)
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, liveDeps()))
}

const usage = `usage:
  facearch-profile validate <file.json>
  facearch-profile upsert <file.json> --brand <id|*> --app <id|*> --code <code> [--title <title>]
`

func run(ctx context.Context, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, file := args[0], args[1]
	switch cmd {
	case "validate":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if _, ok := load(ctx, file, stdout, stderr, d); !ok {
			return 1
		}
		return 0
	case "upsert":
		fs := flag.NewFlagSet("upsert", flag.ContinueOnError)
		fs.SetOutput(stderr)
		brand := fs.String("brand", "", "brand id, or * for any brand")
		app := fs.String("app", "", "application id, or * for any application")
		code := fs.String("code", "", "profile code")
		title := fs.String("title", "", "profile title (defaults to the code)")
		if err := fs.Parse(args[2:]); err != nil || fs.NArg() != 0 || *brand == "" || *app == "" || *code == "" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		raw, ok := load(ctx, file, stdout, stderr, d)
		if !ok {
			return 1
		}
		store, closeStore, err := d.store(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "database: %v\n", err)
			return 1
		}
		defer closeStore()
		p, err := store.Upsert(ctx, profile.UpsertInput{BrandID: *brand, ApplicationID: *app, Code: *code, Title: *title, Document: raw})
		if err != nil {
			fmt.Fprintf(stderr, "upsert: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "stored %s version %d for brand %s app %s (id %s), now active\n", p.Code, p.Version, p.BrandID, p.ApplicationID, p.ID)
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// load reads, parses and validates file against the live catalogue.
func load(ctx context.Context, file string, stdout, stderr io.Writer, d deps) ([]byte, bool) {
	raw, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "read %s: %v\n", file, err)
		return nil, false
	}
	doc, err := profile.Parse(raw)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", file, err)
		return nil, false
	}
	cat, err := d.catalogue(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "catalogue: %v\n", err)
		return nil, false
	}
	if err := profile.Validate(doc, cat); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", file, err)
		return nil, false
	}
	fmt.Fprintf(stdout, "%s: valid against %s (%d classifiers, %d traits, calibration %s)\n",
		file, cat.CatalogueVersion, len(doc.Classifiers), len(doc.Traits), doc.Calibration.Status)
	return raw, true
}

func liveDeps() deps {
	return deps{
		catalogue: func(ctx context.Context) (*measureclient.Catalogue, error) {
			goplatform.LoadDotEnv()
			url := goplatform.First("VISION_AI_WORKER_URL")
			if url == "" {
				url = config.DefaultVisionAIWorkerURL
			}
			timeout, err := goplatform.Duration("FACE_MEASURE_TIMEOUT", config.DefaultFaceMeasureTimeout)
			if err != nil {
				return nil, err
			}
			return measureclient.New(url, timeout).Catalogue(ctx)
		},
		store: func(ctx context.Context) (profile.Store, func(), error) {
			cfg, err := config.Load()
			if err != nil {
				return nil, nil, err
			}
			db, err := goplatform.OpenPostgres(ctx, cfg.DatabaseURL, goplatform.DBOptions{
				Logger:             slog.Default(),
				SlowQueryThreshold: cfg.DBSlowQueryThreshold,
				PingTimeout:        cfg.DependencyPingTimeout,
			})
			if err != nil {
				return nil, nil, err
			}
			closeDB := func() {
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			}
			// Only core-engine's own table; the server migrates the rest.
			if err := db.AutoMigrate(&profile.FaceArchitectureProfile{}); err != nil {
				closeDB()
				return nil, nil, fmt.Errorf("migrate face_architecture_profiles: %w", err)
			}
			return profile.NewGormStore(db), closeDB, nil
		},
	}
}
```

- [ ] **Step 4: Run; expect PASS** (6 tests).

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/cmd/facearch-profile/main.go apps/core-engine/cmd/facearch-profile/main_test.go
git commit -m "feat(core-engine): facearch-profile validate/upsert CLI"
```

---

### Task 11: Uncalibrated seed profile

**Before starting:** have the user review the Seed profile literals tables above. Change a value only in the seed file and its source note, and update the tables and `seed_test.go` expectations to match.

**Files:**
- Create: `apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json`
- Test: `apps/core-engine/cmd/facearch-profile/seed_test.go`

**Interfaces:**
- Consumes: Tasks 1, 4, 8, 10 (`runCLI`, `testDeps` from `main_test.go`).
- Produces: the seed profile file; loaded with `go run ./apps/core-engine/cmd/facearch-profile upsert apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json --brand '*' --app '*' --code seagull-default-uncalibrated`.

- [ ] **Step 1: Write the failing test.** `TestSeedOnFixtureFace` pins the mechanism on the recorded face (every expected label follows from the fixture values and the thresholds in the tables above); `TestSeedClassifiesAnInFrameFace` uses a **synthetic** in-frame face (the landmarker's extrapolated chin values) and pins only the blend behaviour. Neither is an accuracy claim.

`apps/core-engine/cmd/facearch-profile/seed_test.go`:
```go
package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/experience-gateway/apps/core-engine/internal/facearch"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/classify"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/facearchtest"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/measureclient"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/profile"
	"github.com/experience-gateway/apps/core-engine/internal/facearch/traits"
)

const seedPath = "seed/seagull-default-uncalibrated.json"

func TestValidateSeed(t *testing.T) {
	code, out, errOut := runCLI(testDeps(nil), "validate", seedPath)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "valid against fa-measure/1 (1 classifiers, 10 traits, calibration uncalibrated)")
}

func TestUpsertSeedAsGlobalDefault(t *testing.T) {
	store := profile.NewMemoryStore()
	code, _, errOut := runCLI(testDeps(store), "upsert", seedPath, "--brand", "*", "--app", "*", "--code", "seagull-default-uncalibrated")
	require.Equal(t, 0, code, errOut)
	p, err := store.Active(context.Background(), "any-brand", "any-app")
	require.NoError(t, err)
	assert.Equal(t, "seagull-default-uncalibrated", p.Code)
}

func loadSeed(t *testing.T) (*profile.FaceArchitectureProfile, *profile.Document) {
	t.Helper()
	raw, err := os.ReadFile(seedPath)
	require.NoError(t, err)
	p := &profile.FaceArchitectureProfile{ID: "seed", Code: "seagull-default-uncalibrated", Version: 1, Document: string(raw)}
	d, err := profile.Load(p)
	require.NoError(t, err)
	require.NoError(t, profile.Validate(d, facearchtest.Catalogue()))
	return p, d
}

// The seed is uncalibrated and says so.
func TestSeedIsLabelledUncalibrated(t *testing.T) {
	_, d := loadSeed(t)
	assert.Equal(t, profile.CalibrationUncalibrated, d.Calibration.Status)
	assert.Nil(t, d.Calibration.BenchmarkRunID)
	fs := d.Classifiers[0]
	assert.Equal(t, "faceShape", fs.Key)
	labels := []string{}
	for _, c := range fs.Classes {
		labels = append(labels, c.Label)
	}
	assert.Equal(t, []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}, labels)
	keys := []string{}
	for _, tr := range d.Traits {
		keys = append(keys, tr.Key)
	}
	assert.Equal(t, []string{"eyeSize", "eyeAngle", "eyeDistance", "eyeShape", "eyebrowShape",
		"eyebrowDistance", "eyebrowShortness", "lipShape", "noseWidth", "noseLength"}, keys)
}

// On the recorded fixture face (chin out of frame) the seed withholds what
// it cannot see and flags thresholds inside the noise band. Expected labels
// follow from the fixture values in facearchtest and the seed thresholds;
// they check the mechanism, not the seed's accuracy.
func TestSeedOnFixtureFace(t *testing.T) {
	p, d := loadSeed(t)
	out := facearch.Interpret(p, d, facearchtest.FixtureFace())

	fs := out.Classifications["faceShape"]
	assert.Equal(t, classify.StatusInsufficientEvidence, fs.Status)
	assert.Equal(t, []string{"Triangle", "Diamond", "Heart", "InvTriangle", "Oblong", "Oval", "Round", "Square"}, fs.NotAssessable)
	assert.Equal(t, []string{"chin_taper", "jaw_to_mid_width_ratio", "width_length_ratio"}, fs.MissingMeasurements)
	assert.InDelta(t, 0.25, *fs.ObservedWeightShare, 1e-12)

	want := map[string]traits.Result{
		"eyeSize":          {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Big"},
		"eyeAngle":         {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Downturned"},
		"eyeDistance":      {Status: "assessed", Label: "Wide-Set", BoundaryUncertain: true, Alternative: "Average"},
		"eyeShape":         {Status: "assessed", Label: "Almond", BoundaryUncertain: true, Alternative: "Round"},
		"eyebrowShape":     {Status: "assessed", Label: "Soft Angled", BoundaryUncertain: true, Alternative: "Hard Angled"},
		"eyebrowDistance":  {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Far-Apart"},
		"eyebrowShortness": {Status: "assessed", Label: "Normal"},
		"lipShape":         {Status: "assessed", Label: "Average", BoundaryUncertain: true, Alternative: "Downturned"},
		"noseWidth":        {Status: "assessed", Label: "Broad", BoundaryUncertain: true, Alternative: "Average"},
		"noseLength":       {Status: "not_assessable", MissingMeasurements: []string{"nose_length_ratio"}, Reason: "requires nose_length_ratio"},
	}
	assert.Equal(t, want, out.Traits)
	assert.Equal(t, "uncalibrated", out.Provenance.Calibration.Status)
	require.Len(t, out.Measurements, 15, "occluded measurements are shown with value null")
	var withheld []string
	for _, m := range out.Measurements {
		if m.Value.IsNull() {
			withheld = append(withheld, m.Key)
		}
	}
	assert.Equal(t, []string{"width_length_ratio", "jaw_to_mid_width_ratio", "thirds_lower", "nose_length_ratio", "asymmetry_total"}, withheld)
}

// A synthetic in-frame face: the fixture's face with the four jaw-dependent
// measurements set to the values the landmarker extrapolated for the
// cropped chin. Synthetic input, not a measurement of anyone.
func TestSeedClassifiesAnInFrameFace(t *testing.T) {
	p, d := loadSeed(t)
	m := facearchtest.FixtureFace()
	set := map[string]float64{"width_length_ratio": 0.8521, "jaw_to_mid_width_ratio": 0.8326, "chin_taper": 0.4534}
	for i := range m.Measurements {
		if v, ok := set[m.Measurements[i].Key]; ok {
			v := v
			m.Measurements[i].Value = measureclient.Value{Scalar: &v}
			m.Measurements[i].Visibility = measureclient.VisibilityUnknown
			m.Measurements[i].Band = nil
		}
	}
	fs := facearch.Interpret(p, d, m).Classifications["faceShape"]
	assert.Empty(t, fs.NotAssessable)
	assert.Len(t, fs.Scores, 8)
	var sum float64
	for _, s := range fs.Scores {
		sum += s
	}
	assert.InDelta(t, 1.0, sum, 1e-9)
	assert.Equal(t, classify.StatusBlend, fs.Status, "eight wide prototypes: no class reaches 0.6")
	assert.Equal(t, "Oval", fs.Primary)
	assert.Equal(t, "Round", fs.Secondary)
}
```

- [ ] **Step 2: Run; expect FAIL** (`open seed/seagull-default-uncalibrated.json: The system cannot find the file specified` / `no such file or directory`).

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 3: Write the seed.** Every class and trait cites its source; every unsourced number says `placeholder pending benchmark`.

`apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json`:
```json
{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {
    "status": "uncalibrated",
    "benchmarkRunId": null,
    "target": null,
    "heldOutMetrics": null,
    "sources": [
      {"cite": "Perfect Corp AI Face Analyzer API documentation (category vocabulary, as summarised in the spec's Research basis)", "for": "classifiers", "note": "Class and trait LABELS only. Perfect Corp publishes no thresholds, confidences or accuracy, so no number in this file comes from it."},
      {"cite": "Zafar & Popa 2016", "for": "classifiers.faceShape.blendMargin", "note": "Report a top-two blend when the top class is below 60%."},
      {"cite": "placeholder pending benchmark (policy, no source)", "for": "classifiers.faceShape.minObservedWeight", "note": "0.5 = at least half of the prototype weight must be observed. The spec's example value; a product choice, not a measurement."},
      {"cite": "placeholder pending benchmark (no source)", "for": "classifiers.faceShape.classes.Triangle.prototype", "note": "Spreads (width_length_ratio 0.12, width ratios 0.10, chin_taper 0.15) and weights (1.0 each) are the same deliberately wide values in every class. Nothing publishes them."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Triangle", "note": "Jaw widest, forehead narrowest. Qualitative -> numeric by us: jaw_to_mid 1.00 (jaw as wide as the cheeks), upper_to_mid 0.80 (narrow), chin_taper 0.60 (broad chin). width_length_ratio 0.70 is a neutral middle: the heuristic says nothing about length."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Diamond", "note": "Cheekbones widest, forehead and jaw both narrow, pointed chin. Qualitative -> numeric by us: upper_to_mid 0.80, jaw_to_mid 0.75, chin_taper 0.40; width_length_ratio 0.70 neutral. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Heart", "note": "Forehead widest, narrow jaw, pointed chin. Qualitative -> numeric by us: upper_to_mid 1.00, jaw_to_mid 0.75, chin_taper 0.35; width_length_ratio 0.70 neutral. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.InvTriangle", "note": "Forehead widest, jaw narrowing to a less pointed chin than Heart. Qualitative -> numeric by us: upper_to_mid 1.00, jaw_to_mid 0.80, chin_taper 0.50; width_length_ratio 0.70 neutral. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Oblong", "note": "Clearly longer than 1.5x the width; forehead, cheeks and jaw similar. Qualitative -> numeric by us: width_length_ratio 0.60, upper_to_mid 1.00, jaw_to_mid 1.00, chin_taper 0.50. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Oval", "note": "Length about 1.5x width (width_length_ratio 0.67 = 1/1.5, the one ratio these guides state numerically), forehead slightly wider than a rounded jaw. Qualitative -> numeric by us: upper_to_mid 0.90, jaw_to_mid 0.85, chin_taper 0.45. KNOWN BIAS: the guides measure hairline to chin; face_length_mesh stops at mesh point 10, below the hairline, so our width_length_ratio reads higher than theirs and this seed will lean towards Round and Square until calibrated. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Round", "note": "Length about equal to width (width_length_ratio 1.00), cheeks widest, soft jaw. Qualitative -> numeric by us: upper_to_mid 0.90, jaw_to_mid 0.90, chin_taper 0.55. Same hairline bias as Oval. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Consumer face-shape heuristics (qualitative; widely repeated in beauty guides, no single primary source)", "for": "classifiers.faceShape.classes.Square", "note": "Length about equal to width (width_length_ratio 1.00); forehead, cheeks and an angular jaw about equally wide. Qualitative -> numeric by us: upper_to_mid 1.00, jaw_to_mid 1.00, chin_taper 0.60. Same hairline bias as Oval. Spreads and weights: see classes.Triangle.prototype source."},
      {"cite": "Rule of fifths (neoclassical facial canon; the face is five eye-widths wide)", "for": "traits.eyeSize.rules", "note": "Centre 0.20 = one fifth. The +/-0.02 Average band is a placeholder pending benchmark (no source). Canons are not ideals (spec: Research basis); used only to place the Average band."},
      {"cite": "Consumer eye-shape heuristic: upturned = outer corner above the inner corner, downturned = below", "for": "traits.eyeAngle.rules", "note": "Centre 0 deg is the heuristic's own definition. The +/-3 deg Average band is a placeholder pending benchmark (no source); the fixture face's noise band for canthal_tilt_deg is about +/-6 deg (bands are conservative by construction), so expect boundaryUncertain often."},
      {"cite": "Farkas et al. 1985 neoclassical canon (intercanthal width = eye fissure width); consumer rule 'eyes one eye-width apart'", "for": "traits.eyeDistance.rules", "note": "Centre 1.0 from the canon. The +/-0.1 Average band is a placeholder pending benchmark (no source)."},
      {"cite": "placeholder pending benchmark (no source)", "for": "traits.eyeShape.rules", "note": "Only the ORDER is from consumer heuristics (Round = tallest opening, Narrow = flattest, Almond between). Thresholds eye_aperture_ratio > 0.33 Round and < 0.22 Narrow have no source."},
      {"cite": "placeholder pending benchmark (no source)", "for": "traits.eyebrowShape.rules", "note": "Only the qualitative definitions are consumer heuristics (Straight = little arch; angled = a peak with a dropping tail; Rounded = curved, no sharp peak). Thresholds brow_arch_ratio < 0.08, brow_tail_tilt_deg > 35 (Hard Angled) and > 20 (Soft Angled) have no source. Perfect Corp's 'Obscured' is not a label here: hidden brows are not_assessable."},
      {"cite": "Consumer brow-mapping rule: the brow starts above the inner eye corner", "for": "traits.eyebrowDistance.rules", "note": "interbrow_ratio 1.0 = brow heads exactly above the inner corners. The +/-0.1 Average band is a placeholder pending benchmark (no source)."},
      {"cite": "Consumer brow-mapping rule: the brow runs from above the inner eye corner to the line from the nostril through the outer eye corner", "for": "traits.eyebrowShortness.rules", "note": "A brow that follows the rule is at least as long as the eye, so brow_length_to_eye_ratio < 1.0 is Short. Derived by us from a qualitative rule; unvalidated."},
      {"cite": "placeholder pending benchmark (no source)", "for": "traits.lipShape.rules", "note": "Heavy Upper Lip at upper_lower_lip_ratio > 1.0 follows from the words (upper lip taller than lower). Every other lipShape threshold (tilt > 5 deg, ratio < 0.5, fullness > 0.45 / > 0.5 / < 0.3, bow depth > 0.05) and the rule order have no source."},
      {"cite": "Farkas et al. 1985 neoclassical canon (mouth width = 1.5 x nasal width)", "for": "traits.lipShape.rules.Wide", "note": "alar_width_to_mouth_ratio 0.667 = 1/1.5 centres Wide (< 0.60) and Narrow (> 0.75); the band edges are placeholders pending benchmark. The measurement uses the nostril-base points (ALAR_BASE, mesh 98/327), a proxy for alare (proxy: true), so the canon centre is applied to a proxy of the width it defines."},
      {"cite": "Farkas et al. 1985 neoclassical canon (nasal width = intercanthal width)", "for": "traits.noseWidth.rules", "note": "Centre 1.0 from the canon. The +/-0.1 Average band is a placeholder pending benchmark (no source). alar_width_to_intercanthal_ratio uses the nostril-base points (ALAR_BASE, mesh 98/327), a proxy for alare (proxy: true), so the canon centre is applied to a proxy of the width it defines."},
      {"cite": "Equal facial thirds canon (Jayaratne et al. 2012 found it met in 0% of 103 adults: a heuristic midpoint, not a norm)", "for": "traits.noseLength.rules", "note": "Equal middle and lower thirds give nose_length_ratio 0.5. The +/-0.03 Average band is a placeholder pending benchmark (no source)."}
    ]
  },
  "measurementsShown": [
    "width_length_ratio",
    "upper_to_mid_width_ratio",
    "jaw_to_mid_width_ratio",
    "thirds_upper_proxy",
    "thirds_mid",
    "thirds_lower",
    "fifths",
    "eye_width",
    "canthal_tilt_deg",
    "intercanthal_ratio",
    "alar_width_to_intercanthal_ratio",
    "nose_length_ratio",
    "mouth_width",
    "upper_lower_lip_ratio",
    "asymmetry_total"
  ],
  "classifiers": [
    {
      "key": "faceShape",
      "blendMargin": 0.6,
      "minObservedWeight": 0.5,
      "classes": [
        {"label": "Triangle", "requires": ["upper_to_mid_width_ratio", "jaw_to_mid_width_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.70, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 0.80, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.60, "spread": 0.15, "weight": 1.0}}},
        {"label": "Diamond", "requires": ["upper_to_mid_width_ratio", "jaw_to_mid_width_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.70, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 0.80, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 0.75, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.40, "spread": 0.15, "weight": 1.0}}},
        {"label": "Heart", "requires": ["upper_to_mid_width_ratio", "jaw_to_mid_width_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.70, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 0.75, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.35, "spread": 0.15, "weight": 1.0}}},
        {"label": "InvTriangle", "requires": ["upper_to_mid_width_ratio", "jaw_to_mid_width_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.70, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 0.80, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.50, "spread": 0.15, "weight": 1.0}}},
        {"label": "Oblong", "requires": ["width_length_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.60, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.50, "spread": 0.15, "weight": 1.0}}},
        {"label": "Oval", "requires": ["width_length_ratio"], "prototype": {
          "width_length_ratio": {"centre": 0.67, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 0.90, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 0.85, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.45, "spread": 0.15, "weight": 1.0}}},
        {"label": "Round", "requires": ["width_length_ratio"], "prototype": {
          "width_length_ratio": {"centre": 1.00, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 0.90, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 0.90, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.55, "spread": 0.15, "weight": 1.0}}},
        {"label": "Square", "requires": ["width_length_ratio", "jaw_to_mid_width_ratio"], "prototype": {
          "width_length_ratio": {"centre": 1.00, "spread": 0.12, "weight": 1.0},
          "upper_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "jaw_to_mid_width_ratio": {"centre": 1.00, "spread": 0.10, "weight": 1.0},
          "chin_taper": {"centre": 0.60, "spread": 0.15, "weight": 1.0}}}
      ]
    }
  ],
  "traits": [
    {"key": "eyeSize", "default": "Average", "rules": [
      {"label": "Big", "when": [{"measurement": "eye_to_face_width_ratio", "op": ">", "value": 0.22}]},
      {"label": "Small", "when": [{"measurement": "eye_to_face_width_ratio", "op": "<", "value": 0.18}]}]},
    {"key": "eyeAngle", "default": "Average", "rules": [
      {"label": "Downturned", "when": [{"measurement": "canthal_tilt_deg", "op": "<", "value": -3}]},
      {"label": "Upturned", "when": [{"measurement": "canthal_tilt_deg", "op": ">", "value": 3}]}]},
    {"key": "eyeDistance", "default": "Average", "rules": [
      {"label": "Wide-Set", "when": [{"measurement": "intercanthal_ratio", "op": ">", "value": 1.1}]},
      {"label": "Close-set", "when": [{"measurement": "intercanthal_ratio", "op": "<", "value": 0.9}]}]},
    {"key": "eyeShape", "default": "Almond", "rules": [
      {"label": "Round", "when": [{"measurement": "eye_aperture_ratio", "op": ">", "value": 0.33}]},
      {"label": "Narrow", "when": [{"measurement": "eye_aperture_ratio", "op": "<", "value": 0.22}]}]},
    {"key": "eyebrowShape", "default": "Rounded", "rules": [
      {"label": "Straight", "when": [{"measurement": "brow_arch_ratio", "op": "<", "value": 0.08}]},
      {"label": "Hard Angled", "when": [{"measurement": "brow_tail_tilt_deg", "op": ">", "value": 35}]},
      {"label": "Soft Angled", "when": [{"measurement": "brow_tail_tilt_deg", "op": ">", "value": 20}]}]},
    {"key": "eyebrowDistance", "default": "Average", "rules": [
      {"label": "Far-Apart", "when": [{"measurement": "interbrow_ratio", "op": ">", "value": 1.1}]},
      {"label": "Close", "when": [{"measurement": "interbrow_ratio", "op": "<", "value": 0.9}]}]},
    {"key": "eyebrowShortness", "default": "Normal", "rules": [
      {"label": "Short", "when": [{"measurement": "brow_length_to_eye_ratio", "op": "<", "value": 1.0}]}]},
    {"key": "lipShape", "default": "Average", "rules": [
      {"label": "Downturned", "when": [{"measurement": "mouth_corner_tilt_deg", "op": ">", "value": 5}]},
      {"label": "Heavy Upper Lip", "when": [{"measurement": "upper_lower_lip_ratio", "op": ">", "value": 1.0}]},
      {"label": "Heavy Lower Lip", "when": [{"measurement": "upper_lower_lip_ratio", "op": "<", "value": 0.5}]},
      {"label": "Round", "when": [{"measurement": "lip_fullness_ratio", "op": ">", "value": 0.45}, {"measurement": "alar_width_to_mouth_ratio", "op": ">", "value": 0.75}]},
      {"label": "Full", "when": [{"measurement": "lip_fullness_ratio", "op": ">", "value": 0.5}]},
      {"label": "Thin", "when": [{"measurement": "lip_fullness_ratio", "op": "<", "value": 0.3}]},
      {"label": "Wide", "when": [{"measurement": "alar_width_to_mouth_ratio", "op": "<", "value": 0.6}]},
      {"label": "Narrow", "when": [{"measurement": "alar_width_to_mouth_ratio", "op": ">", "value": 0.75}]},
      {"label": "Bow", "when": [{"measurement": "cupid_bow_depth", "op": ">", "value": 0.05}]}]},
    {"key": "noseWidth", "default": "Average", "rules": [
      {"label": "Broad", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": ">", "value": 1.1}]},
      {"label": "Narrow", "when": [{"measurement": "alar_width_to_intercanthal_ratio", "op": "<", "value": 0.9}]}]},
    {"key": "noseLength", "default": "Average", "rules": [
      {"label": "Long", "when": [{"measurement": "nose_length_ratio", "op": ">", "value": 0.53}]},
      {"label": "Short", "when": [{"measurement": "nose_length_ratio", "op": "<", "value": 0.47}]}]}
  ]
}
```

- [ ] **Step 4: Run; expect PASS** (11 tests in the package). For reference, the synthetic in-frame face scores Oval 0.2466, Round 0.2399, Diamond 0.1715, InvTriangle 0.1627, Heart 0.1009, Triangle 0.0393, Square 0.0287, Oblong 0.0104.

Run: `go test ./apps/core-engine/cmd/facearch-profile/ -v`

- [ ] **Step 5: Commit**
```bash
git add apps/core-engine/cmd/facearch-profile/seed apps/core-engine/cmd/facearch-profile/seed_test.go
git commit -m "feat(core-engine): seagull-default-uncalibrated face-architecture seed profile"
```

---

### Task 12: Documentation, end-to-end check, release the lane

**Files:**
- Create: `docs/FACE-ARCHITECTURE.md`
- Modify: `docs/LANES.md` (your row only; not committed)

- [ ] **Step 1: Write `docs/FACE-ARCHITECTURE.md`** covering, in this order: what the endpoint does and does not do (no guidance polygons until Plan 3; no accuracy claim until Plan 4; every response carries `provenance.calibration.status`); request (`POST /api/v1/vision/face-architecture/:brandId/:applicationId`, multipart `image`) and the response fields, including the statuses `single | blend | insufficient_evidence | unavailable` and `assessed | not_assessable | unavailable`, `boundaryUncertain` / `alternative`, and `"guidance": null`; the error table (400 `missing_image` or the worker's 400 body; 409 `no_active_face_architecture_profile`; 422 worker body passed through; 503 worker body or `face_measure_unreachable`; 500 `profile_lookup_failed` / `profile_document_invalid`), all shaped `{"detail": {"code": ...}}`; profile lookup order and "one active profile per scope"; the CLI (`validate`, `upsert`, env `VISION_AI_WORKER_URL`, `FACE_MEASURE_TIMEOUT`, `DATABASE_URL`) and the exact command that loads the seed; the document schema with the validation rules and the source-coverage rule; the list-valued-measurement rule (read from the catalogue's `shape`; core-engine keeps no table); how noise bands drive `boundaryUncertain` (value-centred, conservative, so the flag is common; a `null` band means no uncertainty information) and which worker nulls make a measurement unusable (`occluded`, `canonical frame requires region eyes`, `degenerate geometry`); the worker's `quality.warnings` passed through unchanged; the setting `FACE_MEASURE_TIMEOUT` (default 20 s, why, what replaces it); the table `face_architecture_profiles` as core-engine-owned; a link to the spec and to this plan's Seed profile literals tables.

- [ ] **Step 2: Full suite.**

Run: `go test ./apps/core-engine/... && go vet ./apps/core-engine/internal/facearch/... ./apps/core-engine/cmd/facearch-profile/ ./apps/core-engine/cmd/server/`
Expected: all `ok`; vet silent.

- [ ] **Step 3: End-to-end (only if Plan 1 (`feat/face-measure`) is merged and running, and you hold the `runtime` lane).** Ports are global across all three repositories (`docs/PORTS.md`); claim `runtime` in `docs/LANES.md` first, and skip this step if another session holds it. With the ai-worker vision profile on `VISION_AI_WORKER_URL` and core-engine running against a database you may write to:
```bash
FACEARCH_TEST_AI_WORKER_URL=http://127.0.0.1:8088 go test ./apps/core-engine/internal/facearch/measureclient/ -run TestRecordedCatalogueMatchesLiveWorker -v
go run ./apps/core-engine/cmd/facearch-profile validate apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json
go run ./apps/core-engine/cmd/facearch-profile upsert apps/core-engine/cmd/facearch-profile/seed/seagull-default-uncalibrated.json --brand '*' --app '*' --code seagull-default-uncalibrated
curl -s -F image=@apps/ai-worker/tests/tryon/fixtures/sample_face.jpg http://127.0.0.1:8082/api/v1/vision/face-architecture/any-brand/any-app
```
Expected: the live-catalogue test passes; `validate` prints `valid against fa-measure/1 (1 classifiers, 10 traits, calibration uncalibrated)`; `upsert` prints `stored seagull-default-uncalibrated version 1 ...`; the curl returns 200 with `faceShape.status` `insufficient_evidence` (the photo's chin is out of frame), `noseLength.status` `not_assessable`, `provenance.calibration.status` `uncalibrated` and `"guidance": null`. Release `runtime` afterwards.

- [ ] **Step 4: Remove your row from `docs/LANES.md` Active claims.**

- [ ] **Step 5: Commit**
```bash
git add docs/FACE-ARCHITECTURE.md
git commit -m "docs: face-architecture endpoint, profiles and seed"
```
(`docs/LANES.md` is shared coordination state; leave it uncommitted unless the user asks.)
