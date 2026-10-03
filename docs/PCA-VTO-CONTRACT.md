# PCA + VTO (photo mode) — API contract

Status: **implemented (photo mode), first iteration.** Measurement matches the
WCPA notebook exactly on the golden photo
(`apps/workers/colour/tests/test_wcpa_golden.py`).

Scope: photo mode only — upload photo → personal colour analysis (WCPA) →
quadrant → shade recommendations → photo try-on. Live camera try-on
(`beauty-sdk` `LiveTryOnCanvas`) is out of scope for this contract except for
the shared render parameters in §4.

## 1. Who does what

| Concern | Service | Why |
|---|---|---|
| Face detection, skin colour measurement, CIELAB → Munsell (V, C, hue), makeup rendering | the colour worker (Python, :8092; `apps/workers/colour/worker_colour/`) | Pixel work stays in Python. Munsell conversion needs `colour-science` (ASTM D1535 renotation), which has no Go equivalent; porting it would diverge from the notebook. PCA and VTO share one profile by decision. |
| WCPA classification (quadrant snap, temperature, complementary rule, foundation band), presentation labels, shade lookup, recommendations | `core-engine` (Go, :8082), domain `internal/colour` | Rules, thresholds and product data stay in Go, driven by config. |
| Upload, results, shade picker, before/after | `seagull-web` | Frontend never calls the colour worker directly. |

Code: `apps/core-engine/internal/colour/` (Go) and the colour worker
(`apps/workers/colour/worker_colour/`, tests in `apps/workers/colour/tests/`).

Why a worker of its own: the WCPA code needs only the CPU (MediaPipe and
numpy), while the tryon worker is the torch/GPU one (its own Dockerfile,
compose profile `gpu`). A separate colour worker keeps torch out of colour
analysis; each worker is its own process and image.

Config: core-engine reaches the worker through `COLOUR_WORKER_URL` (default
`http://127.0.0.1:8092`, `docs/PORTS.md`), loaded in
`internal/config/config.go` with the other tunables. `TRYON_ENGINE_URL` (shade
extraction, tryon worker), `SKIN_WORKER_URL` and `FACE_WORKER_URL` are left
untouched — they point at other workers.

## 2. Colour worker (Python, internal only)

The engine loads once at startup (`npm run dev:colour`). No endpoint here is exposed through the gateway.
`GET /health` reports `checks.wcpa` `up` or `down` (required: `down` answers
503) and, when up, `checks.wcpa_chroma`, the Munsell chroma method.

### `POST /pca/analyze`
Request: `multipart/form-data`
- `image` (JPEG/PNG)
- `hijab` (`true|false`) and `hairVisible` (`true|false`) — **asked from the
  user, not detected**. Colour heuristics cannot tell a dark hijab from dark
  hair, and guessing wrong feeds fabric colour in as hair colour
  (`pca_pipeline_v3` §5).

Response `200` (measurement layer of the WCPA notebook, §4 + §1):
```json
{
  "face": { "detected": true, "landmarkSource": "mediapipe_face_landmarker" },
  "measurement": {
    "skinLab": [62.1, 12.4, 17.8],
    "lipLab": [48.0, 32.1, 14.0],
    "irisLab": [30.2, 4.1, 8.8],
    "hairLab": null,
    "lPerSite": { "forehead": 63.0, "cheek_r": 61.8 },
    "patchesUsed": ["forehead", "cheek_r", "cheek_l"],
    "illuminant": { "method": "shades_of_gray", "residual": 0.6 }
  },
  "munsell": {
    "skin": { "V": 6.11, "C": 4.34, "hueFamily": "YR", "hueAngle": 55.1, "chromaSource": "approximation" },
    "lip":  { "V": 4.71, "C": 7.0,  "hueFamily": "R",  "hueAngle": 23.6, "chromaSource": "approximation" }
  },
  "quality": { "qcFailed": [], "degraded": [] },
  "pipelineVersion": "wcpa-notebook-port-0.1"
}
```
- All Lab values CIELAB D65. Field set mirrors the notebook's `ukur()` output.
- `chromaSource` says how Munsell chroma was computed: `approximation`
  (C*ab / `CHROMA_PER_UNIT` 5.0, the notebook's placeholder, and the default)
  or `colour-science` (the Munsell renotation, with
  `WCPA_CHROMA_METHOD=colour-science`). With colour-science, a colour it
  cannot convert falls back and says `approximation`. `/health` reports the
  configured method (`checks.wcpa_chroma`). See §8 note 7.
- `landmarkSource` is always reported. If the real MediaPipe model is missing
  the endpoint **fails** (`503 model_unavailable`) — it never falls back to
  synthetic landmarks.
- `quality.qcFailed` are the notebook's QC codes (`terlalu_gelap`, `menoleh`,
  `wajah_besar`, ...). The pipeline still returns a measurement; callers
  decide whether to ask for a retake.

