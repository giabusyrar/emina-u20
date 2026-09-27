package entity

import (
	"strings"
	"time"
)

// SurveyStatusActive is the status a survey must have to be served and
// evaluated. Survey statuses are lowercase (draft | active | archived); rulesets
// use uppercase (score/entity.RulesetStatusActive).
const SurveyStatusActive = "active"

// NormalizeSurveyStatus returns status in the canonical survey form: trimmed
// and lowercase, defaulting to active. Saving "ACTIVE" used to store a survey
// that GetActive's exact-match lookup could never find.
func NormalizeSurveyStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return SurveyStatusActive
	}
	return status
}

// SurveyDefinition represents a full SurveyJS JSON questionnaire definition
type SurveyDefinition struct {
	UUID          string     `json:"uuid" gorm:"column:uuid;type:uuid;primaryKey;default:gen_random_uuid()"`
	BrandID       string     `json:"brandId" gorm:"column:brand_id;type:varchar(64);uniqueIndex:idx_survey_tenant;not null"`
	ApplicationID string     `json:"applicationId" gorm:"column:application_id;type:varchar(64);uniqueIndex:idx_survey_tenant;not null"`
	Code          string     `json:"code" gorm:"column:code;type:varchar(64);uniqueIndex:idx_survey_tenant;not null"`
	Title         string     `json:"title" gorm:"column:title;type:varchar(255);not null"`
	Status        string     `json:"status" gorm:"column:status;type:varchar(32);default:'active'"` // draft | active | archived
	Version       int        `json:"version" gorm:"column:version;uniqueIndex:idx_survey_tenant;default:1"`
	SchemaJSON    string     `json:"schema" gorm:"column:schema;type:jsonb;not null"`
	CreatedAt     time.Time  `json:"createdAt" gorm:"column:created_at;type:timestamptz;default:now()"`
	UpdatedAt     *time.Time `json:"updatedAt,omitempty" gorm:"column:updated_at;type:timestamptz"`
}

func (SurveyDefinition) TableName() string { return "survey_definitions" }
