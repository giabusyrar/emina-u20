# Tryon shade textures via presigned links — design

Date: 2026-09-29
Status: approved in chat, pending spec review

## Problem

Tryon uploads each shade's texture maps to S3-compatible storage (Cloudflare
R2) and hands core-engine permanent public URLs, which core-engine stores and
serves to the web app. That requires the bucket to be publicly readable. On R2
public reads only work through `*.r2.dev` or a custom domain, and `*.r2.dev` is
DNS-blocked by Indonesia's Internet Positif. The S3 API host
(`<account>.r2.cloudflarestorage.com`) is not blocked, and presigned links
point at it.

Goal: the bucket becomes private; the web app receives short-lived presigned
links; nothing stored in the database is a URL.

## Decisions

| Question | Decision |
|---|---|
| Where links are signed | core-engine, locally, with `aws-sdk-go-v2`'s presign client. Not ai-worker: a shade-asset read must not depend on the GPU worker being up, and signing is local HMAC work with no network call. |
| Existing rows holding public URLs | One-time migration converts them to keys, then `r2.dev` is disabled. |
| Link lifetime | `S3_PRESIGN_TTL`, default `15m`. The web app fetches the asset JSON and loads the images immediately; it never keeps the URL. |
| Web contract | Unchanged. `GET /shade-assets/:id` still returns `colorMapUrl`, `alphaMapUrl`, `finishMapUrl?`. |

Rejected: core-engine calling a new ai-worker presign endpoint (runtime
coupling to the heavy worker, a service hop per read, reverses the loopback
cleanup); an ai-worker redirect endpoint (ai-worker would have to be publicly
reachable and serve browser traffic).

## Current flow (for reference)

1. `ai_worker/tryon/routers/extraction.py` `run_extraction_job` uploads
   `shades/{shade_id}/{color,alpha,finish}.png` through
   `ai_worker/tryon/services/storage.py` `upload_bytes`, which returns
   `ai_worker.core.storage.public_url(key)`.
2. It POSTs `{"status":"ready","colorMapUrl","alphaMapUrl","finishMapUrl"}` to
   `{MATCH_ENGINE_URL}/api/matching/shades/{id}/extraction-callback`.
3. core-engine `MatchHandler.ExtractionCallback` saves an `entity.ShadeAsset`
   (`match_shade_assets`: `color_map_url`, `alpha_map_url` NOT NULL,
   `finish_map_url`).
4. Seagull-web `beauty-sdk` `LiveTryOnCanvas.tsx` fetches
   `/api/matching/shade-assets/:assetId` and sets `img.src = colorMapUrl` with
   `crossOrigin = 'anonymous'`.

Ownership: `match_shade_assets` is migrated only by core-engine. Seagull-gateway
has no model for it; Seagull-web reads it only through the endpoint.

## Design

### 1. Data and contracts

**Table `match_shade_assets`.** `entity.ShadeAsset` replaces the three URL
fields with `ColorMapKey`, `AlphaMapKey`, `FinishMapKey` (columns
`color_map_key`, `alpha_map_key`, `finish_map_key`, `varchar(512)`, nullable
so `AutoMigrate` can add them to a populated table). These fields carry
`json:"-"`: stored keys are never serialized. The old `*_url` columns are
dropped by the migration command (section 3), not by `AutoMigrate`, which
cannot drop or rename.

New columns rather than reusing the old ones: a column named `color_map_url`
holding `shades/9/color.png` would mislead every reader of the data.

**Tryon → core-engine callback.** Payload becomes
`{"status":"ready","colorMapKey","alphaMapKey","finishMapKey"}`. `ready`
without `colorMapKey` and `alphaMapKey` is a 400. `*Url` fields are no longer
read. `failed` is unchanged. Both ends live in this repository and ship
together.

**core-engine → web.** `GET /shade-assets/:id` responds with the same shape as
today:

```json
{"success": true, "asset": {"id", "shadeId", "colorMapUrl", "alphaMapUrl", "finishMapUrl?", "createdAt"}}
```

with each URL presigned on this request. The response is built from a
response type, not by serializing the entity.

**ai-worker.** `ai_worker/tryon/services/storage.py` `upload_bytes` returns the
key. `ai_worker.core.storage.public_url` and the `S3_PUBLIC_URL` setting (and
their tests) are removed; nothing in ai-worker needs a public URL any more.
Vision's `presigned_download_url` is unchanged.

### 2. Signing in core-engine

**Package `apps/core-engine/internal/match/adapters/objectstore`.**

```go
type Presigner interface {
    Presign(ctx context.Context, key string) (string, error)
}
```

The implementation wraps `s3.NewPresignClient` from
`github.com/aws/aws-sdk-go-v2/service/s3`, configured with the endpoint,
static credentials, region, path-style addressing and the TTL. `New` returns
`nil, nil` when S3 is not configured, so callers can tell "unconfigured" from
"broken".