Errors: `400 invalid_image`, `422 no_face_detected`, `503 model_unavailable`.

### `POST /vto/render`
Request: `multipart/form-data`
- `image`: the user photo
- `request`: JSON string — one layer per category, applied in the order
  complexion → blush → brow → eyeshadow → eyeliner → mascara → lip:
```json
{ "layers": [ { "category": "lip", "hexColor": "#B5476A", "alpha": 0.75, "specular": 0.3, "mode": "" } ] }
```
| Category | Engine |
|---|---|
| lip | WCPA §9 `render_vto` (product colour × exposure × shading, texture kept) |
| eyeshadow | **new** (`worker_colour/wcpa/eyeshadow.py`): lid wash in the eye's own frame, from the lash line up toward the brow, darkest at the lash line, extended past the outer corner as a soft wing; styles `natural` (default), `soft_glam` (+ outer V, outer lower line), `smoky` (to the crease and beyond, deep outer V, full lower line) by `mode`. Colour as a pressed-powder pigment: scattered powder plus skin filtered by the shade, with transmittance √R (light crosses the powder twice), so navy, black and deep brown darken the lid instead of tinting it; lid texture kept. `specular` adds a sheen on the lid centre and glitter specks fixed in the eye's own frame (a hash of a cell grid, so they follow the eye in video) |
| blush | **new** (`worker_colour/wcpa/blush.py`): soft teardrop from the apple of the cheek toward the cheekbone at the hairline, clear of the under-eye, the nose side of the pupil line and the lips; styles `lifted` (default), `apple`, `draping` by `mode`. Colour as a translucent pigment: mostly the skin filtered by the shade (deeper rose on dark skin, never chalky), a little scattered product colour, skin texture kept, `specular` adds glow on the cheekbone highlight |
| brow | **new** (`worker_colour/wcpa/brow.py`): "defined brow" — soft-angled MUA template (head on the nostril line, arch at two-thirds, tail on the nostril–outer-corner line and never below the head); own hair tinted; the shape filled as an ombre (soft light head, crisp dark body and tail) with a procedural hair texture; fill stronger for sparse / bald brows; stray hair on full brows inpainted unless the brow is simply bigger than the template; multiply blend. Rendered even when the WCPA hair matte is empty (bald). Catalog `mode` no longer changes the brow look. Report per brow: `full` / `sparse` / `tail_missing` / `bald` + coverage + erased px |
| eyeliner | **new** (`worker_colour/wcpa/liner.py`): styles `thin`, `classic` (default), `cat_eye`, `smoky`, chosen by the layer's `mode`. Line thin at the inner corner, thickening outward; the wing is a filled triangle whose upper edge joins the line a third to a half of the way in from the outer corner, so it grows out of the liner (reference winged-liner photos) |
| mascara | `mask_vto_2` §6 lash strip geometry, pulled along a spline lash line and sampled without aliasing (`worker_colour/wcpa/lashes.py`). Templates drawn in memory by default (`LASH_TEMPLATE_SOURCE=procedural`): long lashes (capped at 1.1× the eye opening, so narrow eyes stay natural) combed into spikes with some short and stray strands, fanning around the eye, plus coated lower lashes; `files` loads PNGs instead (EyelashNet via `worker_colour/tools/build_lash_templates.py`). Styles `natural` (default), `volumizing`, `lengthening` by `mode` |
| complexion | **new** (`worker_colour/wcpa/complexion.py`), phase 1b: evens the tone (spots, redness and dark patches covered via a median tone layer; pores softened, not erased; under-eye dip lifted). Shade: toward the swatch as the photo would show it (× exposure) — hue/chroma 0.55, lightness 0.3 at catalog alpha 0.35 (neutral gain in linear light, so shading keeps its ratios), calibrated by eye against Perfect Corp's foundation try-on; lightness at 0.85 read as a painted mask. Lifts the phase-1a lock (α ≤ 0.35, L\* unchanged) at the product owner's request, 2026-09-29; `SHADE_L = 0` restores it. Finish from `specular`: 0 matte (shine down) … 0.05 natural … 0.25 dewy (glow on the high points). Skin only: hair, beard, hijab and nostrils inside the face oval are left alone; the oval is raised to the hairline |

