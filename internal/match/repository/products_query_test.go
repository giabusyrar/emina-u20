package repository

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
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