**Config (`apps/core-engine/internal/config`).** Adds `S3Endpoint`,
`S3Bucket`, `S3AccessKeyID`, `S3SecretAccessKey`, `S3Region` (default `auto`,
matching ai-worker's `DEFAULT` and R2's requirement), `S3PresignTTL` (default
`15m`). The same env names ai-worker already reads, so one `.env` serves both.
S3 is optional: core-engine boots without it. "Configured" means bucket,
access key and secret are all set, the same test as ai-worker's
`s3_configured`. An unparseable `S3_PRESIGN_TTL` is a startup error, like the
other duration settings.

**Wiring.** `cmd/server/main.go` builds the presigner and passes it to the
match service/handler. `GetShadeAsset` behaviour:

| Situation | Response |
|---|---|
| S3 not configured | 503 `object storage not configured`. Never a fake or unsigned URL. |
| Row has no `color_map_key`/`alpha_map_key` (migration not run) | 409 `shade asset has no storage keys; run the shade-asset-keys migration` |
| Presign fails | 500 generic message; the error is logged, not echoed. |
| No `finish_map_key` | `finishMapUrl` omitted. |
| Asset not found | 404, unchanged. |

### 3. Migration, bucket setup, testing

**Command `apps/core-engine/cmd/shade-asset-keys`.** One-off, same shape as
`cmd/facearch-profile` (reads `.env` via goplatform, connects to
`DATABASE_URL`).

- Dry run by default: prints, per row, each `*_url` and the key it maps to.
  `-apply` writes.
- Recognised URL forms (prefixes from env, overridable by flags; nothing
  hardcoded):
  - `{S3_PUBLIC_URL}/{key}` — `S3_PUBLIC_URL` is read here and nowhere else,
    only to recognise legacy rows.
  - `{S3_ENDPOINT}/{S3_BUCKET}/{key}` — the form written when `S3_PUBLIC_URL`
    was unset.
- Any row whose URL matches neither form is listed and the command exits
  non-zero without writing anything.
- Rows that already have keys and no URL are skipped, so re-running is safe.
- With `-apply`, in one transaction: backfill the key columns, then
  `ALTER TABLE match_shade_assets DROP COLUMN color_map_url, DROP COLUMN
  alpha_map_url, DROP COLUMN finish_map_url`. The drop is recorded in
  `docs/SHARED-DATABASE.md` as a core-engine-owned change.
- If the URL columns no longer exist, the command reports that the migration
  is already done and exits 0.

- `-prepare` only drops the NOT NULL constraints on `color_map_url` and
  `alpha_map_url`. It exists because the new entity no longer writes those
  columns, so a callback insert into the old schema would fail until they are
  nullable or gone.

Rollout order:

1. `shade-asset-keys -prepare`.
2. Deploy ai-worker and core-engine together. `AutoMigrate` adds the nullable
   key columns; new extractions store keys.
3. `shade-asset-keys` (dry run), then `shade-asset-keys -apply`.
4. Disable the bucket's `r2.dev` public URL.

Between 2 and 3, old rows answer 409 rather than a broken image.

**Bucket setup (operator, Cloudflare dashboard; documented in
`docs/OBJECT-STORAGE.md`).**

- CORS rule on the bucket allowing `GET` (and `HEAD`) from the web app's
  origins. Without it `crossOrigin="anonymous"` loads from
  `*.r2.cloudflarestorage.com` fail.
- After `-apply`: disable the `r2.dev` public URL.
- Rotate the R2 API token that was shared in chat.

**Testing.**

- Go, `objectstore`: fake credentials; the URL's host is the endpoint host,
  the path is `/{bucket}/{key}`, `X-Amz-Expires` equals the TTL in seconds;
  `New` returns nil when unconfigured.
- Go, handler: fake presigner, each row of the table in section 2.
- Go, callback: `ready` stores keys; `ready` without keys is 400; `failed`
  unchanged.
- Go, migration: both URL forms map to keys; an unrecognised URL aborts with
  nothing written; dry run writes nothing; re-run after apply is a no-op.
  Key-mapping logic is a pure function tested without a database.
- Go, config: TTL default, parsing, invalid value; S3 optional.
- Python: tryon `upload_bytes` returns the key; the callback payload carries
  `*Key` fields; `public_url`/`S3_PUBLIC_URL` tests removed.
- End to end against Neon + R2 (`.env`): upload a texture through the
  ai-worker storage code, send the callback, `GET /shade-assets/:id`, fetch the
  signed URL (expect 200), re-fetch after expiry with a short
  `S3_PRESIGN_TTL` (expect 403), delete the probe object and row.

## Out of scope

- Seagull-web changes (none needed; its CORS dependency is bucket config).
- Caching signed URLs. Signing is cheap; revisit only if profiling says so.
- Vision model downloads, which already use presigned links.
- Stale Railway references.
