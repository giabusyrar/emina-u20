// One-time (idempotent) seeder that upserts the Skinverse Skin Character
// ruleset from its checked-in JSON file (apps/core-engine/seed/) into
// score_rulesets — so this ruleset, including its hand-authored custom
// decisionTableNodes (pore-band, sensitivity-subtype), exists somewhere
// besides one developer's local Postgres. Safe to re-run: upserts by
// (brand_id, application_id, code), never creates a duplicate.
//
// NOTE: this only captures the schema as of when it was last run. If the
// live ruleset is edited afterward (via the Skin Grading Studio), re-export
// it into the seed/*.json file and re-run this before relying on it again.
package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"

	"github.com/experience-gateway/apps/core-engine/internal/config"
	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
	"github.com/experience-gateway/packages/goplatform"
	"gorm.io/gorm"
)

type rulesetSeed struct {
	BrandID       string          `json:"brandId"`
	ApplicationID string          `json:"applicationId"`
	Code          string          `json:"code"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Status        string          `json:"status"`
	Schema        json.RawMessage `json:"schema"`
}

func main() {
	seedPath := "apps/core-engine/seed/skinverse_skin_character.json"
	if len(os.Args) > 1 {
		seedPath = os.Args[1]
	}

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		log.Fatalf("reading seed file %s: %v", seedPath, err)
	}
	var seed rulesetSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		log.Fatalf("parsing seed file %s: %v", seedPath, err)
	}

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

	var existing entity.ScoreRuleset
	err = db.Where("brand_id = ? AND application_id = ? AND code = ?", seed.BrandID, seed.ApplicationID, seed.Code).
		First(&existing).Error

	existing.BrandID = seed.BrandID
	existing.ApplicationID = seed.ApplicationID
	existing.Code = seed.Code
	existing.Title = seed.Title
	existing.Description = seed.Description
	existing.Status = seed.Status
	existing.Schema = string(seed.Schema)

	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Fatalf("looking up existing ruleset: %v", err)
		}
		log.Printf("creating new ruleset %s/%s/%s", seed.BrandID, seed.ApplicationID, seed.Code)
	} else {
		log.Printf("updating existing ruleset %s/%s/%s (id=%s)", seed.BrandID, seed.ApplicationID, seed.Code, existing.ID)
	}

	if err := db.Save(&existing).Error; err != nil {
		log.Fatalf("saving ruleset: %v", err)
	}

	log.Println("done.")
}
