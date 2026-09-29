# Face architecture engine: measured face geometry, interpreted per brand

Date: 2026-09-29
Status: design, awaiting review
Scope: `apps/ai-worker` (vision profile: landmarker, `/face-measure`),
`apps/core-engine` (new `internal/facearch`, `cmd/facearch-profile`,
`cmd/face-benchmark`). Dashboard and gateway routing are filed in Seagull-web
and Seagull-gateway against the contract described here.

## The problem

Paragon wants face-shape and facial-proportion analysis at feature parity with
Perfect Corp's AI Face Analyzer (which Make Over already uses) and Wardah's
Personal Face Analysis. Nothing in this repo measures face geometry today, and
what looks like a foundation for it is not one:

- `vision/models/landmark_mesh.py` is constructed without a model path, so the
  MediaPipe branch never runs. Every request falls back to a Haar cascade or a
  fixed centred box and `_generate_canonical_468_landmarks`, which emits
  synthetic ring-shaped points. Only seven indices are anatomically placed, and
  confidence is a hardcoded 0.96 / 0.92 / 0.88.
- The downstream pipeline speaks severity 0-100 for skin concerns. A
  width-to-length ratio is not a severity.

So the engine needs a real landmarker, a measurement layer, and an
interpretation layer that does not pretend its thresholds are facts.

## What this design builds

Two layers with one hard boundary between them:

1. **Measure (ai-worker).** A real MediaPipe FaceLandmarker, quality gates, a
   canonical frame, region visibility, and a catalogue of named geometric
   measurements with noise bands. Only numbers come out. Definitions are code
   because they describe geometry, not policy.
2. **Interpret (core-engine).** A per-brand, versioned profile in the database
   turns measurements into a face-shape blend, Perfect Corp-compatible traits
   and makeup-placement polygons. Every threshold lives in the profile, and
   every response says whether the profile is calibrated.

Plus a **benchmark harness** that compares our output with manually collected
Perfect Corp / Wardah / Make Over results and fits a calibrated profile.

**Usable first, accurate later.** The endpoint works end to end on day one
with a seed profile that is explicitly labelled uncalibrated. Accuracy comes
from the benchmark, not from guessing harder.

### Out of scope

- Colour season, undertone, foundation shade match (a colour engine, not this).
- Learned (ONNX) face-shape classifier — needs our own labelled data; the
  `Classifier` interface leaves room for it.
- Eyelid type, brow density, cheekbone prominence — not measurable from 2D
  landmarks; returned as `not_supported` (see Measurement catalogue).
- Perspective (selfie-distance) correction — see Known limitations.
- Admin API / dashboard for profiles (profiles go in through a CLI for now).
- Replacing the synthetic fallback in `/segment-and-pose`, and the licence
  problem with the shipped face-parsing model — separate tasks.

## Research basis

Condensed from a literature, dataset and competitor scan (2026-09-29). Sources
marked (U) were seen only as an abstract or search snippet.

### Models and data we can and cannot use

| Asset | Licence | Commercial | Use |
|---|---|---|---|
| MediaPipe FaceLandmarker (478 pts, transformation matrix) | Apache-2.0 | Yes | v1 landmarker. Confirm the weights wording in Google's model card before release. |
| FairFace | CC BY 4.0 | Yes (dataset) | Fairness evaluation slice (has a Southeast Asian group). No shape labels. |
| FLAME 2023 Open | CC BY 4.0 | Yes | Candidate 3D basis for a later version. |
| InsightFace 2d106/3D68, DECA, EMOCA, MICA | non-commercial | No | Avoid. |
| CelebA, CelebAMask-HQ, 300W(-LP), Menpo, LaPa, FaceScape, UTKFace, FFHQ | non-commercial | No | Avoid, including weights trained on them (SPIGA, PIPNet, HRNet, 3DDFA_V2, BiSeNet face-parsing, `jonathandinu/face-parsing`). |
| Kaggle niten19 / merged Kaggle face-shape sets, fahd9999 classifier | scraped celebrity images | No in practice | Avoid for training or as the sole benchmark. |
| Indonesian Muslim Student Face Dataset | CC BY-NC 3.0 | No | Research evaluation at most. |

