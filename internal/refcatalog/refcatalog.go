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

func ensure(tx *gorm.DB, table, prefix, code, name string) (id string, created bool, err error) {
	code = Code(code)
	if code == "" {
		return "", false, nil
	}
	var ids []string
	if err := tx.Raw("SELECT id FROM "+table+" WHERE code = ? LIMIT 1", code).Scan(&ids).Error; err != nil {
		return "", false, err
	}
	if len(ids) > 0 {
		return ids[0], false, nil
	}
	id = prefix + code
	if err := tx.Exec("INSERT INTO "+table+" (id, code, name, description, created_at, updated_at) VALUES (?, ?, ?, '', now(), now())",
		id, code, strings.TrimSpace(name)).Error; err != nil {
		return "", false, fmt.Errorf("%s %q: %w", table, code, err)
	}
	return id, true, nil
}

// EnsureCategory returns the id of the category with this code, creating it
// (id cat-<code>) if absent, and whether it created it. An empty code means
// no category: "".
func EnsureCategory(tx *gorm.DB, code, name string) (id string, created bool, err error) {
	return ensure(tx, "ref_categories", "cat-", code, name)
}

// EnsureTexture is EnsureCategory for ref_textures (id tex-<code>).
func EnsureTexture(tx *gorm.DB, code, name string) (id string, created bool, err error) {
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

// ProductIDByName returns the id of the live (not deleted) product of this
// brand whose name matches, ignoring case and surrounding space, or "" when
// there is none. Products created through reference-service have ids the
// seeder cannot compute, so this is how a CSV row finds them. More than one
// match is an error: shades must not be attached to a guess.
func ProductIDByName(tx *gorm.DB, brandID, name string) (string, error) {
	var ids []string
	if err := tx.Raw(`SELECT id FROM ref_products
		WHERE brand_id = ? AND LOWER(TRIM(name)) = ? AND deleted_at IS NULL`,
		brandID, Code(name)).Scan(&ids).Error; err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", nil
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%d live products of brand %s are named %q: %v", len(ids), brandID, name, ids)
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
