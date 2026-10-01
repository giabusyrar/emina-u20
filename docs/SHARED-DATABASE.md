# The core database

seagull-core and Seagull-gateway use separate Postgres databases. Every seagull-core table (ref_*, match_*, survey_definitions, score_rulesets, face_architecture_profiles, the model server's vision_* tables) lives in the core database only. The gateway database holds gateway-engine's tables only. `goplatform.OpenPostgres` refuses a DATABASE_URL that contains gateway tables (api_keys, collections, routes, route_groups), so a misconfigured service or seeder fails at start (packages/goplatform/dbguard.go).

Inside this repo the database is still shared: core-engine and reference-service run GORM AutoMigrate, and the model server runs raw SQL, all against the same core database. Ownership between them is by service and is not enforced.

## Before changing any GORM model

1. Find which service owns the table. Run `AutoMigrate` lists:
   - `apps/core-engine/cmd/server/main.go`, `apps/reference-service/cmd/server/main.go`
   - The model server (`apps/workers/models`) creates its tables with raw SQL.
2. If the table is owned by another service, raise it with that service's owner. Do not add the column
   from your side because `AutoMigrate` will happily let you.
3. If the change crosses services, land the additive migration first, deploy both,
   then land the code that depends on it.

## Tables owned by the model server (`apps/workers/models`)

These are created by raw SQL in `apps/workers/models/worker_models/db.py`
(`SCHEMA_STATEMENTS`), not by GORM `AutoMigrate` — the model server is a Python
service and has no Go models. They live in the core database, so
the ownership-and-read rules above apply to them the same way. No other
repository reads any of these tables.

- **`vision_models`** — the model library: one row per uploaded artifact.
  `capability_code` is kept for backward compatibility (a row's primary
  capability); `produces` (`TEXT[]`) is authoritative once set, since one
  model may serve several capabilities. `sha256` (`TEXT`, nullable) is the
  hash of the stored file, added by the additive
  `ALTER TABLE vision_models ADD COLUMN IF NOT EXISTS sha256 TEXT`; rows from
  before it get the value on first fetch (the disk cache is addressed by it,
  `docs/MODEL-REGISTRY.md`).
- **`vision_capability_bindings`** — owned by the model server. One row per
  capability code, naming the model currently serving it. A capability absent
  from this table is unbound (unsupported), not zero.
- **`vision_capability_activation_log`** — owned by the model server.
  Append-only history of every binding change, including deactivation
  (`model_id NULL`). Never read by the scoring path; exists to answer "why did
  scores shift on the 12th."
- **`vision_registry_migrations`** — owned by the model server. A one-row-per-
  migration marker table (currently just `bindings_backfill_v1`) that gates
  the one-time backfill of `produces`/bindings from legacy rows, so `init_pool`
  re-running its statement list on every process start cannot re-run that
  backfill and silently re-bind a capability an operator deliberately
  unbound.

### The dropped uniqueness constraint

`vision_models.capability_code` was originally `UNIQUE`. That constraint is
dropped (`ALTER TABLE vision_models DROP CONSTRAINT IF EXISTS
vision_models_capability_code_key`) as part of the same schema-migration
statement list, immediately after the `produces`/bindings backfill runs. The
library model requires more than one row to be able to share a
`capability_code` — an upload adds a candidate model, it does not replace one
— so the constraint had to go. The drop is safe at that point in the
statement list specifically because the backfill immediately before it (also
run-once, gated by `vision_registry_migrations`) has already given every
pre-existing capability a binding row, so there is no window in which a
second upload could shadow a live model before bindings exist to prevent it.

#### Dropping it makes the model server deploy one-way — plan the rollback as roll-forward

**Once this migration has run, rolling the model server back to the previous
release breaks every model upload.** The previous release's `upsert_model`
inserted with `INSERT ... ON CONFLICT (capability_code)`, and that clause
needs a unique or exclusion constraint on `capability_code` to exist. With
`vision_models_capability_code_key` dropped, it fails with:

```
there is no unique or exclusion constraint matching the ON CONFLICT specification
```

Notes for whoever is holding the deploy:

- The breakage is immediate on rollback, not deferred until a duplicate
  upload; the old statement needs the constraint at plan time.
- Only uploads break. Reads, dispatch and scoring keep working, so the
  failure will show up as "the admin upload form 500s", not as an outage.
- **Roll forward to fix it.** Do not recreate the constraint by hand: once
  two rows share a `capability_code` — the normal state of a library after
  its second upload — it cannot be recreated at all, and recreating it on a
  library that has no duplicates *yet* only moves the breakage to the next
  upload.
- The schema change is shared-database state, so it outlives the server
  version: redeploying the old image does not undo it.

The same warning, from the server's side, is at the top of
`docs/MODEL-REGISTRY.md`.

### Advisory lock on the schema run

`apply_schema` in `apps/workers/models/worker_models/db.py` holds
`pg_advisory_lock(SCHEMA_ADVISORY_LOCK_ID)` — a fixed constant, currently
`1581646198` — for the whole of `SCHEMA_STATEMENTS`, because
`CREATE TABLE IF NOT EXISTS` and `ADD COLUMN IF NOT EXISTS` are not race-free
and two replicas boot together on every rolling deploy. **If another core service ever takes advisory locks on this database, keep its ids distinct from this one.** A collision is not corrupting — it would only serialize two
unrelated migrations against each other — but it is a needless startup stall,
and a deadlock risk if both sides ever take two locks.

### Advisory locks in use

- model server schema run: single-key `pg_advisory_lock(1581646198)`.
- core-engine face-architecture upsert: two-key class `1581646199`
  (`pg_advisory_xact_lock(1581646199, hashtext(brand|app))`), whose key space does not
  overlap single-key locks.

## The product master

`ref_products`, `ref_categories`, `ref_textures` and `ref_product_ingredients`
are owned by reference-service (its `AutoMigrate`). core-engine reads them with
SQL (match `ListProducts`, the try-on colour repository) and writes them only
from two offline commands, through `apps/core-engine/internal/refcatalog`:
`cmd/migrate-product-catalog` and `cmd/seed-colour-shades`. core-engine never
migrates them; both commands refuse to run until the tables exist.

`match_shades.product_id` and `match_product_groups.product_ids` hold
`ref_products` ids. There are no foreign keys; reference-service validates
references on save, and product deletes are soft (`deleted_at`).

`match_product_catalog` is gone: its rows were copied into `ref_products`
with the same ids by `migrate-product-catalog`, and the table was dropped
(see "Table drops outside AutoMigrate" below). `migrate-product-catalog`
now has nothing to read and stops with "match_product_catalog not found".

## If you want this to stop being a hazard

The durable fix is to take migrations away from `AutoMigrate` and give them to a
single versioned migration tool with one owner. That is a separate piece of work
and it is not done. Until it is, this document is the whole protection.

## Column drops outside AutoMigrate

`AutoMigrate` never drops columns, so a drop is a deliberate, recorded step.

- **`match_shade_assets.color_map_url`, `alpha_map_url`, `finish_map_url`**
  (core-engine). Replaced by `*_map_key`. Dropped by
  `apps/core-engine/cmd/shade-asset-keys -apply` after converting each URL
  to its object key; see `docs/OBJECT-STORAGE.md`. No other repository
  reads this table.

## Table drops outside AutoMigrate

- **`match_product_catalog`** (core-engine), dropped from the core database
  (`ep-hidden-unit`) on 2026-10-01, after checking that all 31 of its ids were
  in `ref_products` and that no `match_shades` row lacked a product. A JSON
  backup of the 31 rows was taken first
  (`Paragon/db-backups/match_product_catalog-ep-hidden-unit-20261001.json`,
  outside this repository). Its copy in the gateway database had already been
  dropped with the other core tables.
