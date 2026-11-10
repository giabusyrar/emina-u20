# Reference products: one product master, owned by reference-service

Date: 2026-10-01

## Problem

The dashboard's "Products Master Reference" page is empty. It calls
`GET /api/reference/products`, which the web proxy forwards to
reference-service, and reference-service has no `/products` route and no
products table.

The product data that does exist lives somewhere else, and nothing agrees on
where:

- `MatchRepo.ListProducts` (`apps/core-engine/internal/match/repository/match_repo.go`)
  has read `ref_products`, joined to `ref_brands`, `ref_categories`,
  `ref_textures` and `ref_product_ingredients`, since the first commit. None of
  those tables except `ref_brands` exist, so `GET /match-engine/products`
  fails against a real database.
- `MatchRepo.CreateProduct` / `UpdateProduct` / `DeleteProduct` write to an
  in-memory slice only, and no route is mounted for them.
- `match_product_catalog` (core-engine `AutoMigrate`) is written by
  `cmd/seed-colour-shades` and read by the try-on colour repository
  (`apps/core-engine/internal/colour/repository/repo.go`), and by nothing in
  match.
- `ListProducts` falls back to `'Skincare'` and `'Gel'` when a product has no
  category or texture: invented values a reader cannot tell from real ones.

## Decision

`ref_products` in reference-service is the only product master. Match and
try-on read it. `match_shades.product_id` refers to `ref_products.id`.
`match_product_catalog` is retired.

Category and texture are reference entities of their own (`ref_categories`,
`ref_textures`), managed like the other reference data, not free text.

## Schema (owned by reference-service, created by its `AutoMigrate`)

| Table | Columns |
|---|---|
| `ref_categories` | `id` PK, `code` unique not null, `name` not null, `description`, `created_at`, `updated_at` |
| `ref_textures` | `id` PK, `code` unique not null, `name` not null, `description`, `created_at`, `updated_at` |
| `ref_products` | `id` PK, `code`, `brand_id` not null (index), `category_id` (index), `texture_id` nullable, `name` not null, `description`, `image_url`, `is_active` default true, `created_at`, `updated_at`, `deleted_at` (index, GORM soft delete) |
| `ref_product_ingredients` | `product_id`, `ingredient_id`, composite PK |

`id` and the reference columns are `varchar(64)`, matching the ids already in
`match_shades.product_id` and `match_product_groups.product_ids`. No database
foreign keys: none of the existing `ref_*` tables use them, and rows arrive
from three writers (API, migration, seeder) in an order this design does not
control. The API validates that `brand_id`, `category_id`, `texture_id` and
each ingredient id exist.

`category_id` is nullable in the schema so a migrated product whose old
category text is empty is not given an invented one; the API requires it on
create and update.

## reference-service API

Following the existing entity pattern in `reference_handler.go`
(`GET` list, `POST`/`PUT` save, `DELETE /:id`), responses
`{"success": true, "data": ...}`:

- `/categories`, `/textures`: plain entities, like `/ingredients`.
- `/products`: each item carries `ingredientIds` (the web form's
  multi-relation field). Saving a product replaces its ingredient rows in the
  same transaction. `DELETE` is a soft delete, because shades, product groups
  and try-on refer to product ids; a deleted product disappears from every
  list. `GET /products?brandId=` filters by brand.
- `/all` includes `categories`, `textures` and `products`.

## core-engine

- `MatchRepo.ListProducts` keeps its query against `ref_products`, plus
  `p.is_active` filtering, minus the `'Skincare'` / `'Gel'` fallbacks: an
  unknown category or texture is returned empty. `is_active` comes from the
  row, not `TRUE`.
- `UpdateProduct` / `DeleteProduct` and the three service methods are deleted.
  `CreateProduct` becomes `AddInMemoryProduct`, which the match tests use to
  load products into the in-memory repo, and which refuses to run with a
  database. Products are written through reference-service.
- `ProductCatalogItem` stays as the match read model (the scorer uses it), but
  is removed from core-engine's `AutoMigrate` list. Its `TableName` goes with
  it, so nothing can write `match_product_catalog` through it again.
- The colour repository joins `ref_products` (`p.deleted_at IS NULL AND
  p.is_active`) instead of `match_product_catalog`.
- core-engine reads `ref_*` tables directly with SQL, as `ListProducts`
  already does. It never migrates them.

## Moving the existing data

`apps/core-engine/cmd/migrate-product-catalog`, run once per database:

1. Refuses to run unless `ref_products` and `ref_categories` exist (i.e.
   reference-service has started against this database).
2. For each distinct `match_product_catalog.category`, upserts a
   `ref_categories` row with `code` = the lower-cased text and `name` = the
   text as stored. Same for non-empty `texture` into `ref_textures`.
3. Copies every `match_product_catalog` row into `ref_products` with the
   **same id**, so `match_shades` and `match_product_groups` need no change.
   An id already present in `ref_products` is left alone and reported, not
   overwritten.
4. `ingredients` (comma-separated text) is matched by name against
   `ref_ingredients`; a name with no match is reported and skipped, never
   created.
5. `-dry-run` prints what it would write and writes nothing. Everything runs
   in one transaction.

It writes with SQL, not reference-service's GORM models: those are in
another app's `internal` package.

Dropping `match_product_catalog` is a separate, recorded step in
`docs/SHARED-DATABASE.md` (like the `match_shade_assets` column drops), done
after the migration has been checked on both Neon databases. It is not part
of this change.

## Seeder

`cmd/seed-colour-shades` writes products to `ref_products` and ensures a
`ref_categories` row per CSV `kategori` (`code` = kategori). Product, shade and
colorimetry ids are computed as today, so a re-seed after the migration
updates the same rows. It refuses to run if the `ref_*` tables are missing.
`regionFor` (category → shade region) stays in the seeder.

## Deploy order

1. reference-service (creates the tables, serves the API).
2. `migrate-product-catalog` on each database (`-dry-run` first).
3. core-engine (reads `ref_products`, no longer migrates
   `match_product_catalog`).

Before step 3, core-engine's `ListProducts` fails as it does today and
try-on keeps reading `match_product_catalog`, so steps 1–2 change nothing
users see.

## Tests

- reference-service repository (sqlmock, as in `reference_repo_test.go`):
  product save writes ingredient rows in one transaction and keeps
  `created_at` on update; delete is soft; list excludes deleted rows and
  returns `ingredientIds`.
- reference-service handler: save rejects unknown brand / category /
  ingredient ids.
- migration: category derivation, id preservation, existing id not
  overwritten, unmatched ingredient reported, dry run writes nothing.
- seeder: ids unchanged from today's formula (pins existing data).
- colour and match repositories: the SQL targets `ref_products` and filters
  deleted and inactive rows.

## Out of scope

- seagull-web: category and texture fields on the product form, and pages for
  the two new entities. Raised in that repository.
- Dropping `match_product_catalog`.
- Moving `regionFor` into `ref_categories`.
