# The database was not split

Seagull was split into three repositories. **Postgres was not.**

`gateway-engine` (Seagull-gateway), `core-engine` and `reference-service`
(Seagull-core) each call GORM `AutoMigrate` on startup against the same
`DATABASE_URL`. Before the split, one `go test ./...` and one code review saw
every model at once. Now they do not.

## What this means in practice

- **A model change in one repo can break the other.** `AutoMigrate` adds columns
  and indexes; it does not drop or rename. Two repos adding a differently-typed
  column with the same name on the same table is the failure this document
  exists to prevent.
- **Table ownership is by repo, and it is not enforced by anything.** Nothing in
  Postgres stops core-engine from migrating a gateway table. The only protection
  is that you read this first.
- **Deploy order matters when a contract changes.** A reader deployed before the
  writer that creates its column will fail at query time, not at startup.

## Before changing any GORM model

1. Find which repo owns the table. Run `AutoMigrate` lists in both repos:
   - Seagull-gateway: `apps/gateway-engine/cmd/server/main.go`, `apps/gateway-engine/api/index.go`
   - Seagull-core: `apps/core-engine/cmd/server/main.go`, `apps/reference-service/cmd/server/main.go`
2. If the table is owned by the other repo, raise it there. Do not add the column
   from your side because `AutoMigrate` will happily let you.
3. If the change crosses both, land the additive migration first, deploy both,
   then land the code that depends on it.

## Tables owned by vision-ai-worker

These are created by raw SQL in `apps/vision-ai-worker/app/db.py`
(`SCHEMA_STATEMENTS`), not by GORM `AutoMigrate` — the worker is a Python
service and has no Go models. They still live in the same shared Postgres, so
the ownership-and-read rules above apply to them the same way. No other
repository reads any of these tables.

- **`vision_models`** — the model library: one row per uploaded artifact.
  `capability_code` is kept for backward compatibility (a row's primary
  capability); `produces` (`TEXT[]`) is authoritative once set, since one
  model may serve several capabilities.
- **`vision_capability_bindings`** — owned by vision-ai-worker. One row per
  capability code, naming the model currently serving it. A capability absent
  from this table is unbound (unsupported), not zero.
- **`vision_capability_activation_log`** — owned by vision-ai-worker.
  Append-only history of every binding change, including deactivation
  (`model_id NULL`). Never read by the scoring path; exists to answer "why did
  scores shift on the 12th."
- **`vision_registry_migrations`** — owned by vision-ai-worker. A one-row-per-
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

## If you want this to stop being a hazard

The durable fix is to take migrations away from `AutoMigrate` and give them to a
single versioned migration tool with one owner. That is a separate piece of work
and it is not done. Until it is, this document is the whole protection.
