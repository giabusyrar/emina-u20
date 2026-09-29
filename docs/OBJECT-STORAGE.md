# Object storage

Tryon shade textures and the vision model files live in one S3-compatible
bucket (Cloudflare R2). The bucket is private. Nothing in the database or in
any API response is a permanent public URL.

## What lives in the bucket

- `shades/<shadeId>/{color,alpha,finish}.png` -- tryon shade textures,
  written by ai-worker (tryon profile) after extraction.
- `models/` -- vision model files, downloaded by ai-worker (vision profile)
  through presigned links.

## Configuration

| Variable | Read by | Default | Notes |
|---|---|---|---|
| `S3_ENDPOINT` | ai-worker, core-engine | none | S3 API host, `https://<account>.r2.cloudflarestorage.com` |
| `S3_BUCKET` | ai-worker, core-engine | none | `<bucket>` |
| `S3_ACCESS_KEY_ID` | ai-worker, core-engine | none | Credential; keep in `.env`, never in the repo |
| `S3_SECRET_ACCESS_KEY` | ai-worker, core-engine | none | Credential; keep in `.env`, never in the repo |
| `S3_REGION` | ai-worker, core-engine | `auto` | R2 requires `auto` |
| `S3_PRESIGN_TTL` | core-engine | `15m` | Lifetime of a presigned texture link. Policy, so configurable. An unparseable value is a startup error. |

One `.env` serves both services. Storage is "configured" when bucket, access
key and secret are all set. core-engine boots without storage; only
shade-asset reads are affected (see below).

`S3_PUBLIC_URL` is read only by the `shade-asset-keys` migration command, to
recognise legacy rows. Neither service uses it at runtime.

## How the web app gets textures

1. Tryon uploads the maps and reports their object keys to core-engine in the
   extraction callback (`colorMapKey`, `alphaMapKey`, optional `finishMapKey`).
2. core-engine stores the keys. Stored keys are never serialized.
3. `GET /api/matching/shade-assets/:id` returns `colorMapUrl`, `alphaMapUrl`
   and optionally `finishMapUrl`, each presigned on that request for
   `S3_PRESIGN_TTL`. The web app loads the images immediately and does not
   keep the links.

Signing is local HMAC work in core-engine; it does not depend on ai-worker
being up.

Failure modes:

- Storage not configured: `503 object storage not configured`. No unsigned
  or fake URL is ever returned.
- Row has no storage keys (migration not run): `409` naming the
  `shade-asset-keys` migration.
- Presign failure: generic `500`; the detail is logged, not returned.

## Bucket CORS (required, operator step)

The web app loads textures with `crossOrigin="anonymous"`, so the bucket
needs a CORS rule or the browser refuses the images. This is configured on
the bucket, not in this repository, and nothing in the code sets it.

- Allowed origins: the web app's origins.
- Allowed methods: `GET` and `HEAD`.

On R2: bucket -> Settings -> CORS policy.

## Migrating from public URLs

Rows created before this change store public URLs. Convert them in this
order; the commands run from the repository root and read `DATABASE_URL`
and the S3 variables from `.env`.

1. `go run ./apps/core-engine/cmd/shade-asset-keys -prepare` drops the NOT
   NULL constraints on the old URL columns, so inserts from the new code do
   not fail against the old schema.
2. Deploy ai-worker and core-engine together. `AutoMigrate` adds the
   nullable key columns and new extractions store keys. Until step 4, old
   rows answer 409 rather than a broken image.
3. `go run ./apps/core-engine/cmd/shade-asset-keys` is a dry run: it prints
   each URL and the key it maps to and writes nothing. Check the output. A
   URL it does not recognise is listed and the command exits non-zero
   without writing.
4. `go run ./apps/core-engine/cmd/shade-asset-keys -apply` backfills the key
   columns and drops the old `*_url` columns in one transaction (recorded in
   `docs/SHARED-DATABASE.md`). Re-running afterwards reports the migration is
   already done and exits 0.
5. Disable the bucket's `r2.dev` public URL (operator step). On R2: bucket ->
   Settings -> Public access. Do this only after step 4.

## Why not public URLs

`*.r2.dev` is DNS-blocked in Indonesia (Internet Positif), and R2 offers no
other public read path without a custom domain. Presigned links point at the
S3 API host (`<account>.r2.cloudflarestorage.com`), which is not blocked.
