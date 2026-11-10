// Package entity holds the customer assessment model.
package entity

import "time"

// Record is one stored evaluation. JSON columns hold the evaluation's own
// output verbatim; a column is NULL when the evaluation did not produce it.
type Record struct {
	ID                  string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CustomerID          string     `gorm:"column:customer_id;type:varchar(100);not null;index:idx_cust_assess_cust" json:"customerId"`
	BrandID             string     `gorm:"column:brand_id;type:varchar(64);not null;index:idx_cust_assess_brand" json:"brandId"`
	ApplicationID       string     `gorm:"column:application_id;type:varchar(64);not null" json:"applicationId"`
	FormID              string     `gorm:"column:form_id;type:varchar(100)" json:"formId"`
	TotalScore          float64    `gorm:"column:total_score;type:numeric" json:"totalScore"`
	SkinProfileCode     string     `gorm:"column:skin_profile_code;type:varchar(50)" json:"skinProfileCode"`
	SkinProfile         *string    `gorm:"column:skin_profile;type:jsonb" json:"skinProfile"`
	SkinGradingTiers    *string    `gorm:"column:skin_grading_tiers;type:jsonb" json:"skinGradingTiers"`
	DimensionScores     string     `gorm:"column:dimension_scores;type:jsonb;not null" json:"dimensionScores"`
	RecommendedProducts *string    `gorm:"column:recommended_products;type:jsonb" json:"recommendedProducts"`
	RawAnswers          *string    `gorm:"column:raw_answers;type:jsonb" json:"rawAnswers,omitempty"`
	VisionMetrics       *string    `gorm:"column:vision_metrics;type:jsonb" json:"visionMetrics,omitempty"`
	ExecutionTimeMs     int64      `gorm:"column:execution_time_ms" json:"executionTimeMs"`
	CreatedAt           time.Time  `gorm:"column:created_at;type:timestamptz;autoCreateTime" json:"createdAt"`
	UpdatedAt           *time.Time `gorm:"column:updated_at;type:timestamptz" json:"updatedAt"`
}

func (Record) TableName() string { return "customer_assessments" }
