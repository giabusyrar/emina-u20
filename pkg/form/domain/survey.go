package domain

// SurveyScale bounds numerical inputs or choices
type SurveyScale struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// SurveyValidator holds client-side or server-side validation rules
type SurveyValidator struct {
	Type     string   `json:"type"` // e.g. "numeric", "regex", "text"
	MinValue *float64 `json:"minValue,omitempty"`
	MaxValue *float64 `json:"maxValue,omitempty"`
	Text     string   `json:"text,omitempty"`
}

// SurveyChoice represents a single option in radiogroup, checkbox, or dropdown
type SurveyChoice struct {
	Value        string          `json:"value"`
	Text         string          `json:"text"`
	Score        *int            `json:"score,omitempty"`
	Dimension    *string         `json:"dimension,omitempty"`
	ConditionMap map[string]bool `json:"condition_map,omitempty"`
}

// SurveyQuestion represents a single SurveyJS question element
type SurveyQuestion struct {
	Type       string            `json:"type"`                // radiogroup, checkbox, dropdown, tagbox, buttongroup, imagepicker, boolean, rating, slider, ranking, text
	InputType  string            `json:"inputType,omitempty"` // for type "text": number, email, date
	Name       string            `json:"name"`                // unique field key
	Title      string            `json:"title"`
	VisibleIf  string            `json:"visibleIf,omitempty"`
	Dimension  *string           `json:"dimension,omitempty"`
	Scale      *SurveyScale      `json:"scale,omitempty"`
	Choices    []SurveyChoice    `json:"choices,omitempty"`
	Validators []SurveyValidator `json:"validators,omitempty"`

	// matrix: rows scored against the shared, scored columns.
	Columns []SurveyChoice `json:"columns,omitempty"`
	Rows    []SurveyChoice `json:"rows,omitempty"`

	// boolean: score contribution for each state (defaults 1 / 0).
	ScoreTrue  *int `json:"scoreTrue,omitempty"`
	ScoreFalse *int `json:"scoreFalse,omitempty"`
}

// SurveyPage represents a page containing survey question elements
type SurveyPage struct {
	Name      string           `json:"name"`
	Title     string           `json:"title,omitempty"`
	VisibleIf string           `json:"visibleIf,omitempty"`
	Elements  []SurveyQuestion `json:"elements"`
}

// SurveyModel represents the full SurveyJS schema model
type SurveyModel struct {
	Title         string           `json:"title"`
	BrandID       string           `json:"brand_id,omitempty"`
	ApplicationID string           `json:"application_id,omitempty"`
	Code          string           `json:"code,omitempty"`
	Pages         []SurveyPage     `json:"pages,omitempty"`
	Elements      []SurveyQuestion `json:"elements,omitempty"` // Fallback for single-page survey

	// CalculationMethods maps a dimension key to how its per-answer scores are
	// aggregated downstream (sum | average | max | min | boolean_or | boolean_and).
	// Absent / unknown keys fall back to "sum". XG extension; SurveyJS ignores it.
	CalculationMethods map[string]string `json:"calculation_methods,omitempty"`
}

// GetAllQuestions flattens questions across all pages or elements
func (s *SurveyModel) GetAllQuestions() []SurveyQuestion {
	var questions []SurveyQuestion
	for _, page := range s.Pages {
		questions = append(questions, page.Elements...)
	}
	if len(questions) == 0 && len(s.Elements) > 0 {
		questions = append(questions, s.Elements...)
	}
	return questions
}

// SurveyEvaluateRequest is the payload submitted by brand applications
type SurveyEvaluateRequest struct {
	BrandID       string `json:"brand_id"`
	ApplicationID string `json:"application_id"`
	// CustomerID is the opaque identifier of the person answering, supplied by
	// the integrating client (e.g. Wardah / Goers). Stored verbatim, not validated
	// for format. Required on submit so every answer set is attributable.
	CustomerID         string                 `json:"customer_id"`
	Data               map[string]interface{} `json:"data"`
	CustomerConditions map[string]bool        `json:"customer_conditions,omitempty"`
	VisionSignals      map[string]float64     `json:"vision_signals,omitempty"`
	// ComputedDimensions are dimension rollups this questionnaire's own
	// questions never produce (e.g. Aging/W-T, derived from date_of_birth,
	// not from any Q1-Q6 answer) but that the ruleset still needs a form-side
	// value for so fusion with vision_signals blends per its configured
	// weights instead of silently falling back to 100% vision. Appended to
	// the dimensions ExtractEvaluationData derives from Data.
	ComputedDimensions []DimensionRef `json:"computed_dimensions,omitempty"`
}
