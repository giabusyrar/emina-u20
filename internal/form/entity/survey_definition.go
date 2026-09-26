package entity

import "time"

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