No commercially clean face-shape dataset or classifier exists. A learned
classifier later needs our own consented, labelled Indonesian set (hijab
included, at least three raters per image, kappa reported, soft labels).

### Findings that shaped the design

- **Face-shape labels are ambiguous.** 62 of 117 celebrities cited under two or
  more shapes had conflicting labels across fashion sites (Tio 2019,
  arXiv:1911.07916). Geometry-only classifiers land at 64-80% (Sunhem & Pasupa
  2016; Pasupa et al. 2019; Zafar & Popa 2016). Published 93-99% figures score
  on training data or leak duplicates across splits (Tio 2019; Fuad & Putra
  2025; jerewy faceshape-preprocessing-study).
  → Output a top-two **blend** with a margin (Zafar & Popa 2016, 60/40), not a
  hard class. Do not quote the literature's accuracy as a target.
- **MediaPipe error is 2.67-3.85% of the eye-centre distance for a single
  image**, across 17 regions including South-eastern Asia; human annotators
  disagree by 2.56% (Face Mesh V2 model card). Out of scope for the model:
  roll above 8°, yaw above 80°, less than half the face visible. z comes from
  synthetic data; ground truth is 2D only.
  → Normalise by eye-centre distance, carry a noise band on every measurement,
  never use z as a measurement.
