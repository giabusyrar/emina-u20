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
		if err := tx.Raw(`SELECT COALESCE(id, '') AS id, COALESCE(brand_id, '') AS brand_id, COALESCE(name, '') AS name,
			COALESCE(category, '') AS category, COALESCE(texture, '') AS texture,
			COALESCE(ingredients, '') AS ingredients, COALESCE(key_actives, '') AS key_actives,
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
