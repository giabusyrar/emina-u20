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