- **MediaPipe has no hairline (trichion), gonion or zygion** (mediapipe issue
  #1615). Contour points under a hijab are hallucinated; the model card
  evaluates no occlusion.
  → Name proxies as proxies. Mark measurements from occluded regions
  `occluded` with no value.
- **"Ideal" canons do not hold.** Equal thirds met in 0% of 103 Southern
  Chinese adults (Jayaratne et al. 2012); best Turkish canons 17-33% (Karaca
  Saygili et al. 2016); the phi mask is methodologically faulty (Holland 2008,
  U).
  → No golden-ratio scoring. Population references (Othman et al. 2016,
  Malaysian Malay; Prasetyono 2009, Deutero-Malay women) may be shown as
  labelled data, never as targets.
- **Symmetry:** mirror, relabel left/right pairs, Procrustes-superimpose; the
  residual is asymmetry (Ekrami et al. 2018).
- **Selfie distance inflates central widths** (~30% wider nasal base at 30 cm
  vs 1.5 m, Ward et al. 2018, U).
- **Parity targets.** Perfect Corp's API documents the categories below; a
  February 2026 press release says Make Over uses Perfect Corp's face ratio and
  attribute analysis, so this is Make Over's parity target too. Wardah's
  Personal Face Analysis instead returns a "Face Balance Type" (Curved/Straight
  × Thin/Thick) and raw eye / nose / mouth / lip measurements. Neither vendor
  publishes attribute accuracy; Perfect Corp returns categories with no
  confidence and no "not measurable" state.

| Perfect Corp group | Values |
|---|---|
| faceShape | Triangle, Diamond, Heart, InvTriangle, Oblong, Oval, Round, Square, Unknown |
| eyeShape / eyeSize / eyeAngle / eyeDistance | Narrow, Round, Almond / Big, Small, Average / Downturned, Upturned, Average / Close-set, Wide-Set, Average |
| eyelid | Hooded-lid, Single-lid, Double-lid, Deep-Set |
| eyebrowShape / Thickness / Distance / Shortness | Hard Angled, Soft Angled, Straight, Rounded, Obscured / Dense, Sparse, Average, Unknown / Far-Apart, Close, Average / Short, Normal |
| lipShape | Bow, Downturned, Full, Heavy Lower Lip, Heavy Upper Lip, Narrow, Round, Thin, Wide, Average |
| noseWidth / noseLength | Narrow, Broad, Average / Long, Short, Average |
| cheekbones | Flat Cheekbone, High Cheekbone, Low Cheekbone, Round Cheeks |

- **Makeup guidance** as template regions placed from landmarks and chosen by
  editable rules follows Perfect Corp's published per-shape contour guidance
  and the examples-plus-rules approach of Alashkar et al. (AAAI 2017).

## Architecture

```
image ─► core-engine  POST /api/v1/vision/face-architecture/:brandId/:applicationId
           │ 1. measureclient → ai-worker POST /api/v1/face-measure (landmarks=true)
           │      FaceLandmarker → gates → canonical frame → visibility
           │      → measurements + noise bands
           │ 2. profile lookup (brand+app → brand+* → *+*)
           │ 3. classifiers (face shape blend, optional balance type)
           │    traits (rule lists) · guidance (templates → polygons)
           ▼
         response: quality, regions, measurements, classifications, traits,
                   guidance, provenance (model, catalogue, profile, calibration)
```

The ai-worker stays stateless and brand-agnostic. core-engine owns policy
because it already owns per-brand configuration in the database.

## Measurement layer (ai-worker, vision profile)

### Files

- `vision/models/face_landmarker.py` — MediaPipe FaceLandmarker (Tasks API,
  image mode, one face, iris refinement, transformation matrix). Loaded once
  at profile startup; missing model means the endpoint answers 503. There is
  no synthetic fallback.
- `vision/facegeom/manifests/*.json` — one manifest per supported model file,
  matched by the file's SHA-256: name, variant, licence, source URL,
  `landmarkErrorIod` (single-image range from the model card), `maxRollDeg`
  (8, model card out-of-scope limit). These are properties of one exact model
  file, so a model with no matching manifest is refused (503) rather than
  measured with another model's error figures.
- `vision/facegeom/` — pure numpy, no I/O:
  - `anchors.py` — the named mapping from mesh index to the point it stands
    in for (table below), one module, versioned, sources cited.
  - `frame.py` — eye-centre distance, roll correction, pose from the
    transformation matrix.
  - `visibility.py` — region visibility interface and the v1 detector.
  - `measures.py` — the catalogue: one function per measurement.
  - `noise.py` — noise-band estimation.
  - `symmetry.py` — mirrored Procrustes asymmetry.
- `vision/routers/face_measure.py` — `POST /api/v1/face-measure`,
  `GET /api/v1/face-measure/catalogue`.
- `vision/schemas/face_measure.py` — Pydantic response models.

### Configuration

New `Settings` entries in `ai_worker/core/settings.py`, each documented in its
docstring. Defaults are flagged under Open literals.

| Setting | Meaning |
|---|---|
| `VISION_FACE_LANDMARKER_PATH` | Path to `face_landmarker.task`; manifest is read from the same directory. |
| `FACE_MEASURE_MAX_YAW_DEG`, `FACE_MEASURE_MAX_PITCH_DEG` | Pose gate (product policy). |
| `FACE_MEASURE_MIN_IOD_PX` | Minimum eye-centre distance in pixels (resolution gate). |
| `FACE_MEASURE_NOISE_SAMPLES` | Perturbation samples for noise bands. |
| `FACE_MEASURE_OCCLUSION_DELTA_E` | Colour-difference threshold for the v1 visibility detector. |

### Pipeline

1. **Detect.** Exactly one face, else 422 `face_count`.
2. **Gates.** Roll ≤ manifest `maxRollDeg`; yaw and pitch within config; eye
   distance ≥ `FACE_MEASURE_MIN_IOD_PX`. A failed gate is a 422 carrying every
   failed gate and its measured value, never a best-effort answer.
3. **Canonical frame.** Eye centre = midpoint of that eye's two corners (the
   model card's definition). Rotate so the eye line is horizontal; express all
   lengths in eye-centre-distance units (IOD). Nothing is reported in mm.
4. **Region visibility.** Regions: `forehead`, `contour_left`, `contour_right`,
   `jaw`, `brows`, `eyes`, `nose`, `lips`. Each is `observed`, `occluded` or
   `unknown`. Colour consistency can only judge skin regions (`forehead`,
   `contour_*`, `jaw`); feature regions (eyes, brows, nose, lips) are always
   `unknown` in v1, their values computed and marked so. v1 detector: compare the CIELAB colour statistics inside each
   region's mesh polygon with a cheek reference patch; beyond
   `FACE_MEASURE_OCCLUSION_DELTA_E` the region is `occluded`. The detector sits
   behind an interface so the existing hijab detector can replace it if the
   licence audit clears it.
   **Out of frame.** A region with any landmark outside the image is
   `occluded` with reason `(out of frame)`, and the response carries
   `face_partly_out_of_frame`. MediaPipe extrapolates points it cannot see
   (the CC0 fixture photo's chin sits below the frame and still gets a
   menton), so a cropped chin is treated like a covered one.
5. **Measure.** Every catalogue entry declares the regions it needs. If any is
   `occluded`, the entry is returned with `visibility: "occluded"` and no
   value. `unknown` computes the value and marks it `unknown`.
6. **Noise bands.** Perturb every landmark with isotropic Gaussian noise whose
   standard deviation is the upper bound of the manifest's
   `landmarkErrorIod`, `FACE_MEASURE_NOISE_SAMPLES` times, seeded from the
   image hash so a result is reproducible. The band is the 5th-95th percentile
   of the recomputed values.

### Anchor mapping (initial)

MediaPipe publishes no semantic landmark definitions. This table is ours: an
initial mapping from the MediaPipe canonical mesh and the only published list
(arXiv:2508.13363), to be checked by the fixture tests in the plan. Names say
what each point is a proxy for.

| Anchor | Index | Proxy for |
|---|---|---|
| `mesh_top` | 10 | top of mesh (not trichion) |
| `glabella` | midpoint of 9 and 8 | glabella (mid brow band; 168 sits at eye level, nearer nasion, on the fixture face) |
| `nose_tip` | 1 | pronasale |
| `subnasale` | 2 | subnasale |
| `alar_r`, `alar_l` | 98, 327 | alare |
| `exo_r`, `exo_l` | 33, 263 | exocanthion |
| `endo_r`, `endo_l` | 133, 362 | endocanthion |
| `iris_r`, `iris_l` | 468, 473 | iris centre |
| `lid_up_r`, `lid_lo_r`, `lid_up_l`, `lid_lo_l` | 159, 145, 386, 374 | mid upper / lower eyelid |
| `brow_up_r[]`, `brow_lo_r[]` | 70, 63, 105, 66, 107 / 46, 53, 52, 65, 55 | brow upper / lower contour |
| `brow_up_l[]`, `brow_lo_l[]` | 300, 293, 334, 296, 336 / 276, 283, 282, 295, 285 | brow upper / lower contour |
| `cheilion_r`, `cheilion_l` | 61, 291 | mouth corners |
| `lip_up_top`, `lip_up_in`, `lip_lo_in`, `lip_lo_bot` | 0, 13, 14, 17 | upper vermilion top, stomion upper / lower, lower vermilion bottom |
| `oval_r`, `oval_l` | 234, 454 | face-oval extremes (not zygion) |
| `forehead_r`, `forehead_l` | 54, 284 | upper contour pair (forehead-width proxy) |
| `jaw_r`, `jaw_l` | 172, 397 | lower contour pair (jaw-width proxy, not gonion) |
| `menton` | 152 | chin bottom |
| `face_oval[]` | MediaPipe `FACEMESH_FACE_OVAL` ordering | outline |

### Measurement catalogue (`fa-measure/1`)

| Group | Key | Definition | Needs |
|---|---|---|---|
| Outline | `face_length_mesh` | mesh_top → menton | forehead |
| | `face_width_oval` | oval_r ↔ oval_l | contour |
| | `upper_width_proxy` | forehead_r ↔ forehead_l | forehead, contour |
| | `jaw_width_proxy` | jaw_r ↔ jaw_l | jaw |
| | `width_length_ratio` | face_width_oval / face_length_mesh | forehead, contour |
| | `upper_to_mid_width_ratio`, `jaw_to_mid_width_ratio` | upper_width_proxy / face_width_oval, jaw_width_proxy / face_width_oval | forehead, contour / jaw, contour |
| | `jaw_contour_angles[]` | angle from vertical of menton → each lower face_oval point (Tio 2019) | jaw |
| | `chin_taper`, `oval_fit_residual` | ellipse fits to outline, chin and cheek points (Zafar & Popa 2016) | contour, jaw |
| Proportions | `thirds_upper_proxy` | mesh_top → glabella, always `proxy: true` | forehead |
| | `thirds_mid`, `thirds_lower` | glabella → subnasale → menton | — |
| | `lower_face_split` | subnasale → stomion vs stomion → menton | lips |
| | `fifths[5]` | oval_r → exo_r → endo_r → endo_l → exo_l → oval_l | contour, eyes |
| Eyes | `eye_width` | exo → endo, mean of both sides (side differences go to symmetry) | eyes |
| | `eye_aperture_ratio` | lid distance / eye width, per side | eyes |
| | `canthal_tilt_deg` | endo → exo angle vs the eye line, per side | eyes |
| | `intercanthal_ratio` | endo_r ↔ endo_l / mean eye width | eyes |
| Brows | `brow_length`, `brow_arch_ratio`, `brow_peak_position`, `brow_tail_tilt_deg`, `interbrow_ratio` | from brow contours | brows |
| | `brow_band_height` | mean upper-lower contour gap; a shape proxy, not density | brows |
| Nose | `alar_width_ratio` | alar width / intercanthal width, and / mouth width | nose (and lips) |
| | `nose_length_ratio` | glabella → subnasale / glabella → menton (avoids the forehead so it survives a hijab) | nose, brows, jaw |
| Lips | `mouth_width` | cheilion_r ↔ cheilion_l | lips |
| | `upper_lower_lip_ratio`, `lip_fullness_ratio`, `cupid_bow_depth`, `mouth_corner_tilt_deg` | from lip contours | lips |
| Cheeks | `cheek_widest_height` | vertical position of the widest outline point, relative to eye line and subnasale | contour |
| Symmetry | `asymmetry_total`, `asymmetry_by_region{}` | mirrored Procrustes residual in IOD (Ekrami 2018) | per region |
| Not supported | `eyelid_type`, `brow_density`, `cheekbone_prominence` | always `visibility: "not_supported"` with a reason | — |

Also in `fa-measure/1` (added while writing the plan, each a plain derived
scalar so profiles need no list handling): `jaw_taper_angle_deg`,
`thirds_lower_to_mid_ratio`, `eye_to_face_width_ratio`,
`brow_length_to_eye_ratio`, `brow_eye_gap`, and per-region symmetry as
separate keys `asymmetry_eyes|brows|nose|lips|contour|jaw`. The nose ratio
is split into `alar_width_to_intercanthal_ratio` and
`alar_width_to_mouth_ratio`. The plan's `measures.py` is the authoritative
list.

Brand configuration decides which measurements are shown and how they are
banded. It never changes a definition. A definition change bumps
`catalogueVersion`.

### Response

```json
{
  "catalogueVersion": "fa-measure/1",
  "model": {"name": "face_landmarker", "version": "...", "manifestSha256": "..."},
  "quality": {
    "rollDeg": 2.1, "yawDeg": -4.0, "pitchDeg": 3.2, "iodPx": 118,
    "gatesPassed": ["face_count", "roll", "yaw", "pitch", "iod_px"],
    "warnings": ["perspective_not_corrected"]
  },
  "regions": {"forehead": "occluded", "jaw": "observed", "...": "..."},
  "measurements": [
    {
      "key": "width_length_ratio",
      "value": null, "unit": "ratio", "band": null,
      "visibility": "occluded", "proxy": true,
      "landmarks": [10, 152, 234, 454],
      "reason": "requires region forehead"
    }
  ],
  "landmarks": null
}
```

`landmarks` (478 × [x, y] in image pixels) is returned only when the request
sets `landmarks=true`; core-engine always sets it, because guidance polygons
need it.

`GET /api/v1/face-measure/catalogue` returns every key with unit, required
regions, `proxy`, `supported` and `catalogueVersion`. core-engine validates
profiles against it.

### Errors

| Case | Status |
|---|---|
| No model file or manifest | 503 `face_landmarker_unavailable` |
| Unreadable image | 400 |
| Zero or several faces | 422 `face_count` |
| Any gate failed | 422 with `failedGates[{gate, value, limit}]` |

## Interpretation layer (core-engine)

### Package `internal/facearch/`

- `measureclient/` — HTTP client for `/face-measure` and the catalogue; base
  URL `VISION_AI_WORKER_URL`. Passes 422 and 503 through with their bodies.
- `profile/` — `FaceArchitectureProfile` entity, repository, lookup, document
  validation.
- `classify/` — `Classifier` interface; `PrototypeClassifier` for v1.
- `traits/` — rule-list evaluation.
- `guidance/` — template resolution and rule selection.
- `handler.go` — the HTTP handler, registered in `cmd/server/main.go`.

### Profile entity

Modelled on `score_rulesets`, in a new core-engine-owned table added to
core-engine's `AutoMigrate` list (additive; see `docs/SHARED-DATABASE.md`):

```go
type FaceArchitectureProfile struct {
    ID            string     // uuid
    BrandID       string     // '*' = any
    ApplicationID string     // '*' = any
    Code          string
    Title         string
    Status        string     // ACTIVE | INACTIVE, normalised uppercase
    Version       int
    Document      string     // jsonb, schema below
    CreatedAt     time.Time
    UpdatedAt     *time.Time
}
// table: face_architecture_profiles
```

Lookup order is brand+app → brand+`*` → `*`+`*`, highest active version. No
active profile means **409 `no_active_face_architecture_profile`**; there are
no built-in defaults.

### Profile document

```json
{
  "schemaVersion": 1,
  "catalogueVersion": "fa-measure/1",
  "calibration": {
    "status": "uncalibrated",
    "benchmarkRunId": null,
    "target": null,
    "heldOutMetrics": null,
    "sources": [{"cite": "Zafar & Popa 2016", "for": "blendMargin"}]
  },
  "measurementsShown": ["width_length_ratio", "thirds_mid", "..."],
  "classifiers": [
    {
      "key": "faceShape",
      "blendMargin": 0.6,
      "minObservedWeight": 0.5,
      "classes": [
        {
          "label": "Oval",
          "requires": ["width_length_ratio"],
          "prototype": {
            "width_length_ratio": {"centre": 0.0, "spread": 0.0, "weight": 1.0}
          }
        }
      ]
    }
  ],
  "traits": [
    {
      "key": "noseWidth",
      "rules": [
        {"label": "Broad",  "when": [{"measurement": "alar_width_ratio", "op": ">", "value": 0.0}]},
        {"label": "Narrow", "when": [{"measurement": "alar_width_ratio", "op": "<", "value": 0.0}]}
      ],
      "default": "Average"
    }
  ],
  "guidance": {
    "templates": {
      "contour_jaw_sides": {
        "role": "contour",
        "points": [{"index": 172}, {"lerp": [172, 152, 0.3], "offsetIod": [0.0, -0.05]}]
      }
    },
    "rules": [
      {"id": "round-contour", "when": {"faceShape": ["Round"]},
       "apply": [{"template": "contour_jaw_sides", "intensity": 0.6}],
       "textKeys": ["facearch.guidance.round.contour"]}
    ]
  }
}
```

(Numbers above are shape examples, not values. Seed values are the subject of
Open literals.)

### Semantics

**Classifiers (prototype scoring).** For class *c* over its observed
measurements *m*:
`score_c = exp(-½ Σ_m w_m ((x_m − centre_m) / spread_m)²)`, then normalised
across assessable classes to sum to 1.

- A class whose `requires` includes an occluded or missing measurement is
  `not_assessable` and excluded from normalisation.
- If the observed share of total prototype weight is below
  `minObservedWeight`, or fewer than two classes are assessable, the result is
  `insufficient_evidence` with the missing measurements listed.
- If the top class's normalised score is below `blendMargin`, the result is a
  blend of the top two (`primary`, `secondary`); otherwise a single `primary`.
- The output always includes every assessable class's score.

Wardah's Face Balance Type, if a brand wants it, is a second classifier entry
using the same mechanism. No code change.

**Traits.** Rules are evaluated in order; the first whose conditions all hold
wins; otherwise `default`. Conditions on an occluded or unsupported measurement
make the trait `not_assessable`. If a condition's measurement band straddles
the threshold, the trait carries `boundaryUncertain: true` and the label the
other side of the threshold would give (`alternative`).

**Guidance.** Template points are a landmark index, or `lerp [a, b, t]`
between two landmarks, with an optional offset in IOD units in the canonical
frame. core-engine resolves them against the returned landmarks and maps back
to image pixels. Rules match on classifier results (primary, or primary or
secondary for a blend) and on trait labels. A template with any anchor in an
occluded region is dropped and listed in `droppedTemplates` with the reason.
Text is returned as i18n keys; the brand's content owns the words.

**Validation (on upsert and on load).** Every measurement key must exist and be
`supported` in the ai-worker catalogue for the document's `catalogueVersion`.
Every template index is 0-477. Spreads are positive; margins are in (0.5, 1].
At upsert a failure rejects the document. At load, catalogue drift since the
upsert marks the affected classifier or trait `unavailable` with a reason
rather than failing the whole request.

### Endpoint

`POST /api/v1/vision/face-architecture/:brandId/:applicationId`, multipart
`image`.

```json
{
  "quality": {"...": "passed through"},
  "regions": {"...": "passed through"},
  "measurements": ["only measurementsShown, as returned by ai-worker"],
  "classifications": {
    "faceShape": {
      "status": "blend",
      "primary": "Oval", "secondary": "Oblong",
      "scores": {"Oval": 0.52, "Oblong": 0.31, "Round": 0.10},
      "notAssessable": ["Heart"]
    }
  },
  "traits": {
    "noseWidth": {"label": "Average", "boundaryUncertain": true, "alternative": "Broad"}
  },
  "guidance": {
    "regions": [{"role": "contour", "template": "contour_jaw_sides",
                 "polygon": [[412, 880]], "intensity": 0.6,
                 "ruleId": "round-contour", "textKeys": ["..."]}],
    "droppedTemplates": [{"template": "highlight_forehead_centre", "reason": "forehead occluded"}]
  },
  "provenance": {
    "model": {"...": "..."},
    "catalogueVersion": "fa-measure/1",
    "profile": {"id": "...", "code": "seagull-default-uncalibrated", "version": 1},
    "calibration": {"status": "uncalibrated"}
  }
}
```

| Case | Status |
|---|---|
| ai-worker gate or face-count failure | 422, body passed through |
| ai-worker model unavailable or unreachable | 503 |
| No active profile | 409 |

### Profile CLI

`apps/core-engine/cmd/facearch-profile`:
- `validate <file.json>` — schema and catalogue validation, no database write.
- `upsert <file.json> --brand --app --code` — validate, then insert as the
  next version and activate it.

## Seed profile

`seagull-default-uncalibrated`, brand `*`, app `*`, shipped as a JSON file
under `apps/core-engine/cmd/facearch-profile/seed/` and loaded with the CLI.
`calibration.status` is `uncalibrated`, and every response shows it. Every
number cites the heuristic it came from in `calibration.sources`. It contains:
the face-shape classifier over Perfect Corp's eight classes; traits for
eyeSize, eyeAngle, eyeDistance, eyeShape, eyebrowShape, eyebrowDistance,
eyebrowShortness, lipShape, noseWidth and noseLength; and contour, highlight,
blush and brow templates and rules per face shape after Perfect Corp's
published contour guidance. It is replaced by the first calibrated profile.

## Benchmark and calibration harness

`apps/core-engine/cmd/face-benchmark`, a Go CLI. It calls the real ai-worker
for measurements and runs `facearch` classification in-process against a
profile **file**, so a candidate profile is scored before anyone upserts it.

### Inputs (local only; never committed)

- `manifest.csv`: `image_id, image_path, subject_id, consent_ref, hijab, notes`.
- `results.csv` (long format): `image_id, source, attribute, value`, where
  `source` is `perfectcorp`, `wardah`, `makeover` or `rater:<id>`. Collected by
  hand from each vendor's tool; nothing automates their products.
- `label-map.json`: vendor vocabulary → our classifier and trait labels.

### Subcommands

1. `measure` — call `/face-measure` for every image; cache results keyed by
   image SHA-256 + model version + catalogue version.
2. `evaluate --profile p.json` — per source × attribute: exact agreement, top-2
   agreement (vendor label in our blend), macro-F1, confusion matrix, Cohen's
   kappa ours-vs-source **and source-vs-source** (the practical ceiling), ECE
   of class scores, gate-rejection and `insufficient_evidence` rates split by
   `hijab`. Writes `report.json` and `report.md`, no images.
3. `calibrate --target <source|rater-consensus> --profile base.json` — dedup
   near-duplicate images by perceptual hash, split by `subject_id`, fit
   prototype centres and spreads per class and 1-D trait thresholds on the
   training split, evaluate on held-out, and write a profile with
   `calibration.status = calibrated`, the run id, the target and the held-out
   metrics.

## Testing

**ai-worker (pytest):**
- `facegeom` on hand-built landmark arrays with known answers: perfect ellipse,
  mirrored face (asymmetry 0), rotated face (roll correction), scaled face
  (IOD invariance), occluded region (value withheld).
- Noise bands are reproducible for the same image hash.
- Anchor mapping sanity on the CC0 `tests/tryon/fixtures/sample_face.jpg`:
  ordering relations that must hold on any upright face (mesh_top above
  glabella above subnasale above menton; exo outside endo; cheilion below
  alar). Skipped with a clear reason when the model file is absent.
- Router: 503 without a model, 422 on each gate, catalogue shape.

**core-engine (Go, testify):**
- Prototype scoring, normalisation, blend margin, `not_assessable`,
  `insufficient_evidence`.
- Trait rule order, `boundaryUncertain`, occluded conditions.
- Template resolution, pixel mapping, occlusion drop.
- Profile lookup fallback and 409; validation against a catalogue fixture.
- measureclient against `httptest`.
- Benchmark metrics, person split, dedup, label mapping and calibration fitting
  on a synthetic cached-measurement fixture.

## Build order

1. Landmarker, manifest, `facegeom`, `/face-measure` and catalogue. Needs the
   `face_landmarker.task` download (≈3.6 MB, Apache-2.0, Google storage),
   approved by the user at that step.
2. core-engine `facearch`, profile table and CLI, uncalibrated seed. **The
   endpoint works end to end here.**
3. Guidance templates and rules.
4. Benchmark harness.

## Open literals

Per AGENTS.md "Do not hardcode", these values have no validated source today.
Each is config or profile data, named, documented and defined once:

| Value | Where | Proposed default and source |
|---|---|---|
| Pose gate yaw / pitch | ai-worker env | 15° / 15°. Product policy: the ICAO ±5° (U) is too strict for selfies; 3D tolerance studies (±16° yaw, U) do not transfer to 2D. To be tightened from benchmark gate-rejection data. |
| Minimum eye-centre distance | ai-worker env | 60 px. Keeps the model card's ~3% IOD error under ~2 px. Policy. |
| Noise samples | ai-worker env | 64. Compute cost vs band stability; no accuracy claim. |
| Occlusion ΔE threshold | ai-worker env | No default. While unset the detector reports every region `unknown` and adds a `visibility_detector_unconfigured` warning, so values are computed but never presented as observed. Set it from the fixture and the first benchmark images (hijab slice). |
| Seed profile centres, spreads, trait thresholds, templates | seed JSON | Consumer face-shape heuristics and Perfect Corp's published contour guidance, each cited in `calibration.sources`; labelled uncalibrated in every response. |
| `blendMargin` | profile | 0.6, Zafar & Popa 2016. |

Facts kept in code or the model manifest, not config: the anchor mapping
(MediaPipe topology), the eye-centre definition and landmark error (model
card), the 8° roll limit (model card).

## Known limitations

- **Perspective is not corrected.** Close selfies inflate central widths
  relative to the jaw. v1 returns a `perspective_not_corrected` warning on
  every result; a distance estimate needs camera intrinsics (EXIF focal length)
  and is deferred.
- **Hijab wearers get fewer answers.** Forehead-dependent measurements and the
  classes that need them are withheld, and face shape is more often a blend or
  `insufficient_evidence`. This is deliberate; guessing a hidden forehead is
  the defect this design exists to avoid.
- **Accuracy is unknown until the benchmark runs.** Expect geometry-only
  agreement with any single vendor in the 64-80% range the literature reports,
  bounded above by how well the vendors agree with each other.

## Lanes

`ai-worker-vision` (new files under `vision/`, `tests/vision/`),
`ai-worker-core` (settings), new lane `core-facearch`
(`apps/core-engine/internal/facearch/`, `cmd/facearch-profile/`,
`cmd/face-benchmark/`), and a one-line route registration in
`apps/core-engine/cmd/server/main.go`.
