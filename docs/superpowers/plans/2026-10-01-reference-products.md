# Reference Products Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ref_products` (reference-service) the single product master that the dashboard, match and try-on all read, and move the 31 seeded products into it with their ids unchanged.

**Architecture:** reference-service gains four GORM models (`ref_categories`, `ref_textures`, `ref_products`, `ref_product_ingredients`) and CRUD routes. core-engine stops migrating `match_product_catalog`; match and colour read `ref_products` with SQL. A small core-engine package, `internal/refcatalog`, writes those tables for the two offline commands (the new `migrate-product-catalog` and the existing `seed-colour-shades`) with plain SQL, because the GORM models live in another app's `internal` package.

**Tech Stack:** Go, Echo, GORM (postgres driver), go-sqlmock, Postgres (Neon).

**Spec:** `docs/superpowers/specs/2026-10-01-reference-products-design.md`

## Global Constraints

- Only reference-service `AutoMigrate`s the four `ref_*` tables. core-engine never creates or alters them (`docs/SHARED-DATABASE.md`).
- Product, shade and colorimetry ids from `seed-colour-shades` must not change: `id("clr-prd-", brandID, produk)` etc. Pinned: brand `brd-1790654876204254300` + `Make Over Blush On Single` → `clr-prd-e41ea982f8fcb07e`.
- No invented values: no `'Skincare'` / `'Gel'` fallbacks; an unknown category or texture is empty. Migration never creates an ingredient or a category it was not given.
- Ids are `varchar(64)`. No database foreign keys.
- `DELETE /products/:id` is a soft delete (`deleted_at`).
- Never run a test that does DDL or `DELETE` against a shared database. Scratch-database tests read their URL from an env var and skip without it (pattern: `internal/facearch/profile/store_test.go`). Neon ignores `search_path`, so a scratch *schema* is not isolation; it must be a separate database.
- Do not `git push`. Commit per task.
- Lanes (`docs/LANES.md`): claim before Task 1 — `reference`, `core-match`, `core-colour`, `apps/core-engine/internal/refcatalog/`, `apps/core-engine/cmd/migrate-product-catalog/`, `apps/core-engine/cmd/server/main.go`, `docs/SHARED-DATABASE.md`, `docs/COLOUR-LOCAL-RUN.md`. Release at the end.
- Known, accepted: core-engine caches `ListProducts` in Redis (`match:products:*`) for its configured TTL, so a product edited in reference-service shows up in match after that TTL. Not fixed here.

## Deviation from the spec

The spec says `MatchRepo.CreateProduct` is deleted. The match service tests use it to load products into the in-memory repo (`NewMatchRepo(nil)`), so it is **renamed** to `AddInMemoryProduct` and refuses to run with a database. `UpdateProduct`, `DeleteProduct` and the three service delegations are deleted as specified. Task 5 updates the spec line.

## File map

| File | Responsibility |
|---|---|
| `apps/reference-service/internal/reference/entity/product.go` (new) | `RefCategory`, `RefTexture`, `RefProduct`, `RefProductIngredient` |
| `apps/reference-service/internal/reference/repository/taxonomy_repo.go` (new) | categories + textures CRUD |
| `apps/reference-service/internal/reference/repository/product_repo.go` (new) | products CRUD, reference validation, ingredient rows |
| `apps/reference-service/internal/reference/repository/product_repo_test.go` (new) | sqlmock tests |
| `apps/reference-service/internal/reference/controllers/product_handler.go` (new) | handlers for the three entities |
| `apps/reference-service/internal/reference/controllers/product_handler_test.go` (new) | 400 on invalid reference |
| `apps/reference-service/internal/reference/controllers/reference_handler.go` | routes, `/all` |
| `apps/reference-service/cmd/server/main.go` | `AutoMigrate` list |
| `apps/core-engine/internal/match/repository/match_repo.go` | `ListProducts` query; remove product writes |
| `apps/core-engine/internal/match/service/match_service.go` | remove product write delegations |
| `apps/core-engine/internal/match/entity/entity.go` | drop `ProductCatalogItem.TableName` |
| `apps/core-engine/cmd/server/main.go` | drop `ProductCatalogItem` from `AutoMigrate` |
| `apps/core-engine/internal/colour/repository/repo.go` | join `ref_products` |
| `apps/core-engine/internal/refcatalog/refcatalog.go` (new) | SQL writes to the ref product tables |
| `apps/core-engine/cmd/migrate-product-catalog/main.go` (new) | one-off copy from `match_product_catalog` |
| `apps/core-engine/cmd/seed-colour-shades/main.go` | write products via `refcatalog` |
| `docs/SHARED-DATABASE.md`, `docs/COLOUR-LOCAL-RUN.md` | ownership, run order |

---

### Task 1: reference-service product models

**Files:**
- Create: `apps/reference-service/internal/reference/entity/product.go`
- Create: `apps/reference-service/internal/reference/entity/product_test.go`
- Modify: `apps/reference-service/cmd/server/main.go:50-60`

**Interfaces:**
- Produces: `entity.RefCategory`, `entity.RefTexture`, `entity.RefProduct` (field `IngredientIDs []string`, `gorm:"-"`; `IsActive *bool`; `CategoryID, TextureID *string`; `DeletedAt gorm.DeletedAt`), `entity.RefProductIngredient{ProductID, IngredientID string}`.

- [ ] **Step 1: Write the failing test**

```go
package entity

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func parse(t *testing.T, model any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProductTables(t *testing.T) {
	for model, want := range map[any]string{
		&RefCategory{}: "ref_categories", &RefTexture{}: "ref_textures",
		&RefProduct{}: "ref_products", &RefProductIngredient{}: "ref_product_ingredients",
	} {
		if got := parse(t, model).Table; got != want {
			t.Errorf("%T table = %q, want %q", model, got, want)
		}
	}
}

// Shades, product groups and try-on refer to product ids, so a delete must
// be soft, and ingredient ids are not a column.
func TestRefProductSchema(t *testing.T) {
	s := parse(t, &RefProduct{})
	if f := s.LookUpField("deleted_at"); f == nil {
		t.Fatal("ref_products needs deleted_at for soft delete")
	}
	if f := s.LookUpField("IngredientIDs"); f != nil && f.DBName != "" {
		t.Errorf("IngredientIDs must not be a column, got %q", f.DBName)
	}
	for _, col := range []string{"id", "code", "brand_id", "category_id", "texture_id", "name", "description", "image_url", "is_active"} {
		if s.LookUpField(col) == nil {
			t.Errorf("missing column %s", col)
		}
	}
	pk := parse(t, &RefProductIngredient{}).PrimaryFieldDBNames
	if len(pk) != 2 {
		t.Errorf("ref_product_ingredients primary key = %v, want product_id + ingredient_id", pk)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/reference-service && go test ./internal/reference/entity/ -run 'TestProductTables|TestRefProductSchema' -v`
Expected: FAIL, `undefined: RefCategory`.

- [ ] **Step 3: Write the models**

```go
package entity

import (
	"time"

	"gorm.io/gorm"
)

// RefCategory is a product category: makeup sections (lip, blush,
// complexion, ...) and skincare kinds alike.
type RefCategory struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	Code        string    `gorm:"column:code;type:varchar(64);uniqueIndex;not null" json:"code"`
	Name        string    `gorm:"column:name;not null" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (RefCategory) TableName() string { return "ref_categories" }

