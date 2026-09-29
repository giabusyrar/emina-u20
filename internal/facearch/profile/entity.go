// Package profile holds per-brand face-architecture profiles: the entity,
// the document inside it, validation, and lookup.
// Spec: docs/superpowers/specs/2026-09-29-face-architecture-engine-design.md,
// "Interpretation layer (core-engine)".
package profile

import (
	"strings"
	"time"
)

// Profile statuses. Uppercase, like score_rulesets.
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
)

// Wildcard matches any brand or application in a profile's scope.
const Wildcard = "*"

// NormalizeStatus returns status trimmed and uppercase, defaulting to ACTIVE
// (same rule as score/entity.NormalizeRulesetStatus).
func NormalizeStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return StatusActive
	}
	return status
}

// FaceArchitectureProfile is one version of a brand's interpretation policy.
// Table face_architecture_profiles is new and owned by core-engine
// (docs/SHARED-DATABASE.md): nothing in Seagull-gateway reads or migrates it.
// The unique index name is prefixed so it cannot collide with another
// table's index in the shared schema.
type FaceArchitectureProfile struct {
	ID            string     `gorm:"type:uuid;primaryKey" json:"id"`
	BrandID       string     `gorm:"type:varchar(64);not null;default:'*';uniqueIndex:uq_facearch_profile_version" json:"brandId"`
	ApplicationID string     `gorm:"type:varchar(64);not null;default:'*';uniqueIndex:uq_facearch_profile_version" json:"applicationId"`
	Code          string     `gorm:"type:varchar(64);not null;uniqueIndex:uq_facearch_profile_version" json:"code"`
	Title         string     `gorm:"type:varchar(255);not null" json:"title"`
	Status        string     `gorm:"type:varchar(32);not null;default:'ACTIVE';index" json:"status"`
	Version       int        `gorm:"type:int;not null;default:1;uniqueIndex:uq_facearch_profile_version" json:"version"`
	Document      string     `gorm:"type:jsonb;not null" json:"document"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     *time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (FaceArchitectureProfile) TableName() string { return "face_architecture_profiles" }
