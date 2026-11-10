package entity

import (
	"strings"
	"time"
)

// RulesetStatusActive is the status a ruleset must have to score. Ruleset
// statuses are uppercase; surveys use lowercase (form/entity.SurveyStatusActive).
const RulesetStatusActive = "ACTIVE"

// NormalizeRulesetStatus returns status in the canonical ruleset form: trimmed
// and uppercase, defaulting to ACTIVE. Saving "active" used to store a ruleset
// the exact-match active lookup could never find.
func NormalizeRulesetStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return RulesetStatusActive
	}
	return status
}

// ScoreRuleset represents a versioned JDM Decision Model stored in PostgreSQL
type ScoreRuleset struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BrandID       string     `gorm:"type:varchar(64);not null;default:'*';index:idx_ruleset_tenant" json:"brandId"`
	ApplicationID string     `gorm:"type:varchar(64);not null;default:'*';index:idx_ruleset_tenant" json:"applicationId"`
	Code          string     `gorm:"type:varchar(64);not null;index:idx_ruleset_tenant" json:"code"`
	Title         string     `gorm:"type:varchar(255);not null" json:"title"`
	Description   string     `gorm:"type:text" json:"description"`
	Status        string     `gorm:"type:varchar(32);not null;default:'ACTIVE';index" json:"status"`
	Version       int        `gorm:"type:int;not null;default:1;index:idx_ruleset_tenant" json:"version"`
	Schema        string     `gorm:"type:jsonb;not null" json:"schema"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     *time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (ScoreRuleset) TableName() string {
	return "score_rulesets"
}
