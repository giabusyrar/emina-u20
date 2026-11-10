// Idempotent seeder: loads the VTO catalog CSV used by the WCPA notebook
// (vto_products.csv, or the demo catalog seed/vto_products_demo.csv) into
// ref_products (+ ref_categories) + match_shades + match_shade_colorimetry.
// The ref tables belong to reference-service and are created by its
// AutoMigrate, so it must have started against this database once.
//
// Colours are converted by the colour worker (POST /colour/munsell
// at COLOUR_WORKER_URL; npm run dev:colour) — the same hex→Lab and Lab→Munsell
// code the person measurement uses — so it must be running. Rows are upserted
// by deterministic ids, so re-running updates instead of duplicating.
//
// From the repository root (npm run seed:colour is the same command):
//
//	go run ./cmd/seed-colour-shades -csv path/to/vto_products.csv -brand <brandId>
//	go run ./cmd/seed-colour-shades -csv seed/vto_products_demo.csv \
//	    -brand-map "Make Over=<brandId>,Wardah=<brandId>,OMG=<brandId>"
//
// CSV columns: kategori, produk, shade, hex, alpha, specular, mode, sumber;
// optional brand, munsell_v, munsell_c.
//
//   - sumber says where the hex comes from: [SWATCH] (the product's swatch)
//     or [PERKIRAAN] (estimated for a demo, not from the brand; the API
//     reports colourSource "estimate" and the UI says so). Any other value
//     stops the seeder: a colour of unknown origin must not reach the UI
//     looking like a real one.
//   - munsell_v + munsell_c (cube readings) make the row source "cube".
//   - brand names the row's brand; -brand-map gives each name its gateway
//     brand id. Rows without a brand use -brand.
package main

import (
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/adapters/workerclient"
	colourDomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/domain"
	colourEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/colour/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/config"
	matchEntity "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/match/entity"
	goplatform "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/platform"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/refcatalog"
)

// Shade.Region is lip | eye | cheek | skin (match domain + LiveTryOnCanvas).
// The exact category is kept on the colorimetry row.
var regionFor = map[string]string{
	"lip":        "lip",
	"blush":      "cheek",
	"complexion": "skin",
	"brow":       "eye",
	"eyeliner":   "eye",
	"mascara":    "eye",
	"eyeshadow":  "eye",
}

func id(prefix string, parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	return prefix + hex.EncodeToString(h[:8])
}

type csvRow struct {
	line                                    int
	kategori, produk, shade, hexColor, mode string
	brand, source                           string
	alpha, specular                         float64
	cubeV, cubeC                            *float64
}

// sourceOf maps the CSV `sumber` tag to the colour source of the row's hex.
func sourceOf(tag string) (string, error) {
	switch strings.ToUpper(strings.Trim(strings.TrimSpace(tag), "[]")) {
	case "SWATCH":
		return colourDomain.SourceSwatch, nil
	case "PERKIRAAN":
		return colourDomain.SourceEstimate, nil
	}
	return "", fmt.Errorf("sumber %q: want [SWATCH] (product swatch) or [PERKIRAAN] (estimate)", tag)
}

// parseBrandMap reads "Make Over=id1,Wardah=id2" (names case-insensitive).
func parseBrandMap(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		name, id, ok := strings.Cut(part, "=")
		name, id = strings.TrimSpace(name), strings.TrimSpace(id)
		if !ok || name == "" || id == "" {
			return nil, fmt.Errorf("-brand-map entry %q: want <brand name>=<brand id>", part)
		}
		out[strings.ToLower(name)] = id
	}
	return out, nil
}

