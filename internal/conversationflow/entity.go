// Package conversationflow stores which core engines a live conversation runs
// after a survey is completed, and how the advisor presents itself. The
// conversation engine (apps/conversation-engine) reads the active flow over
// HTTP; nothing in core-engine executes a flow.
package conversationflow

import "time"

// Flow statuses are lowercase, like survey_definitions.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Engine names a flow step may use. The conversation engine maps each to a
// core-engine endpoint.
const (
	EngineScore            = "score"
	EngineMatch            = "match"
	EngineVision           = "vision"
	EngineFaceArchitecture = "face_architecture"
	EngineColour           = "colour"
)

// engineInputs lists, per engine, the inputs a survey answer does not give it.
// Colour's are the required form fields of POST /core/colour-engine/analyze.
// Match takes none: it resolves its routine blueprint by brand code and its
// strategy_id field is unused.
var engineInputs = map[string][]string{
	EngineScore:            nil,
	EngineMatch:            nil,
	EngineVision:           nil,
	EngineFaceArchitecture: nil,
	EngineColour:           {"hijab", "hairVisible"},
}

// Input supplies one engine input: from a boolean survey answer, or a fixed value.
type Input struct {
	FromAnswer string `json:"from_answer,omitempty"`
	Value      *bool  `json:"value,omitempty"`
}

// Step is one engine to run, in flow order.
type Step struct {
	Engine string           `json:"engine"`
	Inputs map[string]Input `json:"inputs,omitempty"`
}

// Persona is how the advisor presents itself. Voice empty means the
// conversation engine's CONV_VOICE_DEFAULT.
type Persona struct {
	Name  string `json:"name"`
	Voice string `json:"voice,omitempty"`
	Intro string `json:"intro,omitempty"`
}

// Flow is one row of conversation_flows (core database).
type Flow struct {
	ID            string    `json:"id" gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	BrandID       string    `json:"brand_id" gorm:"column:brand_id;type:varchar(64);uniqueIndex:idx_conversation_flow_version;not null"`
	ApplicationID string    `json:"application_id" gorm:"column:application_id;type:varchar(64);uniqueIndex:idx_conversation_flow_version;not null"`
	SurveyCode    string    `json:"survey_code" gorm:"column:survey_code;type:varchar(64);uniqueIndex:idx_conversation_flow_version;not null"`
	Status        string    `json:"status" gorm:"column:status;type:varchar(32);not null"`
	Version       int       `json:"version" gorm:"column:version;uniqueIndex:idx_conversation_flow_version;not null"`
	Locale        string    `json:"locale" gorm:"column:locale;type:varchar(16);not null"`
	Persona       Persona   `json:"persona" gorm:"column:persona;type:jsonb;serializer:json;not null"`
	Steps         []Step    `json:"steps" gorm:"column:steps;type:jsonb;serializer:json;not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at;type:timestamptz;autoCreateTime"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"column:updated_at;type:timestamptz;autoUpdateTime"`
}

func (Flow) TableName() string { return "conversation_flows" }