Response `200`: `image/png`, at the notebook's working size (`load_img`: longest
side ≤ 1400 px). Headers `X-Pipeline-Version`, `X-Render-Log`.

Errors: `400 invalid_image | unsupported_region | invalid_hex`,
`422 no_face_detected`, `503 model_unavailable`.

### `POST /colour/munsell`
`{"hexes": ["#RRGGBB", ...]}` or `{"labs": [[L, a, b], ...]}` → Lab + Munsell
through the same §1 code. Used by the catalog seeder so shades and people share
one conversion.

### `GET /vto/render-params`
The render recipe (§4) as JSON, for a live renderer. Needs no model.

## 3. core-engine (Go, through the gateway)

Mounted like the existing engines: `/core/colour-engine/*` (what the gateway
routes to; registered in Seagull `gateway-engine/cmd/seed-routes`, folder
"Colour Analysis (WCPA)") and `/api/colour/*`. The web app reaches it the same
way as the other studios: `/core/...` → Next proxy → gateway-proxy → core-engine. Errors follow the sentinel pattern: `400` validation, `404` unknown
shade, `422` no face, `503` worker unavailable. The body is the service's
error envelope plus `code`: `{"success": false, "error": "…", "error_code":
"no_face_detected", "code": "no_face_detected"}` (`code` repeats `error_code`
because seagull-web's colour client reads it). While the WCPA config cannot
load, the colour routes answer `503 colour_engine_disabled` with the reason,
and `/health` on the colour prefixes answers 503 in the service's health shape
(`"status": "degraded"`, `"checks": {"wcpa-config": "down"}`) with `error` and
`code` added; otherwise `/health` is the service's shared health check.

### `POST /api/colour/analyze`
Request: `multipart/form-data`, fields `image`, `hijab`, `hairVisible`.

Response `200` (classification + presentation layers; measurement stays internal):
```json
{
  "quadrant": {
    "code": "HV_LC",
    "technicalName": "High Value x Low Chroma",
    "displayName": null,
    "provisional": true
  },
  "complementary": { "codes": ["HV_HC"], "temperatureRule": "neutral_to_cool", "provisional": true },
  "labels": {
    "undertone": "netral",
    "value": "terang",
    "chroma": "lembut",
    "seasonEquivalents": ["Light Summer", "Light Spring"]
  },
  "foundationBand": "light",
  "flags": ["uneven_skin_tone"],
  "qualityFailed": [],
  "retake": { "required": false, "reasons": [], "provisional": true },
  "recommendations": {
    "lip": [
      {
        "shadeId": "shd_…", "productId": "prd_…", "productName": "…", "shadeName": "…",
        "hexColor": "#B5476A", "hueName": "strong purplish red",
        "status": "same_quadrant", "colourSource": "swatch"
      }
    ],
    "complexion": [
      {
        "shadeId": "shd_…", "productId": "prd_…", "productName": "…", "shadeName": "…",
        "hexColor": "#E3B9A0", "hueName": "light yellowish pink",
        "status": "same_quadrant", "colourSource": "swatch",
        "skinDeltaE00": 1.42, "skinDeltaEBand": "close_inspection", "withinTolerance": true
      }
    ]
  },
  "catalog": {
    "mascara": [
      {
        "shadeId": "shd_…", "productId": "prd_…", "productName": "…", "shadeName": "Black",
        "hexColor": "#000000", "hueName": "",
        "status": "", "colourSource": "estimate", "mode": "lengthening"
      }
    ]
  },
  "configVersion": "wcpa-2026-09-28",
  "debug": null
}
```
- `retake`: whether the photo should be retaken. `reasons` are the worker's
  failed QC checks (`qualityFailed`) that config `retake.codes` lists, in config
  order (default `terlalu_gelap`, `terlalu_terang`, `cahaya_campuran`: the
  illumination ones); `required` is `reasons` non-empty; `reasons` is `[]`,
  never null. `provisional` is `true` while the worker's QC thresholds are
  `[KARANGAN]` (the UI must show it as provisional). `qualityFailed` stays the
  worker's full list. With `retake.enforce` off (default) the result is returned
  as usual and the UI decides. With it on and `required`, `/analyze` answers
  `422` with no shade result:
  ```json
  {"success": false, "error": "retake required: terlalu_gelap", "error_code": "retake_required",
   "code": "retake_required", "message": "retake required: terlalu_gelap",
   "reasons": ["terlalu_gelap"]}
  ```
- `provisional: true` while the person thresholds are placeholders
  (`[KARANGAN]` in the notebook). The UI must show it.
- `displayName` stays `null` until the brand names the quadrants; the UI then
  shows the technical name marked as such.
- Recommendations are grouped per category and ordered as in the notebook
  (same quadrant, then complementary with an allowed temperature; within a
  group by distance to the person's V/C), at most `recommendation.perCategory`.
  Only the categories in config `recommendation.categories` (lip, blush,
  complexion, eyeshadow) are rated; see §8.
- `catalog` is **every** shade of every category, for the try-on: products by
  name, each product's shades light to dark (L\*). `status` is this person's
  rating (`same_quadrant`, `complementary`) or `""` (outside both, or a
  category that is not rated). `mode` is the render style (mascara type,
  eyeliner and eyeshadow look). Without it, a category with no kept shade
  (black mascara for most people) could not be tried at all.
- `hueName` only for lip, blush and complexion (config `hueNames.categories`);
  `""` elsewhere. It is the ISCC-NBS level-3 colour name (Kelly 1958, NBS
  RP2911) of the nearest of the 267 block centroids by CIEDE2000, e.g.
  "vivid red", "light yellowish pink". Nearest centroid approximates block
  membership near block boundaries (§8, item 6).
- `skinDeltaE00`, `skinDeltaEBand`, `withinTolerance`: only for categories
  meant to match the skin, config `skinMatch.categories` (complexion); absent
  elsewhere, because a lipstick is meant to differ from skin.
  `skinDeltaE00` is the CIEDE2000 difference between the shade and the measured
  skin, 2 decimals. `skinDeltaEBand` says how visible that is:
  `not_perceptible` (< 1.0), `close_inspection` (1.0–2.0), `noticeable`
  (2.0–5.0), `at_a_glance` (≥ 5.0). These bands are properties of human vision
  and fixed in code (`internal/colour/domain/perceptibility.go`).
  `withinTolerance` is present only when the brand set `skinMatch.maxDeltaE00`
  (policy): `true` when `skinDeltaE00` is at most that tolerance. The order of
  recommendations does not change; this is extra information about each shade.
- `colourSource`: `cube`, `swatch` or `estimate`. Swatch-derived shades are
  flagged; `estimate` is a demo colour, not from the brand, and the UI says so.
- Raw colorimetry (the user's `skinLab`) is **never** in this response unless
  the request carries `?debug=1` **and** core-engine runs with
  `COLOUR_DEBUG_ENABLED=true`; then it appears under `debug`.
- `hexColor` is the product swatch, not a measurement of the user.

### `POST /api/colour/tryon`
Request: `multipart/form-data`, fields `image` and `shadeIds` (repeated, or
comma-separated; at most one shade per category — a full look in one call).
Go resolves each shade's hex, alpha, specular and mode from the catalog — the
frontend never sends colour values. Response: `image/png` from the colour worker.

### `GET /api/colour/catalog`
Every shade to try, grouped by category, **without** an analysis (try-on
straight from a photo): `{"catalog": {…}, "configVersion": "…"}`, the same
entries as `catalog` above with `status` always `""`.

### `GET /api/colour/quadrants`
Quadrant codes, labels and `provisional` flag, for the UI legend.

## 4. Shared render parameters (`render_params.json`)

Lives in `apps/workers/colour/worker_colour/render_params.json`
(`RENDER_PARAMS_PATH` to use another file). Single source for the render's named constants (the ones each
module declares at its top), so the Python photo renderer and the future
WebGL live renderer read the same file. Language-neutral JSON with a `version`
field; one section per layer (`complexion`, `complexion_mask`, `blush`,
`brow`, `eyeshadow`, `eyeliner`, `mascara`, `lashes`, `lip`), `notebook` (the
WCPA cells' render values, written into the cells' namespace at load), `roi`
(the frame region each layer is rendered in) and `video` (landmark smoothing,
exposure easing). Keys are the Python constant names; their meaning is
documented where each module reads them. `tests/test_wcpa_params.py` checks
every value in the file is the value the code uses. Smaller details inside
the render functions (bounding-box pads, blur floors, thresholds) are still
literals in the code; they move into the file as each layer is ported to
WebGL. Video and the live plan: `docs/VIDEO-RENDER.md`.

## 5. Data model (core-engine, owns `match_*` tables)

`match_shades` is **not changed**. Products now live in `ref_products` (owned by
reference-service; `match_product_catalog` is retired). The seeder writes
`ref_products`, `match_shades` and `match_shade_colorimetry` (`Shade.Region` mapped
to lip/eye/cheek/skin; the exact category lives on the colorimetry row).

**New table `match_shade_colorimetry`** — the shade's colour in WCPA terms,
kept separate from the shade because it is re-measured (swatch → cube) and must
say where each number came from:

| column | type | note |
|---|---|---|
| `id` | varchar(64) PK | |
| `shade_id` + `source` | varchar, unique together | → `match_shades.id`; `cube` \| `swatch` \| `estimate` — best wins, in that order |
| `category` | varchar(32) | lip, blush, brow, eyeliner, mascara, eyeshadow, complexion |
| `lab_l`, `lab_a`, `lab_b` | float | CIELAB D65 |
| `munsell_v`, `munsell_c`, `chroma_source` | float, varchar | |
| `alpha`, `specular`, `mode` | float, float, varchar | render parameters from the catalog CSV |

The shade's quadrant is **computed**, not stored: Go snaps (V, C) to the
shade centres from config. When RnD supplies an explicit mapping, it is added
as an override table in the same shape (`source = rnd_mapping`) — no change to
this one.

**WCPA rules** — `apps/core-engine/config/wcpa.json`, path overridable with
`WCPA_CONFIG_PATH`. Every number is copied from the notebook with its
provenance tag kept:

| Block | Values | Tag in notebook |
|---|---|---|
| Five quadrants, person centres + scale | `PUSAT_ORANG`, `SKALA_ORANG` | `[KARANGAN]` → `provisional` |
| Person value/chroma group cuts | `PERSON_VALUE_*`, `PERSON_CHROMA_*` | `[KARANGAN]` → `provisional` |
| Shade V/C bands | Value 7–10 / 4–6.5 / 1–3.5, Chroma 6–9 / 3–5.5 | WCPA "Shade Classification Score" slide (newer than the notebook's) |
| Shade centres | band midpoints; MM chroma 5.75 | derived — confirm with colourist |
| Temperature from b\*−a\* | 5.6 / 4.4 | `[WCPA]` |
| Foundation L\* bands | 70 / 66 / 63 / 60 | `[WCPA]` |
| Complementary map | `KOMPLEMENTER` | `[INTERPRETASI]` → `provisional` |
| Hue names | ISCC-NBS level 3, nearest centroid (CIEDE2000) | standard (Kelly 1958); replaced the brief's `WARDAH_HUE` |
| Hue-name categories | lip, blush, complexion | service choice (§8) |
| Rated categories | lip, blush, complexion, eyeshadow | service choice (§8) |
| Presentation: quadrant names, season table, labels | `NAMA_KUADRAN`, `SEASON_PADANAN` | brand / colourist to confirm |

The file carries a `status` per block. Any block still `placeholder` or
`interpretation` makes the API mark its output `provisional`.

## 6. Product data

Seeded from the notebook's existing catalog `vto_products.csv` (columns
`kategori, produk, shade, hex, alpha, specular, mode, sumber`, optional
`brand`, and `munsell_v, munsell_c` from cube) into `ProductCatalogItem` +
`Shade` + `match_shade_colorimetry` through an idempotent seeder
(`cmd/seed-colour-shades`, same pattern as the existing seeders).

- `sumber` `[SWATCH]` → `source = swatch`; `[PERKIRAAN]` → `source = estimate`;
  anything else stops the seeder. Cube readings → `source = cube`. A hex row
  replaces the other hex row of the same shade (a swatch replaces an
  estimate); cube rows are never removed by it.
- `brand` names the row's brand; `-brand-map "Make Over=<id>,Wardah=<id>,OMG=<id>"`
  gives the gateway brand ids. Rows without a brand use `-brand`; a brand
  missing from the map stops the seeder.

**Demo catalog** `apps/core-engine/seed/vto_products_demo.csv`: the notebook's
187 Make Over swatch rows plus 48 `[PERKIRAAN]` rows, so every section (brow,
lip, complexion, blush, eyeliner, eyeshadow, mascara) has several products to
try. Product and shade names are the brands' own; the estimated colours, how
they were made, and the sources are in `seed/vto_products_demo.md`.

## 7. Porting approach (Python)

Step 1 — **verbatim**: the measurement cells (`pca_pipeline_v3` base cells +
`# PIPELINE 03`–`08`) and the WCPA §1/§9/§9b functions are copied unchanged
into `apps/workers/colour/worker_colour/wcpa/` and driven the same way the WCPA notebook's
`ukur()` drives them. Golden test: notebook output JSON == service output for
the same photos.

Step 2 — refactor into plain functions behind the same golden test. Render
constants move into `render_params.json` at this step.

## 8. Deliberate differences from the notebooks

1. Brows: neither notebook's brow is used. The WCPA opaque brow painted the
   product colour over the brow (normal blend), which turned taupe swatches
   into grey stickers; the service multiplies pigment onto the wearer's own
   brow inside a rule-based shape instead. The shape's arch is placed at
   two-thirds of the length rather than on the notebook's iris line, which
   landed at 85–90 % on every test photo and hooked the tail down.
2. Recommendation order: the notebook sorted by the status string, which put
   complementary shades before same-quadrant ones; same quadrant comes first.
3. Shade bands: from the newer WCPA slide, not the notebook's `SHADE_*`.
4. Complexion render is new (no notebook had one).
5. Rated categories: the notebook rated every category, so black mascara and
   eyeliner and brown brow products were kept or dropped by a skin quadrant
   that says nothing about them. Config `recommendation.categories` rates lip,
   blush, complexion and eyeshadow; the rest are offered unrated in `catalog`.
   An empty list restores the notebook.
6. Hue names: the brief's families went by hue angle only, so they named
   saturated reds "Peach" (94 of 162 catalogue lip shades on 2026-10-01; a
   vivid red and a peach share a CIELAB hue angle and differ in lightness and
   chroma) and mislabelled eye products (black mascara "Red"). They are
   replaced by ISCC-NBS level-3 names, from the nearest of the standard's 267
   block centroids under CIEDE2000 (`internal/colour/domain/isccnbs`). That is
   an approximation: the standard defines blocks by Munsell boundaries, and a
   colour near a boundary can be nearer a neighbouring centroid; the 32
   open-chroma ("+") blocks have a lower-bound centroid. Config
   `hueNames.categories` still names lip, blush and complexion only.
7. Munsell chroma: the notebook's colour-science path never ran. It called
   `colour.xyY_to_munsell_specification`, which colour-science (every release
   up to 0.4.7) has only in `colour.notation.munsell`; the AttributeError was
   swallowed and every chroma was the approximation (C*ab / 5), with the
   package installed or not. The golden export says so
   (`chroma_sumber: pendekatan`). Had the call worked, it would also have read
   `spec[1]` (the value) and skipped the D65 → C adaptation the renotation
   needs. `cells/wcpa_munsell.py` fixes all three (`[PORT]`; **the notebook
   needs the same fix**). The service still defaults to the approximation
   (`WCPA_CHROMA_METHOD=approximation`): the person thresholds (`[KARANGAN]`,
   waiting for `cube_readings.csv`), the seeded shade chroma and every notebook
   result were made with it. Switch to `colour-science` as part of that
   calibration: set the thresholds and re-seed the shades with the same
   method, or people and shades are compared on different scales.
8. Try-on detects the face with the detector alone, not the notebook's full
   `qc_measure`: a face touching the frame edge made a later quality step
   fail, and try-on answered 500.

## 9. Open

1. Person thresholds and complementary map — calibrate from the October cube data.
2. MM chroma centre for shades (5.75) — colourist to confirm.
3. Brow hair strands: the hair texture is procedural (line-integral
   convolution on a strip in the brow's own frame). Individually drawn hair
   strokes (microblading look) need real strand templates (EBStore);
   `brow.hair_strip` is the function to replace.
4. Demo catalog: 48 shades carry estimated colours (`[PERKIRAAN]`) until the
   brands send hex codes; eyeshadow has no swatch rows yet.
5. Quadrant display names — brand.
