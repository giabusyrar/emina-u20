package repository

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPreferBestSource(t *testing.T) {
	rows := []row{
		{ShadeID: "a", Source: "estimate", HexColor: "#111111"},
		{ShadeID: "a", Source: "swatch", HexColor: "#222222"},
		{ShadeID: "b", Source: "cube"},
		{ShadeID: "b", Source: "swatch"},
		{ShadeID: "c", Source: "estimate"},
	}
	got := preferBest(rows)
	if len(got) != 3 {
		t.Fatalf("one candidate per shade, got %d", len(got))
	}
	want := map[string]string{"a": "swatch", "b": "cube", "c": "estimate"}
	for _, c := range got {
		if c.ColourSource != want[c.ShadeID] {
			t.Errorf("%s: source %s want %s", c.ShadeID, c.ColourSource, want[c.ShadeID])
		}
	}
	if got[0].ShadeID != "a" || got[1].ShadeID != "b" || got[2].ShadeID != "c" {
		t.Errorf("order must follow the query: %v %v %v", got[0].ShadeID, got[1].ShadeID, got[2].ShadeID)
	}
}

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