func readCSV(path string) ([]csvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(h))] = i
	}
	for _, need := range []string{"kategori", "produk", "shade", "hex", "alpha"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("missing column %q", need)
		}
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	optFloat := func(s string) *float64 {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return &v
		}
		return nil
	}
	var rows []csvRow
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		hx := strings.ToUpper(get(rec, "hex"))
		if len(hx) != 7 || hx[0] != '#' {
			log.Printf("skip line %d: no usable hex (%q)", line, hx)
			continue
		}
		alpha, err := strconv.ParseFloat(get(rec, "alpha"), 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: alpha: %w", line, err)
		}
		spec, _ := strconv.ParseFloat(get(rec, "specular"), 64)
		source, err := sourceOf(get(rec, "sumber"))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, csvRow{
			line:     line,
			kategori: get(rec, "kategori"), produk: get(rec, "produk"), shade: get(rec, "shade"),
			hexColor: hx, mode: get(rec, "mode"), alpha: alpha, specular: spec,
			brand: get(rec, "brand"), source: source,
			cubeV: optFloat(get(rec, "munsell_v")), cubeC: optFloat(get(rec, "munsell_c")),
		})
	}
	return rows, nil
}

// brandIDs resolves every row's brand id, or reports all rows that have none.
func brandIDs(rows []csvRow, fallback string, byName map[string]string) ([]string, error) {
	ids := make([]string, len(rows))
	missing := map[string]bool{}
	for i, r := range rows {
		switch {
		case r.brand != "":
			if id, ok := byName[strings.ToLower(r.brand)]; ok {
				ids[i] = id
			} else {
				missing[fmt.Sprintf("brand %q (add it to -brand-map)", r.brand)] = true
			}
		case fallback != "":
			ids[i] = fallback
		default:
			missing[fmt.Sprintf("line %d has no brand column value and -brand is not set", r.line)] = true
		}
	}
	if len(missing) > 0 {
		var list []string
		for m := range missing {
			list = append(list, m)
		}
		sort.Strings(list)
		return nil, fmt.Errorf("no brand id for: %s", strings.Join(list, "; "))
	}
	return ids, nil
}

// productFor is the ref_products row for a CSV product. The id formula is
// pinned by TestProductIDIsStable.
func productFor(brandID, categoryID string, r csvRow) refcatalog.Product {
	return refcatalog.Product{
		ID: id("clr-prd-", brandID, r.produk), BrandID: brandID, CategoryID: categoryID,
		Name: r.produk, IsActive: true,
	}
}

// resolveProduct picks the product a CSV row's shades belong to. existing is
// the id of the brand's product with that name ("" if none). A product the
// seeder created itself has the computed id and is upserted as before; one
// created through reference-service keeps its own id and row, which the
// seeder does not overwrite: an editor owns it.
func resolveProduct(computed, existing string) (prodID string, upsert bool) {
	if existing == "" || existing == computed {
		return computed, true
	}
	return existing, false
}