// RefTexture is a product texture (cream, gel, fluid, ...).
type RefTexture struct {
	ID          string    `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	Code        string    `gorm:"column:code;type:varchar(64);uniqueIndex;not null" json:"code"`
	Name        string    `gorm:"column:name;not null" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (RefTexture) TableName() string { return "ref_textures" }

// RefProduct is the product master. Match (core-engine) and try-on read it;
// match_shades.product_id refers to its id. CategoryID is nullable so a
// migrated product without a category is not given an invented one; the API
// requires it. IsActive is a pointer so a save that leaves it out keeps the
// stored value instead of writing false.
type RefProduct struct {
	ID            string         `gorm:"primaryKey;column:id;type:varchar(64)" json:"id"`
	Code          string         `gorm:"column:code;type:varchar(64)" json:"code"`
	BrandID       string         `gorm:"column:brand_id;type:varchar(64);not null;index" json:"brandId"`
	CategoryID    *string        `gorm:"column:category_id;type:varchar(64);index" json:"categoryId"`
	TextureID     *string        `gorm:"column:texture_id;type:varchar(64)" json:"textureId"`
	Name          string         `gorm:"column:name;not null" json:"name"`
	Description   string         `gorm:"column:description" json:"description"`
	ImageURL      string         `gorm:"column:image_url;type:varchar(512)" json:"imageUrl"`
	IsActive      *bool          `gorm:"column:is_active;not null;default:true" json:"isActive"`
	IngredientIDs []string       `gorm:"-" json:"ingredientIds"`
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func (RefProduct) TableName() string { return "ref_products" }

// RefProductIngredient links a product to a ref_ingredients row.
type RefProductIngredient struct {
	ProductID    string `gorm:"primaryKey;column:product_id;type:varchar(64)" json:"productId"`
	IngredientID string `gorm:"primaryKey;column:ingredient_id;type:varchar(64)" json:"ingredientId"`
}

func (RefProductIngredient) TableName() string { return "ref_product_ingredients" }
```

Then in `apps/reference-service/cmd/server/main.go`, add to the `AutoMigrate(...)` list after `&entity.RefSkinCondition{},`:

```go
		&entity.RefCategory{},
		&entity.RefTexture{},
		&entity.RefProduct{},
		&entity.RefProductIngredient{},
```

- [ ] **Step 4: Run tests and build**

Run: `cd apps/reference-service && go test ./internal/reference/entity/ -v && go build ./...`
Expected: PASS, build succeeds.

- [ ] **Step 5: Commit**

```bash
git add apps/reference-service/internal/reference/entity/product.go apps/reference-service/internal/reference/entity/product_test.go apps/reference-service/cmd/server/main.go
git commit -m "feat(reference): product, category and texture models"
```

---

### Task 2: categories and textures repository

**Files:**
- Create: `apps/reference-service/internal/reference/repository/taxonomy_repo.go`
- Create: `apps/reference-service/internal/reference/repository/taxonomy_repo_test.go`

**Interfaces:**
- Consumes: Task 1 models; `keepCreatedAt` and `newMockRepo` (existing, `reference_repo.go` / `reference_repo_test.go`).
- Produces: `(*ReferenceRepo).ListCategories(ctx) ([]entity.RefCategory, error)`, `SaveCategory(ctx, *entity.RefCategory) error`, `DeleteCategory(ctx, idOrCode string) error`, and the same three for `Texture`. Cache keys `ref:categories`, `ref:textures`.

- [ ] **Step 1: Write the failing test**

```go
package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
)

// A save with a code but no id reuses the row that has that code, so the
// same category saved twice is one row.
func TestSaveCategoryReusesIDByCode(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ref_categories" WHERE code = $1`)).
		WithArgs("lip", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name"}).AddRow("cat-lip", "lip", "Lip"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "created_at" FROM "ref_categories"`)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ref_categories"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	item := &entity.RefCategory{Code: "lip", Name: "Lip"}
	if err := repo.SaveCategory(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if item.ID != "cat-lip" {
		t.Errorf("ID = %q, want the existing cat-lip", item.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSaveTextureNewCodeGetsID(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ref_textures" WHERE code = $1`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "created_at" FROM "ref_textures"`)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ref_textures"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "ref_textures"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	item := &entity.RefTexture{Code: "cream", Name: "Cream"}
	if err := repo.SaveTexture(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if item.ID == "" {
		t.Error("a new texture must get an id")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/reference-service && go test ./internal/reference/repository/ -run 'TestSaveCategory|TestSaveTexture' -v`
Expected: FAIL, `repo.SaveCategory undefined`.

- [ ] **Step 3: Implement**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
)

// Categories CRUD
func (r *ReferenceRepo) ListCategories(ctx context.Context) ([]entity.RefCategory, error) {
	const cacheKey = "ref:categories"
	var items []entity.RefCategory
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &items) && len(items) > 0 {
		return items, nil
	}
	if r.db == nil {
		return items, nil
	}
	err := r.db.WithContext(ctx).Order("name asc").Find(&items).Error
	if err == nil && r.cache != nil && len(items) > 0 {
		r.cache.Set(ctx, cacheKey, items)
	}
	return items, err
}

func (r *ReferenceRepo) SaveCategory(ctx context.Context, item *entity.RefCategory) error {
	if r.db == nil {
		return nil
	}
	if item.ID == "" && item.Code != "" {
		var existing entity.RefCategory
		if err := r.db.WithContext(ctx).Where("code = ?", item.Code).First(&existing).Error; err == nil {
			item.ID = existing.ID
		} else {
			item.ID = fmt.Sprintf("cat-%d", time.Now().UnixNano())
		}
	}
	if item.ID == "" {
		item.ID = fmt.Sprintf("cat-%d", time.Now().UnixNano())
	}
	if err := keepCreatedAt(r.db.WithContext(ctx), &entity.RefCategory{}, item.ID, &item.CreatedAt); err != nil {
		return err
	}
	err := r.db.WithContext(ctx).Save(item).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, "ref:categories")
	}
	return err
}

func (r *ReferenceRepo) DeleteCategory(ctx context.Context, id string) error {
	if r.db == nil {
		return nil
	}
	err := r.db.WithContext(ctx).Where("id = ? OR code = ?", id, id).Delete(&entity.RefCategory{}).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, "ref:categories")
	}
	return err
}

// Textures CRUD
func (r *ReferenceRepo) ListTextures(ctx context.Context) ([]entity.RefTexture, error) {
	const cacheKey = "ref:textures"
	var items []entity.RefTexture
	if r.cache != nil && r.cache.Get(ctx, cacheKey, &items) && len(items) > 0 {
		return items, nil
	}
	if r.db == nil {
		return items, nil
	}
	err := r.db.WithContext(ctx).Order("name asc").Find(&items).Error
	if err == nil && r.cache != nil && len(items) > 0 {
		r.cache.Set(ctx, cacheKey, items)
	}
	return items, err
}

func (r *ReferenceRepo) SaveTexture(ctx context.Context, item *entity.RefTexture) error {
	if r.db == nil {
		return nil
	}
	if item.ID == "" && item.Code != "" {
		var existing entity.RefTexture
		if err := r.db.WithContext(ctx).Where("code = ?", item.Code).First(&existing).Error; err == nil {
			item.ID = existing.ID
		} else {
			item.ID = fmt.Sprintf("tex-%d", time.Now().UnixNano())
		}
	}
	if item.ID == "" {
		item.ID = fmt.Sprintf("tex-%d", time.Now().UnixNano())
	}
	if err := keepCreatedAt(r.db.WithContext(ctx), &entity.RefTexture{}, item.ID, &item.CreatedAt); err != nil {
		return err
	}
	err := r.db.WithContext(ctx).Save(item).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, "ref:textures")
	}
	return err
}

func (r *ReferenceRepo) DeleteTexture(ctx context.Context, id string) error {
	if r.db == nil {
		return nil
	}
	err := r.db.WithContext(ctx).Where("id = ? OR code = ?", id, id).Delete(&entity.RefTexture{}).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, "ref:textures")
	}
	return err
}
```

`First` orders by primary key and adds `LIMIT $2`; if sqlmock reports the query text differently, run with `-v`, read the actual SQL in the failure, and match its prefix. Do not loosen the assertions on which table is hit.

- [ ] **Step 4: Run tests**

Run: `cd apps/reference-service && go test ./internal/reference/repository/ -v`
Expected: PASS (new and existing tests).

- [ ] **Step 5: Commit**

```bash
git add apps/reference-service/internal/reference/repository/taxonomy_repo.go apps/reference-service/internal/reference/repository/taxonomy_repo_test.go
git commit -m "feat(reference): categories and textures repository"
```

---

### Task 3: products repository

**Files:**
- Create: `apps/reference-service/internal/reference/repository/product_repo.go`
- Create: `apps/reference-service/internal/reference/repository/product_repo_test.go`

**Interfaces:**
- Consumes: Task 1 models, `newMockRepo`.
- Produces:
  - `var ErrInvalidReference = errors.New("invalid reference")` (wrapped with detail; handlers test with `errors.Is`).
  - `(*ReferenceRepo).ListProducts(ctx, brandID string) ([]entity.RefProduct, error)` — `brandID == ""` means all; each item has `IngredientIDs` set (never nil: `[]string{}`).
  - `(*ReferenceRepo).SaveProduct(ctx, *entity.RefProduct) error`
  - `(*ReferenceRepo).DeleteProduct(ctx, idOrCode string) error` (soft)
  - Cache key `ref:products` (full list; brand filter applied in Go).

- [ ] **Step 1: Write the failing tests**

```go
package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
)

func strp(s string) *string { return &s }

func expectFound(mock sqlmock.Sqlmock, table string, ids ...string) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM ` + table + ` WHERE id IN`)).WillReturnRows(rows)
}

func TestSaveProductRequiresCategory(t *testing.T) {
	repo, _ := newMockRepo(t)
	err := repo.SaveProduct(context.Background(), &entity.RefProduct{Name: "X", BrandID: "brd-1"})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("err = %v, want ErrInvalidReference", err)
	}
}

func TestSaveProductRejectsUnknownIngredient(t *testing.T) {
	repo, mock := newMockRepo(t)
	expectFound(mock, "ref_brands", "brd-1")
	expectFound(mock, "ref_categories", "cat-lip")
	expectFound(mock, "ref_ingredients", "ing-1") // ing-2 missing

	err := repo.SaveProduct(context.Background(), &entity.RefProduct{
		Name: "X", BrandID: "brd-1", CategoryID: strp("cat-lip"), IngredientIDs: []string{"ing-1", "ing-2"},
	})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("err = %v, want ErrInvalidReference", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// An update keeps created_at and is_active when the body leaves them out, and
// replaces the ingredient rows in the same transaction.
func TestSaveProductUpdateReplacesIngredients(t *testing.T) {
	repo, mock := newMockRepo(t)
	created := time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC)
	expectFound(mock, "ref_brands", "brd-1")
	expectFound(mock, "ref_categories", "cat-lip")
	expectFound(mock, "ref_ingredients", "ing-1")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT created_at, is_active FROM ref_products WHERE id = $1`)).
		WithArgs("prd-1").
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "is_active"}).AddRow(created, false))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ref_products"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "ref_product_ingredients" WHERE product_id = $1`)).
		WithArgs("prd-1").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "ref_product_ingredients"`)).
		WithArgs("prd-1", "ing-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	item := &entity.RefProduct{ID: "prd-1", Name: "X", BrandID: "brd-1", CategoryID: strp("cat-lip"), IngredientIDs: []string{"ing-1"}}
	if err := repo.SaveProduct(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if !item.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want stored %v", item.CreatedAt, created)
	}
	if item.IsActive == nil || *item.IsActive {
		t.Errorf("IsActive = %v, want the stored false", item.IsActive)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListProductsAttachesIngredientsAndFiltersBrand(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ref_products" WHERE "ref_products"."deleted_at" IS NULL ORDER BY name asc`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "brand_id", "name"}).
			AddRow("prd-1", "brd-1", "A").AddRow("prd-2", "brd-2", "B"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "ref_product_ingredients" WHERE product_id IN ($1,$2)`)).
		WillReturnRows(sqlmock.NewRows([]string{"product_id", "ingredient_id"}).AddRow("prd-1", "ing-1"))

	got, err := repo.ListProducts(context.Background(), "brd-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "prd-1" {
		t.Fatalf("got %+v, want only prd-1", got)
	}
	if len(got[0].IngredientIDs) != 1 || got[0].IngredientIDs[0] != "ing-1" {
		t.Errorf("IngredientIDs = %v, want [ing-1]", got[0].IngredientIDs)
	}
}

func TestDeleteProductIsSoft(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ref_products" SET "deleted_at"=`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.DeleteProduct(context.Background(), "prd-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/reference-service && go test ./internal/reference/repository/ -run 'Product' -v`
Expected: FAIL, `repo.SaveProduct undefined`.

- [ ] **Step 3: Implement**

```go
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
)

// ErrInvalidReference means a product names a brand, category, texture or
// ingredient that does not exist. There are no foreign keys on these tables
// (docs/superpowers/specs/2026-10-01-reference-products-design.md), so this
// check is the only one.
var ErrInvalidReference = errors.New("invalid reference")

const productsCacheKey = "ref:products"

// missingIDs returns the ids from want that have no row in table.
func missingIDs(tx *gorm.DB, table string, want []string) ([]string, error) {
	if len(want) == 0 {
		return nil, nil
	}
	var found []string
	if err := tx.Raw("SELECT id FROM "+table+" WHERE id IN ?", want).Scan(&found).Error; err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, id := range found {
		have[id] = true
	}
	var missing []string
	for _, id := range want {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func (r *ReferenceRepo) validateProduct(tx *gorm.DB, p *entity.RefProduct) error {
	if p.BrandID == "" {
		return fmt.Errorf("%w: brandId is required", ErrInvalidReference)
	}
	if p.CategoryID == nil || *p.CategoryID == "" {
		return fmt.Errorf("%w: categoryId is required", ErrInvalidReference)
	}
	checks := []struct {
		table, field string
		ids          []string
	}{
		{"ref_brands", "brandId", []string{p.BrandID}},
		{"ref_categories", "categoryId", []string{*p.CategoryID}},
	}
	if p.TextureID != nil && *p.TextureID != "" {
		checks = append(checks, struct {
			table, field string
			ids          []string
		}{"ref_textures", "textureId", []string{*p.TextureID}})
	}
	checks = append(checks, struct {
		table, field string
		ids          []string
	}{"ref_ingredients", "ingredientIds", p.IngredientIDs})
	for _, c := range checks {
		missing, err := missingIDs(tx, c.table, c.ids)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return fmt.Errorf("%w: %s %v not found", ErrInvalidReference, c.field, missing)
		}
	}
	return nil
}

func (r *ReferenceRepo) ListProducts(ctx context.Context, brandID string) ([]entity.RefProduct, error) {
	all := []entity.RefProduct{} // never nil: an empty list is [] in JSON
	if !(r.cache != nil && r.cache.Get(ctx, productsCacheKey, &all) && len(all) > 0) {
		if r.db == nil {
			return []entity.RefProduct{}, nil
		}
		if err := r.db.WithContext(ctx).Order("name asc").Find(&all).Error; err != nil {
			return nil, err
		}
		if len(all) > 0 {
			ids := make([]string, len(all))
			for i, p := range all {
				ids[i] = p.ID
			}
			var links []entity.RefProductIngredient
			if err := r.db.WithContext(ctx).Where("product_id IN ?", ids).Find(&links).Error; err != nil {
				return nil, err
			}
			byProduct := map[string][]string{}
			for _, l := range links {
				byProduct[l.ProductID] = append(byProduct[l.ProductID], l.IngredientID)
			}
			for i := range all {
				all[i].IngredientIDs = byProduct[all[i].ID]
				if all[i].IngredientIDs == nil {
					all[i].IngredientIDs = []string{}
				}
				sort.Strings(all[i].IngredientIDs)
			}
		}
		if r.cache != nil && len(all) > 0 {
			r.cache.Set(ctx, productsCacheKey, all)
		}
	}
	if brandID == "" {
		return all, nil
	}
	out := []entity.RefProduct{}
	for _, p := range all {
		if p.BrandID == brandID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *ReferenceRepo) SaveProduct(ctx context.Context, item *entity.RefProduct) error {
	if item.BrandID == "" || item.CategoryID == nil || *item.CategoryID == "" {
		return r.validateProduct(nil, item) // fails before touching the db
	}
	if r.db == nil {
		return nil
	}
	db := r.db.WithContext(ctx)
	if err := r.validateProduct(db, item); err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = fmt.Sprintf("prd-%d", time.Now().UnixNano())
	}
	// created_at and is_active are kept from the stored row when the body
	// leaves them out (keepCreatedAt explains why Save needs this).
	var stored []struct {
		CreatedAt time.Time
		IsActive  bool
	}
	if err := db.Raw("SELECT created_at, is_active FROM ref_products WHERE id = ?", item.ID).Scan(&stored).Error; err != nil {
		return err
	}
	if len(stored) > 0 {
		item.CreatedAt = stored[0].CreatedAt
		if item.IsActive == nil {
			v := stored[0].IsActive
			item.IsActive = &v
		}
	} else if item.IsActive == nil {
		v := true
		item.IsActive = &v
	}
	ingredients := item.IngredientIDs
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(item).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", item.ID).Delete(&entity.RefProductIngredient{}).Error; err != nil {
			return err
		}
		if len(ingredients) == 0 {
			return nil
		}
		links := make([]entity.RefProductIngredient, len(ingredients))
		for i, id := range ingredients {
			links[i] = entity.RefProductIngredient{ProductID: item.ID, IngredientID: id}
		}
		return tx.Create(&links).Error
	})
	if item.IngredientIDs == nil {
		item.IngredientIDs = []string{}
	}
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, productsCacheKey)
	}
	return err
}

// DeleteProduct soft-deletes: shades, product groups and try-on refer to
// product ids, and a deleted product drops out of every list.
func (r *ReferenceRepo) DeleteProduct(ctx context.Context, id string) error {
	if r.db == nil {
		return nil
	}
	err := r.db.WithContext(ctx).Where("id = ? OR code = ?", id, id).Delete(&entity.RefProduct{}).Error
	if err == nil && r.cache != nil {
		r.cache.Invalidate(ctx, productsCacheKey)
	}
	return err
}
```

`validateProduct(nil, …)` only reaches the db when brand and category are both set, so the nil call is safe; keep that ordering if you refactor. The `struct{table, field string; ids []string}` repetition can be a named local type `refCheck` — do that, it reads better.

- [ ] **Step 4: Run tests**

Run: `cd apps/reference-service && go test ./internal/reference/repository/ -v`
Expected: PASS. If GORM's SQL differs from an expectation (e.g. the `IN` placeholder list or the soft-delete `UPDATE`), read the actual SQL from the `-v` failure and match it; keep the table and the `WithArgs` checks.

- [ ] **Step 5: Commit**

```bash
git add apps/reference-service/internal/reference/repository/product_repo.go apps/reference-service/internal/reference/repository/product_repo_test.go
git commit -m "feat(reference): products repository with ingredient links and soft delete"
```

---

### Task 4: routes and handlers

**Files:**
- Create: `apps/reference-service/internal/reference/controllers/product_handler.go`
- Create: `apps/reference-service/internal/reference/controllers/product_handler_test.go`
- Modify: `apps/reference-service/internal/reference/controllers/reference_handler.go:21-72` (routes), `:296-323` (`GetAllReferences`)
- Modify: `apps/reference-service/go.mod` (go-sqlmock becomes a direct dependency: `go mod tidy`)

**Interfaces:**
- Consumes: Tasks 2–3 repository methods, `repository.ErrInvalidReference`.
- Produces: `GET/POST/PUT /categories`, `DELETE /categories/:id`; same for `/textures`; `GET /products?brandId=`, `POST/PUT /products`, `DELETE /products/:id`. Responses `{"success": true, "data": ...}`; invalid reference → 400 via `goplatform.JSONError`.

- [ ] **Step 1: Write the failing test**

```go
package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/experience-gateway/apps/reference-service/internal/reference/repository"
)

func newHandler(t *testing.T) (*echo.Echo, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	NewReferenceHandler(repository.NewReferenceRepo(db)).RegisterRoutes(e.Group("/api/reference"))
	return e, mock
}

func TestSaveProductWithoutCategoryIs400(t *testing.T) {
	e, _ := newHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/reference/products",
		strings.NewReader(`{"name":"X","brandId":"brd-1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
	}
}

func TestListProductsRouteExists(t *testing.T) {
	e, mock := newHandler(t)
	mock.ExpectQuery(`SELECT \* FROM "ref_products"`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/reference/products", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("an empty list must be [], got %s", rec.Body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/reference-service && go test ./internal/reference/controllers/ -v`
Expected: FAIL, 404 from the router.

- [ ] **Step 3: Implement**

`product_handler.go`:

```go
package controllers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/experience-gateway/apps/reference-service/internal/reference/entity"
	"github.com/experience-gateway/apps/reference-service/internal/reference/repository"
	"github.com/experience-gateway/packages/goplatform"
)

// saveError maps an invalid reference to 400 and anything else to 500.
func saveError(c echo.Context, err error) error {
	if errors.Is(err, repository.ErrInvalidReference) {
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}
	return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
}

// Categories Handlers
func (h *ReferenceHandler) ListCategories(c echo.Context) error {
	items, err := h.repo.ListCategories(c.Request().Context())
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items, "success": true})
}

func (h *ReferenceHandler) SaveCategory(c echo.Context) error {
	var item entity.RefCategory
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.repo.SaveCategory(c.Request().Context(), &item); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": item, "success": true})
}

func (h *ReferenceHandler) DeleteCategory(c echo.Context) error {
	if err := h.repo.DeleteCategory(c.Request().Context(), c.Param("id")); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Textures Handlers
func (h *ReferenceHandler) ListTextures(c echo.Context) error {
	items, err := h.repo.ListTextures(c.Request().Context())
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items, "success": true})
}

func (h *ReferenceHandler) SaveTexture(c echo.Context) error {
	var item entity.RefTexture
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.repo.SaveTexture(c.Request().Context(), &item); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": item, "success": true})
}

func (h *ReferenceHandler) DeleteTexture(c echo.Context) error {
	if err := h.repo.DeleteTexture(c.Request().Context(), c.Param("id")); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}

// Products Handlers
func (h *ReferenceHandler) ListProducts(c echo.Context) error {
	items, err := h.repo.ListProducts(c.Request().Context(), c.QueryParam("brandId"))
	if err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": items, "success": true})
}

func (h *ReferenceHandler) SaveProduct(c echo.Context) error {
	var item entity.RefProduct
	if err := c.Bind(&item); err != nil {
		return goplatform.JSONError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.repo.SaveProduct(c.Request().Context(), &item); err != nil {
		return saveError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": item, "success": true})
}

func (h *ReferenceHandler) DeleteProduct(c echo.Context) error {
	if err := h.repo.DeleteProduct(c.Request().Context(), c.Param("id")); err != nil {
		return goplatform.JSONError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"success": true})
}
```

In `RegisterRoutes`, before `// All in one batch endpoint`:

```go
	// Categories
	g.GET("/categories", h.ListCategories)
	g.POST("/categories", h.SaveCategory)
	g.PUT("/categories", h.SaveCategory)
	g.DELETE("/categories/:id", h.DeleteCategory)

	// Textures
	g.GET("/textures", h.ListTextures)
	g.POST("/textures", h.SaveTexture)
	g.PUT("/textures", h.SaveTexture)
	g.DELETE("/textures/:id", h.DeleteTexture)

	// Products
	g.GET("/products", h.ListProducts)
	g.POST("/products", h.SaveProduct)
	g.PUT("/products", h.SaveProduct)
	g.DELETE("/products/:id", h.DeleteProduct)
```

In `GetAllReferences`, add after `skinConditions, _ := ...`:

```go
	categories, _ := h.repo.ListCategories(ctx)
	textures, _ := h.repo.ListTextures(ctx)
	products, _ := h.repo.ListProducts(ctx, "")
```

and to the `data` map:

```go
			"categories":         categories,
			"textures":           textures,
			"products":           products,
```

Run `cd apps/reference-service && go mod tidy`.

- [ ] **Step 4: Run tests and build**

Run: `cd apps/reference-service && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/reference-service/internal/reference/controllers/ apps/reference-service/internal/reference/repository/product_repo.go apps/reference-service/go.mod apps/reference-service/go.sum
git commit -m "feat(reference): categories, textures and products routes"
```

---

### Task 5: match reads `ref_products`; core-engine stops owning the catalog table

**Files:**
- Modify: `apps/core-engine/internal/match/repository/match_repo.go:51-170`
- Create: `apps/core-engine/internal/match/repository/products_query_test.go`
- Modify: `apps/core-engine/internal/match/service/match_service.go:265-279`
- Modify: `apps/core-engine/internal/match/service/match_service_test.go`, `match_service_benchmark_test.go` (rename calls)
- Modify: `apps/core-engine/internal/match/entity/entity.go:19-22`
- Modify: `apps/core-engine/cmd/server/main.go:97`
- Modify: `docs/superpowers/specs/2026-10-01-reference-products-design.md` (the `CreateProduct` line)

**Interfaces:**
- Produces: `productsQuery(brandID string) (string, []any)` (package-private), `(*MatchRepo).AddInMemoryProduct(ctx, entity.ProductCatalogItem) (*entity.ProductCatalogItem, error)` — returns an error when the repo has a database.
- Removed: `MatchRepo.CreateProduct/UpdateProduct/DeleteProduct`, `MatchService.CreateProduct/UpdateProduct/DeleteProduct`, `ProductCatalogItem.TableName`.

- [ ] **Step 1: Write the failing test**

```go
package repository

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/match/entity"
)

func TestProductsQueryReadsRefProducts(t *testing.T) {
	q, args := productsQuery("")
	for _, want := range []string{"FROM ref_products p", "p.deleted_at IS NULL", "AND p.is_active", "p.is_active AS is_active"} {
		if !strings.Contains(q, want) {
			t.Errorf("query lacks %q:\n%s", want, q)
		}
	}
	// An unknown category or texture is empty, never an invented value.
	for _, banned := range []string{"'Skincare'", "'Gel'", "TRUE AS is_active"} {
		if strings.Contains(q, banned) {
			t.Errorf("query still contains %s", banned)
		}
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want none without a brand", args)
	}
	if _, args := productsQuery("brd-1"); len(args) != 2 {
		t.Errorf("brand filter args = %v, want id and name", args)
	}
}

func TestAddInMemoryProductRefusesDatabase(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=invalid"}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMatchRepo(db).AddInMemoryProduct(context.Background(), entity.ProductCatalogItem{ID: "p"}); err == nil {
		t.Fatal("products are written through reference-service; with a database this must fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/core-engine && go test ./internal/match/repository/ -run 'TestProductsQuery|TestAddInMemory' -v`
Expected: FAIL, `undefined: productsQuery`.

- [ ] **Step 3: Implement**

Replace the DB branch of `ListProducts` (from `var res []entity.ProductCatalogItem` / `rawQuery := ...` through the `if/else` that runs it) with:

```go
	var res []entity.ProductCatalogItem
	q, args := productsQuery(brandID)
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&res).Error; err != nil {
		return nil, err
	}
```

and add, above `ListProducts`:

```go
// productsQuery reads the product master reference-service owns
// (ref_products, docs/superpowers/specs/2026-10-01-reference-products-design.md).
// A product without a category or texture gets an empty one, never a guess.
func productsQuery(brandID string) (string, []any) {
	q := `
		SELECT
			p.id AS id,
			p.brand_id::text AS brand_id,
			p.name AS name,
			COALESCE(c.name, '') AS category,
			COALESCE(t.name, '') AS texture,
			COALESCE(
				(SELECT string_agg(ing.name, ', ')
				 FROM ref_product_ingredients rpi
				 JOIN ref_ingredients ing ON ing.id = rpi.ingredient_id
				 WHERE rpi.product_id = p.id), ''
			) AS ingredients,
			COALESCE(p.description, '') AS key_actives,
			COALESCE(p.image_url, '') AS image_url,
			p.is_active AS is_active,
			p.created_at AS created_at,
			p.updated_at AS updated_at
		FROM ref_products p
		LEFT JOIN ref_brands b ON b.id = p.brand_id
		LEFT JOIN ref_categories c ON c.id = p.category_id
		LEFT JOIN ref_textures t ON t.id = p.texture_id
		WHERE p.deleted_at IS NULL AND p.is_active`
	if brandID == "" || brandID == "*" {
		return q + " ORDER BY c.name ASC, p.name ASC", nil
	}
	return q + " AND (LOWER(p.brand_id::text) = LOWER(?) OR LOWER(b.name) = LOWER(?)) ORDER BY c.name ASC, p.name ASC",
		[]any{brandID, brandID}
}
```

Replace `CreateProduct`, `UpdateProduct` and `DeleteProduct` in `match_repo.go` with:

```go
// AddInMemoryProduct loads a product into the in-memory repo (no database:
// tests and local runs without Postgres). With a database it refuses:
// products are written through reference-service, and a write here would
// vanish on restart while looking saved.
func (r *MatchRepo) AddInMemoryProduct(ctx context.Context, product entity.ProductCatalogItem) (*entity.ProductCatalogItem, error) {
	if !r.useMemory {
		return nil, fmt.Errorf("match products are read-only here; write them through reference-service /api/reference/products")
	}
	if product.ID == "" {
		product.ID = fmt.Sprintf("prod-%d", time.Now().UnixNano())
	}
	product.IsActive = true
	product.CreatedAt = time.Now()

	r.memMu.Lock()
	defer r.memMu.Unlock()
	r.products = append([]entity.ProductCatalogItem{product}, r.products...)
	if r.cache != nil {
		r.cache.InvalidatePrefix(ctx, "match:products:")
	}
	return &product, nil
}
```

If `domain` is no longer used in `match_repo.go` after removing `DeleteProduct`, check with `go build` (other functions use `domain.ErrNotFound`; leave the import if so).

In `match_service.go`, delete `CreateProduct`, `UpdateProduct`, `DeleteProduct` (keep `ListProducts`).

In the two service test files, rename every `repo.CreateProduct(` to `repo.AddInMemoryProduct(`:

```bash
sed -i 's/repo\.CreateProduct(/repo.AddInMemoryProduct(/g' apps/core-engine/internal/match/service/match_service_test.go apps/core-engine/internal/match/service/match_service_benchmark_test.go
```

In `entity.go`, delete:

```go
func (ProductCatalogItem) TableName() string {
	return "match_product_catalog"
}
```

and change the struct's comment to:

```go
// ProductCatalogItem is match's read model of a product. It is scanned from
// ref_products (reference-service owns that table); it has no table of its own.
```

In `cmd/server/main.go`, delete the line `&matchEntity.ProductCatalogItem{},` from the `AutoMigrate` list.

In the spec, replace the bullet that starts `- The in-memory \`CreateProduct\`` with:

```markdown
- `UpdateProduct` / `DeleteProduct` and the three service methods are deleted.
  `CreateProduct` becomes `AddInMemoryProduct`, which the match tests use to
  load products into the in-memory repo, and which refuses to run with a
  database. Products are written through reference-service.
```

- [ ] **Step 4: Run tests and build**

Run: `cd apps/core-engine && go build ./... && go test ./internal/match/... ./cmd/server/...`
Expected: PASS. `grep -rn "CreateProduct\|UpdateProduct\|DeleteProduct" apps/core-engine --include=*.go` shows only `CreateProductGroup` / `UpdateProductGroup` / `DeleteProductGroup`.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/internal/match apps/core-engine/cmd/server/main.go docs/superpowers/specs/2026-10-01-reference-products-design.md
git commit -m "feat(match): read products from ref_products; drop the in-memory product writes"
```

---

### Task 6: try-on colour candidates join `ref_products`

**Files:**
- Modify: `apps/core-engine/internal/colour/repository/repo.go:37-46`
- Modify: `apps/core-engine/internal/colour/repository/repo_test.go`

**Interfaces:**
- Consumes: nothing new. Produces: unchanged `ListCandidates` / `GetCandidates`.

- [ ] **Step 1: Write the failing test** (append to `repo_test.go`; add imports `context`, `strings`, `gorm.io/driver/postgres`, `gorm.io/gorm`)

```go
// Try-on lists shades of live products from the product master.
func TestQueryJoinsRefProducts(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=invalid"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var rows []row
		return New(tx).query(context.Background()).Scan(&rows)
	})
	for _, want := range []string{"JOIN ref_products AS p ON p.id = s.product_id", "p.deleted_at IS NULL", "p.is_active"} {
		if !strings.Contains(sql, want) {
			t.Errorf("query lacks %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "match_product_catalog") {
		t.Errorf("query still reads match_product_catalog:\n%s", sql)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/core-engine && go test ./internal/colour/repository/ -run TestQueryJoinsRefProducts -v`
Expected: FAIL, "query still reads match_product_catalog".

- [ ] **Step 3: Implement** — in `query`, replace the last two lines:

```go
		Joins("JOIN ref_products AS p ON p.id = s.product_id").
		Where("p.deleted_at IS NULL AND p.is_active")
```

and change the `Repo` doc comment to `// Repo reads catalog shades joined with their colorimetry and their product in ref_products.`

- [ ] **Step 4: Run tests**

Run: `cd apps/core-engine && go test ./internal/colour/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/internal/colour/repository/
git commit -m "feat(colour): try-on candidates read products from ref_products"
```

---

### Task 7: `refcatalog` — SQL writes for the offline commands

**Files:**
- Create: `apps/core-engine/internal/refcatalog/refcatalog.go`
- Create: `apps/core-engine/internal/refcatalog/refcatalog_test.go`

**Interfaces:**
- Produces:
  - `var ErrTablesMissing = errors.New(...)`
  - `RequireTables(db *gorm.DB) error`
  - `Code(s string) string` — lower-case, trimmed.
  - `EnsureCategory(tx *gorm.DB, code, name string) (string, error)` — `""` for an empty code; id `cat-<code>` when created.
  - `EnsureTexture(tx *gorm.DB, code, name string) (string, error)` — same, `tex-<code>`.
  - `type Product struct { ID, BrandID, CategoryID, TextureID, Name, Description, ImageURL string; IsActive bool }`
  - `InsertProductIfAbsent(tx *gorm.DB, p Product) (bool, error)` — never overwrites.
  - `UpsertProduct(tx *gorm.DB, p Product) error` — on conflict updates `brand_id`, `category_id`, `name`, `updated_at` only.
  - `IngredientIDsByName(tx *gorm.DB) (map[string]string, error)` — key `Code(name)`.
  - `SetIngredients(tx *gorm.DB, productID string, ingredientIDs []string) error` — inserts, ignoring existing links.
  - `SplitIngredients(list string, byName map[string]string) (ids, unmatched []string)` — pure.

- [ ] **Step 1: Write the failing tests**

```go
package refcatalog

import (
	"os"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSplitIngredients(t *testing.T) {
	by := map[string]string{"niacinamide": "ing-1", "hyaluronic acid": "ing-2"}
	ids, unmatched := SplitIngredients(" Niacinamide, Hyaluronic Acid ,Glitter,, niacinamide", by)
	if !reflect.DeepEqual(ids, []string{"ing-1", "ing-2"}) {
		t.Errorf("ids = %v, want [ing-1 ing-2] (deduplicated, in order)", ids)
	}
	if !reflect.DeepEqual(unmatched, []string{"Glitter"}) {
		t.Errorf("unmatched = %v, want [Glitter]", unmatched)
	}
}

func TestCode(t *testing.T) {
	if got := Code("  Lip Cream "); got != "lip cream" {
		t.Errorf("Code = %q", got)
	}
}

// scratchDB needs a disposable Postgres *database* (Neon ignores
// search_path, so a schema is not isolation). It creates the ref product
// tables as reference-service's models define them and deletes their rows.
// Never point it at a shared database.
func scratchDB(t *testing.T) *gorm.DB {
	url := os.Getenv("REFCATALOG_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("REFCATALOG_TEST_DATABASE_URL not set (scratch Postgres only)")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS ref_categories (id varchar(64) PRIMARY KEY, code varchar(64) NOT NULL UNIQUE, name text NOT NULL, description text, created_at timestamptz, updated_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS ref_textures (id varchar(64) PRIMARY KEY, code varchar(64) NOT NULL UNIQUE, name text NOT NULL, description text, created_at timestamptz, updated_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS ref_ingredients (id text PRIMARY KEY, code text, name text NOT NULL UNIQUE, category text, description text, created_at timestamptz, updated_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS ref_products (id varchar(64) PRIMARY KEY, code varchar(64), brand_id varchar(64) NOT NULL, category_id varchar(64), texture_id varchar(64), name text NOT NULL, description text, image_url varchar(512), is_active boolean NOT NULL DEFAULT true, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS ref_product_ingredients (product_id varchar(64), ingredient_id varchar(64), PRIMARY KEY (product_id, ingredient_id))`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	clean := func() {
		for _, tbl := range []string{"ref_product_ingredients", "ref_products", "ref_categories", "ref_textures", "ref_ingredients"} {
			db.Exec("DELETE FROM " + tbl)
		}
	}
	clean()
	t.Cleanup(clean)
	return db
}

func TestEnsureCategoryIsIdempotent(t *testing.T) {
	db := scratchDB(t)
	a, err := EnsureCategory(db, "lip", "lip")
	if err != nil || a != "cat-lip" {
		t.Fatalf("first = %q %v", a, err)
	}
	b, err := EnsureCategory(db, "LIP ", "Lip")
	if err != nil || b != a {
		t.Fatalf("second = %q %v, want %q", b, err, a)
	}
	if id, _ := EnsureCategory(db, "", ""); id != "" {
		t.Errorf("empty code must give no category, got %q", id)
	}
}

func TestInsertProductIfAbsentNeverOverwrites(t *testing.T) {
	db := scratchDB(t)
	p := Product{ID: "prd-1", BrandID: "brd-1", Name: "Original", IsActive: true}
	if ok, err := InsertProductIfAbsent(db, p); err != nil || !ok {
		t.Fatalf("insert = %v %v", ok, err)
	}
	p.Name = "Changed"
	if ok, err := InsertProductIfAbsent(db, p); err != nil || ok {
		t.Fatalf("second insert = %v %v, want false", ok, err)
	}
	var name string
	db.Raw("SELECT name FROM ref_products WHERE id = 'prd-1'").Scan(&name)
	if name != "Original" {
		t.Errorf("name = %q, the existing row must be left alone", name)
	}
}

func TestUpsertProductKeepsCreatedAtAndActive(t *testing.T) {
	db := scratchDB(t)
	db.Exec(`INSERT INTO ref_products (id, brand_id, name, is_active, created_at, updated_at) VALUES ('prd-1','brd-1','Old',false,'2026-01-01','2026-01-01')`)
	if err := UpsertProduct(db, Product{ID: "prd-1", BrandID: "brd-2", Name: "New", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		BrandID, Name string
		IsActive      bool
		CreatedAt     string
	}
	db.Raw("SELECT brand_id, name, is_active, created_at::date::text AS created_at FROM ref_products WHERE id = 'prd-1'").Scan(&got)
	if got.BrandID != "brd-2" || got.Name != "New" || got.IsActive || got.CreatedAt != "2026-01-01" {
		t.Errorf("got %+v: name and brand update; is_active and created_at stay", got)
	}
}

func TestRequireTablesWithoutProducts(t *testing.T) {
	db := scratchDB(t)
	if err := RequireTables(db); err != nil {
		t.Fatalf("tables exist, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core-engine && go test ./internal/refcatalog/ -v`
Expected: FAIL to compile (`undefined: SplitIngredients`). The scratch tests skip without `REFCATALOG_TEST_DATABASE_URL`.

- [ ] **Step 3: Implement**

```go
// Package refcatalog writes the product master that reference-service owns
// (ref_products, ref_categories, ref_textures, ref_product_ingredients) for
// core-engine's offline commands: migrate-product-catalog and
// seed-colour-shades. It uses SQL because the GORM models are in
// reference-service's internal package, and it never creates or alters those
// tables: reference-service's AutoMigrate does (docs/SHARED-DATABASE.md).
package refcatalog

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var ErrTablesMissing = errors.New("ref product tables missing: start reference-service against this database once so its AutoMigrate creates them")

var tables = []string{"ref_products", "ref_categories", "ref_textures", "ref_product_ingredients", "ref_ingredients"}

// RequireTables fails unless every ref product table exists.
func RequireTables(db *gorm.DB) error {
	var missing []string
	for _, t := range tables {
		if !db.Migrator().HasTable(t) {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrTablesMissing, strings.Join(missing, ", "))
	}
	return nil
}

// Code normalises a category or texture text into its code.
func Code(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func ensure(tx *gorm.DB, table, prefix, code, name string) (string, error) {
	code = Code(code)
	if code == "" {
		return "", nil
	}
	var ids []string
	if err := tx.Raw("SELECT id FROM "+table+" WHERE code = ? LIMIT 1", code).Scan(&ids).Error; err != nil {
		return "", err
	}
	if len(ids) > 0 {
		return ids[0], nil
	}
	id := prefix + code
	if err := tx.Exec("INSERT INTO "+table+" (id, code, name, description, created_at, updated_at) VALUES (?, ?, ?, '', now(), now())",
		id, code, strings.TrimSpace(name)).Error; err != nil {
		return "", fmt.Errorf("%s %q: %w", table, code, err)
	}
	return id, nil
}

// EnsureCategory returns the id of the category with this code, creating it
// (id cat-<code>) if absent. An empty code means no category: "".
func EnsureCategory(tx *gorm.DB, code, name string) (string, error) {
	return ensure(tx, "ref_categories", "cat-", code, name)
}

// EnsureTexture is EnsureCategory for ref_textures (id tex-<code>).
func EnsureTexture(tx *gorm.DB, code, name string) (string, error) {
	return ensure(tx, "ref_textures", "tex-", code, name)
}

// Product is one ref_products row. Empty CategoryID / TextureID are stored as
// NULL.
type Product struct {
	ID, BrandID, CategoryID, TextureID, Name, Description, ImageURL string
	IsActive                                                        bool
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

const insertProduct = `INSERT INTO ref_products
	(id, code, brand_id, category_id, texture_id, name, description, image_url, is_active, created_at, updated_at)
	VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, now(), now())`

// InsertProductIfAbsent inserts p unless its id exists, and reports whether
// it did. An existing row is never changed.
func InsertProductIfAbsent(tx *gorm.DB, p Product) (bool, error) {
	res := tx.Exec(insertProduct+" ON CONFLICT (id) DO NOTHING",
		p.ID, p.BrandID, nullable(p.CategoryID), nullable(p.TextureID), p.Name, p.Description, p.ImageURL, p.IsActive)
	return res.RowsAffected == 1, res.Error
}

// UpsertProduct inserts p, or updates brand, category and name of the row
// with its id. is_active, deleted_at, created_at and the fields an editor
// owns (description, image) are left as they are.
func UpsertProduct(tx *gorm.DB, p Product) error {
	return tx.Exec(insertProduct+` ON CONFLICT (id) DO UPDATE SET
		brand_id = EXCLUDED.brand_id, category_id = EXCLUDED.category_id,
		name = EXCLUDED.name, updated_at = now()`,
		p.ID, p.BrandID, nullable(p.CategoryID), nullable(p.TextureID), p.Name, p.Description, p.ImageURL, p.IsActive).Error
}

// IngredientIDsByName maps Code(name) to id for every ref_ingredients row.
func IngredientIDsByName(tx *gorm.DB) (map[string]string, error) {
	var rows []struct{ ID, Name string }
	if err := tx.Raw("SELECT id, name FROM ref_ingredients").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[Code(r.Name)] = r.ID
	}
	return out, nil
}

// SplitIngredients splits a comma-separated list and resolves each name.
// Names with no ingredient row come back in unmatched, as written; they are
// never created.
func SplitIngredients(list string, byName map[string]string) (ids, unmatched []string) {
	seen := map[string]bool{}
	for _, part := range strings.Split(list, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		id, ok := byName[Code(name)]
		if !ok {
			unmatched = append(unmatched, name)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, unmatched
}

// SetIngredients links a product to ingredients; existing links stay.
func SetIngredients(tx *gorm.DB, productID string, ingredientIDs []string) error {
	for _, id := range ingredientIDs {
		if err := tx.Exec("INSERT INTO ref_product_ingredients (product_id, ingredient_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
			productID, id).Error; err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd apps/core-engine && go test ./internal/refcatalog/ -v`
Expected: `TestSplitIngredients`, `TestCode` PASS; scratch tests SKIP. If you have a disposable Postgres *database*, also run with `REFCATALOG_TEST_DATABASE_URL` set and expect all PASS. Never set it to a shared Neon URL.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/internal/refcatalog/
git commit -m "feat(core-engine): refcatalog writes for the ref product tables"
```

---

### Task 8: `migrate-product-catalog` command

**Files:**
- Create: `apps/core-engine/cmd/migrate-product-catalog/main.go`
- Create: `apps/core-engine/cmd/migrate-product-catalog/main_test.go`

**Interfaces:**
- Consumes: Task 7 `refcatalog`.
- Produces: `go run ./apps/core-engine/cmd/migrate-product-catalog [-dry-run]`; pure `toProduct(old oldProduct, categoryID, textureID string) refcatalog.Product`.

- [ ] **Step 1: Write the failing test**

```go
package main

import "testing"

// The id is kept, so match_shades.product_id and product groups still point
// at it; key actives become the description, which match reads back as key
// actives.
func TestToProductKeepsIDAndKeyActives(t *testing.T) {
	old := oldProduct{ID: "clr-prd-e41ea982f8fcb07e", BrandID: "brd-1", Name: "Make Over Blush On Single",
		KeyActives: "pigment", ImageURL: "https://x/y.png", IsActive: true}
	p := toProduct(old, "cat-blush", "")
	if p.ID != old.ID || p.BrandID != "brd-1" || p.Name != old.Name || p.CategoryID != "cat-blush" ||
		p.TextureID != "" || p.Description != "pigment" || p.ImageURL != old.ImageURL || !p.IsActive {
		t.Errorf("got %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/core-engine && go test ./cmd/migrate-product-catalog/ -v`
Expected: FAIL, `undefined: oldProduct`.

- [ ] **Step 3: Implement**

```go
// One-off: copies match_product_catalog into ref_products, the product master
// reference-service owns, keeping every id so match_shades.product_id and
// match_product_groups still point at the same products
// (docs/superpowers/specs/2026-10-01-reference-products-design.md).
//
// Run once per database, after reference-service has started against it (its
// AutoMigrate creates the ref tables), with -dry-run first:
//
//	go run ./apps/core-engine/cmd/migrate-product-catalog -dry-run
//	go run ./apps/core-engine/cmd/migrate-product-catalog
//
// Categories and textures are created from the old text columns (code = the
// text lower-cased). Ingredient names are matched against ref_ingredients;
// unmatched names are reported, never created. A product id already in
// ref_products is reported and left alone. Everything runs in one
// transaction; -dry-run rolls it back.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"

	"gorm.io/gorm"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/apps/core-engine/internal/refcatalog"
	"github.com/experience-gateway/packages/goplatform"
)

type oldProduct struct {
	ID, BrandID, Name, Category, Texture, Ingredients, KeyActives, ImageURL string
	IsActive                                                                bool
}

func toProduct(o oldProduct, categoryID, textureID string) refcatalog.Product {
	return refcatalog.Product{
		ID: o.ID, BrandID: o.BrandID, CategoryID: categoryID, TextureID: textureID,
		Name: o.Name, Description: o.KeyActives, ImageURL: o.ImageURL, IsActive: o.IsActive,
	}
}

var errDryRun = errors.New("dry run: rolled back")

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would be written, write nothing")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	db, err := goplatform.OpenPostgres(context.Background(), cfg.DatabaseURL, goplatform.DBOptions{
		Logger:             slog.Default(),
		SlowQueryThreshold: cfg.DBSlowQueryThreshold,
		PingTimeout:        cfg.DependencyPingTimeout,
	})
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	if err := refcatalog.RequireTables(db); err != nil {
		log.Fatal(err)
	}
	if !db.Migrator().HasTable("match_product_catalog") {
		log.Fatal("match_product_catalog not found: nothing to migrate")
	}

	var copied, kept, linked int
	unmatched := map[string]int{}
	err = db.Transaction(func(tx *gorm.DB) error {
		var olds []oldProduct
		if err := tx.Raw(`SELECT id, brand_id, name, category, texture, ingredients, key_actives,
			COALESCE(image_url, '') AS image_url, is_active FROM match_product_catalog ORDER BY id`).Scan(&olds).Error; err != nil {
			return err
		}
		byName, err := refcatalog.IngredientIDsByName(tx)
		if err != nil {
			return err
		}
		for _, o := range olds {
			catID, err := refcatalog.EnsureCategory(tx, o.Category, o.Category)
			if err != nil {
				return err
			}
			texID, err := refcatalog.EnsureTexture(tx, o.Texture, o.Texture)
			if err != nil {
				return err
			}
			inserted, err := refcatalog.InsertProductIfAbsent(tx, toProduct(o, catID, texID))
			if err != nil {
				return err
			}
			if !inserted {
				kept++
				log.Printf("kept: %s (%q) is already in ref_products; left unchanged", o.ID, o.Name)
				continue
			}
			copied++
			ids, missing := refcatalog.SplitIngredients(o.Ingredients, byName)
			for _, m := range missing {
				unmatched[m]++
			}
			if err := refcatalog.SetIngredients(tx, o.ID, ids); err != nil {
				return err
			}
			linked += len(ids)
		}
		if *dryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		log.Fatalf("migrating: %v", err)
	}
	log.Printf("copied %d products, kept %d already present, linked %d ingredients", copied, kept, linked)
	for name, n := range unmatched {
		log.Printf("ingredient %q (%d products) has no ref_ingredients row; not linked", name, n)
	}
	if *dryRun {
		log.Print("dry run: nothing was written")
	}
}
```

- [ ] **Step 4: Run tests and build**

Run: `cd apps/core-engine && go test ./cmd/migrate-product-catalog/ -v && go build ./cmd/migrate-product-catalog/`
Expected: PASS. Do not run the command against a database in this task.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/cmd/migrate-product-catalog/
git commit -m "feat(core-engine): one-off migration of match_product_catalog into ref_products"
```

---

### Task 9: seeder writes `ref_products`

**Files:**
- Modify: `apps/core-engine/cmd/seed-colour-shades/main.go` (header comment, imports, product block at `:264-276`, before the transaction)
- Modify: `apps/core-engine/cmd/seed-colour-shades/main_test.go`

**Interfaces:**
- Consumes: Task 7 `refcatalog.RequireTables`, `EnsureCategory`, `UpsertProduct`.

- [ ] **Step 1: Write the failing test** (append to `main_test.go`)

```go
// Product ids must not change: match_shades, product groups and the
// migrated ref_products rows already use them. Pinned against the id stored
// in the neon-seagull database on 2026-10-01.
func TestProductIDIsStable(t *testing.T) {
	if got := id("clr-prd-", "brd-1790654876204254300", "Make Over Blush On Single"); got != "clr-prd-e41ea982f8fcb07e" {
		t.Errorf("product id = %q, want clr-prd-e41ea982f8fcb07e", got)
	}
}

func TestProductFromRow(t *testing.T) {
	p := productFor("brd-1", "cat-lip", csvRow{produk: "Wardah Colorfit Velvet Matte Lip Mousse"})
	if p.ID != id("clr-prd-", "brd-1", "Wardah Colorfit Velvet Matte Lip Mousse") || p.CategoryID != "cat-lip" ||
		p.BrandID != "brd-1" || p.Name != "Wardah Colorfit Velvet Matte Lip Mousse" || !p.IsActive {
		t.Errorf("got %+v", p)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/core-engine && go test ./cmd/seed-colour-shades/ -v`
Expected: `TestProductIDIsStable` PASS (it pins today's formula), `TestProductFromRow` FAIL: `undefined: productFor`.

- [ ] **Step 3: Implement**

Add the import `"github.com/experience-gateway/apps/core-engine/internal/refcatalog"`; remove `matchEntity` if nothing else uses it (shades still use `matchEntity.Shade`, so it stays).

Add:

```go
// productFor is the ref_products row for a CSV product. The id formula is
// pinned by TestProductIDIsStable.
func productFor(brandID, categoryID string, r csvRow) refcatalog.Product {
	return refcatalog.Product{
		ID: id("clr-prd-", brandID, r.produk), BrandID: brandID, CategoryID: categoryID,
		Name: r.produk, IsActive: true,
	}
}
```

After the `db, err := goplatform.OpenPostgres(...)` block, add:

```go
	if err := refcatalog.RequireTables(db); err != nil {
		log.Fatal(err)
	}
```

Replace the product block inside the transaction:

```go
			prodID := id("clr-prd-", brands[i], r.produk)
			if !seenProd[prodID] {
				seenProd[prodID] = true
				var p matchEntity.ProductCatalogItem
				if err := tx.Where("id = ?", prodID).First(&p).Error; err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				p.ID, p.BrandID, p.Name, p.Category, p.IsActive = prodID, brands[i], r.produk, r.kategori, true
				if err := tx.Save(&p).Error; err != nil {
					return fmt.Errorf("product %q: %w", r.produk, err)
				}
				nProd++
			}
```

with:

```go
			prodID := id("clr-prd-", brands[i], r.produk)
			if !seenProd[prodID] {
				seenProd[prodID] = true
				catID, err := refcatalog.EnsureCategory(tx, r.kategori, r.kategori)
				if err != nil {
					return fmt.Errorf("category %q: %w", r.kategori, err)
				}
				if err := refcatalog.UpsertProduct(tx, productFor(brands[i], catID, r)); err != nil {
					return fmt.Errorf("product %q: %w", r.produk, err)
				}
				nProd++
			}
```

In the header comment, replace `// match_product_catalog + match_shades + match_shade_colorimetry.` with:

```go
// ref_products (+ ref_categories) + match_shades + match_shade_colorimetry.
// The ref tables belong to reference-service and are created by its
// AutoMigrate, so it must have started against this database once.
```

- [ ] **Step 4: Run tests and build**

Run: `cd apps/core-engine && go test ./cmd/seed-colour-shades/ -v && go build ./... && go vet ./cmd/seed-colour-shades/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/core-engine/cmd/seed-colour-shades/
git commit -m "feat(core-engine): seed-colour-shades writes products to ref_products"
```

---

### Task 10: docs

**Files:**
- Modify: `docs/SHARED-DATABASE.md`
- Modify: `docs/COLOUR-LOCAL-RUN.md`
- Modify: `apps/core-engine/seed/vto_products_demo.md` (one line)

- [ ] **Step 1: `docs/SHARED-DATABASE.md`** — add a section before `## If you want this to stop being a hazard`:

```markdown
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

`match_product_catalog` is retired: core-engine no longer migrates or reads
it. Its rows were copied into `ref_products` with the same ids by
`migrate-product-catalog`. Dropping the table is a separate recorded step,
not done yet.
```

- [ ] **Step 2: `docs/COLOUR-LOCAL-RUN.md`** — where the seeder is described (`grep -n seed-colour-shades docs/COLOUR-LOCAL-RUN.md`), add one sentence after the command: `The seeder writes products to ref_products, which reference-service creates: start reference-service against the same DATABASE_URL once before the first seed.`

- [ ] **Step 3: `apps/core-engine/seed/vto_products_demo.md`** — change the second paragraph's first sentence to end `Seeded with \`cmd/seed-colour-shades\` into \`ref_products\` (see \`docs/COLOUR-LOCAL-RUN.md\`).`

- [ ] **Step 4: Commit**

```bash
git add docs/SHARED-DATABASE.md docs/COLOUR-LOCAL-RUN.md apps/core-engine/seed/vto_products_demo.md
git commit -m "docs: ref_products is the product master; match_product_catalog retired"
```

---

### Task 11: roll out to the databases (needs the user, step by step)

Shared databases. **Ask the user before each write step**; report results between them.

- [ ] **Step 1:** Full test run: `go test ./...` in `apps/reference-service` and `apps/core-engine`. Report output.
- [ ] **Step 2:** Ask the user how reference-service reaches each database (local run with that `DATABASE_URL`, or the VPS deploy — see memory `vps-deploy`). Starting it creates the four tables via `AutoMigrate`. Needs the `runtime` lane if run locally (port per `docs/PORTS.md`).
- [ ] **Step 3:** Per database (neon-seagull `ep-sparkling-cherry`, then the core-engine `.env` one `ep-hidden-unit`): `go run ./apps/core-engine/cmd/migrate-product-catalog -dry-run` with that `DATABASE_URL`; show the user the report (expect `copied 31 products`).
- [ ] **Step 4:** On approval, run without `-dry-run`; verify read-only: `SELECT count(*) FROM ref_products` = 31, and every `match_shades.product_id` has a `ref_products` row:
  `SELECT count(*) FROM match_shades s LEFT JOIN ref_products p ON p.id = s.product_id WHERE p.id IS NULL` = 0.
- [ ] **Step 5:** Deploy core-engine (user's call); then `GET /api/reference/products` and `GET /api/matching/products` each list 31 products, and try-on still lists 243 shades.
- [ ] **Step 6:** Release the lanes in `docs/LANES.md`.

---

## Self-review notes

- Spec coverage: schema (T1), API incl. `/all` and brand filter (T2–T4), match/colour reads and no fallbacks (T5–T6), core-engine stops migrating the old table (T5), migration incl. dry run / no overwrite / unmatched ingredients (T7–T8), seeder (T9), deploy order (T11), docs (T10). Dropping `match_product_catalog` and seagull-web changes stay out, as in the spec.
- `refcatalog`'s scratch-test DDL mirrors Task 1's models by hand; if the models change, change the DDL too.