func main() {
	csvPath := flag.String("csv", "", "path to vto_products.csv (required)")
	brandID := flag.String("brand", "", "brand id for rows without a brand column value, as used elsewhere in the gateway")
	brandMap := flag.String("brand-map", "", `brand ids by the CSV brand column: "Make Over=<id>,Wardah=<id>,OMG=<id>"`)
	flag.Parse()
	if *csvPath == "" || (*brandID == "" && *brandMap == "") {
		flag.Usage()
		os.Exit(2)
	}
	byName, err := parseBrandMap(*brandMap)
	if err != nil {
		log.Fatal(err)
	}

	rows, err := readCSV(*csvPath)
	if err != nil {
		log.Fatalf("reading %s: %v", *csvPath, err)
	}
	for _, r := range rows {
		if _, ok := regionFor[r.kategori]; !ok {
			log.Fatalf("unknown kategori %q (product %q); add it to regionFor", r.kategori, r.produk)
		}
	}
	brands, err := brandIDs(rows, *brandID, byName)
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	hexes := make([]string, len(rows))
	for i, r := range rows {
		hexes[i] = r.hexColor
	}
	worker := workerclient.New(cfg.ColourWorkerURL, cfg.ColourWorkerTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ColourWorkerTimeout)
	defer cancel()
	colours, err := worker.MunsellFromHex(ctx, hexes)
	if err != nil {
		log.Fatalf("converting colours via the colour worker at %s (COLOUR_WORKER_URL): %v", cfg.ColourWorkerURL, err)
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
	if err := db.AutoMigrate(&colourEntity.ShadeColorimetry{}); err != nil {
		log.Fatalf("migrating match_shade_colorimetry: %v", err)
	}

	var nProd, nShade, nCol, nEstimate int
	err = db.Transaction(func(tx *gorm.DB) error {
		prodFor := map[string]string{} // brand id + produk -> product id
		for i, r := range rows {
			key := brands[i] + "\x1f" + r.produk
			prodID, seen := prodFor[key]
			if !seen {
				existing, err := refcatalog.ProductIDByName(tx, brands[i], r.produk)
				if err != nil {
					return fmt.Errorf("product %q: %w", r.produk, err)
				}
				var upsert bool
				prodID, upsert = resolveProduct(id("clr-prd-", brands[i], r.produk), existing)
				prodFor[key] = prodID
				if upsert {
					catID, _, err := refcatalog.EnsureCategory(tx, r.kategori, r.kategori)
					if err != nil {
						return fmt.Errorf("category %q: %w", r.kategori, err)
					}
					if err := refcatalog.UpsertProduct(tx, productFor(brands[i], catID, r)); err != nil {
						return fmt.Errorf("product %q: %w", r.produk, err)
					}
				} else {
					log.Printf("product %q: shades attach to existing %s (created outside the seeder; row left as is)", r.produk, prodID)
				}
				nProd++
			}

			shadeID := id("clr-shd-", prodID, r.shade)
			var s matchEntity.Shade
			if err := tx.Where("id = ?", shadeID).First(&s).Error; err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			isNew := s.ID == ""
			s.ID, s.ProductID, s.Name, s.HexColor, s.Region = shadeID, prodID, r.shade, r.hexColor, regionFor[r.kategori]
			if isNew {
				s.ExtractionStatus = "pending" // live-camera texture extraction is a separate pipeline
			}
			if err := tx.Save(&s).Error; err != nil {
				return fmt.Errorf("shade %q/%q: %w", r.produk, r.shade, err)
			}
			nShade++

			source, v, c := r.source, colours[i].Munsell.V, colours[i].Munsell.C
			chromaSrc := colours[i].Munsell.ChromaSource
			if r.cubeV != nil && r.cubeC != nil {
				source, v, c, chromaSrc = colourDomain.SourceCube, *r.cubeV, *r.cubeC, "cube"
			} else {
				// The hex's origin is whatever the CSV says now: a swatch row
				// replaces an older estimate of the same shade and vice versa.
				// Cube readings are never removed by a hex row.
				stale := []string{colourDomain.SourceSwatch, colourDomain.SourceEstimate}
				if err := tx.Where("shade_id = ? AND source IN ? AND source <> ?", shadeID, stale, source).
					Delete(&colourEntity.ShadeColorimetry{}).Error; err != nil {
					return fmt.Errorf("colorimetry %q/%q: %w", r.produk, r.shade, err)
				}
			}
			if source == colourDomain.SourceEstimate {
				nEstimate++
			}
			// Load the existing row first, like shades above, so
			// a re-seed updates it without resetting its created_at.
			colID := id("clr-col-", shadeID, source)
			var col colourEntity.ShadeColorimetry
			if err := tx.Where("id = ?", colID).First(&col).Error; err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			now := time.Now()
			col.ID, col.ShadeID, col.Source, col.Category = colID, shadeID, source, r.kategori
			col.LabL, col.LabA, col.LabB = colours[i].Lab[0], colours[i].Lab[1], colours[i].Lab[2]
			col.MunsellV, col.MunsellC, col.ChromaSource = v, c, chromaSrc
			col.Alpha, col.Specular, col.Mode, col.UpdatedAt = r.alpha, r.specular, r.mode, &now
			if err := tx.Save(&col).Error; err != nil {
				return fmt.Errorf("colorimetry %q/%q: %w", r.produk, r.shade, err)
			}
			nCol++
		}
		return nil
	})
	if err != nil {
		log.Fatalf("seeding: %v", err)
	}
	perBrand := map[string]int{}
	for _, b := range brands {
		perBrand[b]++
	}
	log.Printf("seeded %d products, %d shades, %d colorimetry rows; shades per brand id: %v", nProd, nShade, nCol, perBrand)
	if nEstimate > 0 {
		log.Printf("%d shades carry estimated colours ([PERKIRAAN]); the API reports them as colourSource \"estimate\"", nEstimate)
	}
}
