package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// jakarta is loaded once; a missing tzdata falls back to a fixed +07:00 so
// age calculation never breaks in a minimal container that lacks the IANA
// database.
var jakarta = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// AgeOverThirty reports whether dob (YYYY-MM-DD) is more than 30 years
// before now, evaluated in Asia/Jakarta so a birthday doesn't shift by a day
// depending on the server's own timezone. Someone turning exactly 30 today
// is NOT over thirty (the spec's Aging axis is "<=30 -> T").
func AgeOverThirty(dob string, now time.Time) (bool, error) {
	birth, err := time.ParseInLocation("2006-01-02", dob, jakarta)
	if err != nil {
		return false, err
	}
	today := now.In(jakarta)

	age := today.Year() - birth.Year()
	hadBirthdayThisYear := today.Month() > birth.Month() ||
		(today.Month() == birth.Month() && today.Day() >= birth.Day())
	if !hadBirthdayThisYear {
		age--
	}
	return age > 30, nil
}

// fieldMapping mirrors the ruleset schema's field_mapping — which questions/
// vendor fields feed each dimension's form/vision side — compiled by the
// Skin Grading Studio's Blending tab. Only the vision side matters here.
type fieldMapping struct {
	Form   string `json:"form"`
	Vision string `json:"vision"`
}

// RulesetVisionSignals reads field_mapping out of a ruleset's raw JDM schema
// and, for every dimension whose vision side names a vendor field also
// present in vendorFields (already flattened by leaf key, health-space, high
// = healthy), returns the concern-space (0 = healthy) value to send as that
// dimension's vision_signals entry — inverted, because /evaluate's fusion
// happens in concern space before the one health-space flip at the end.
func RulesetVisionSignals(schema string, vendorFields map[string]float64) map[string]float64 {
	var parsed struct {
		FieldMapping map[string]fieldMapping `json:"field_mapping"`
	}
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		return map[string]float64{}
	}

	out := make(map[string]float64)
	for dimKey, fm := range parsed.FieldMapping {
		if fm.Vision == "" {
			continue
		}
		healthVal, ok := vendorFields[fm.Vision]
		if !ok {
			continue
		}
		concern := 100.0 - healthVal
		if concern < 0 {
			concern = 0
		}
		if concern > 100 {
			concern = 100
		}
		out[dimKey] = concern
	}
	return out
}

// RulesetSchemaWithCode pairs a ruleset's code with its raw schema — just
// enough for ResolveRulesetCode to match against, without depending on the
// Score module's own entity/client types.
type RulesetSchemaWithCode struct {
	Code   string
	Status string
	Schema string
}

// rulesetStatusActive mirrors score/entity.RulesetStatusActive; the Form
// module does not import the Score module's packages. Compared
// case-insensitively, since a ruleset saved as "active" is active.
const rulesetStatusActive = "ACTIVE"

// ResolveRulesetCode finds the active ruleset among candidates whose schema
// declares form_survey_code == surveyCode (set on the ruleset's own Setup
// tab) and returns its code. Falls back to surveyCode unchanged when none
// match, so a setup that never linked a ruleset to a survey — or still
// relies on both sharing the same code by convention — keeps working exactly
// as before.
func ResolveRulesetCode(candidates []RulesetSchemaWithCode, surveyCode string) string {
	for _, rs := range candidates {
		if rs.Status != "" && !strings.EqualFold(rs.Status, rulesetStatusActive) {
			continue
		}
		var meta struct {
			FormSurveyCode string `json:"form_survey_code"`
		}
		if json.Unmarshal([]byte(rs.Schema), &meta) == nil && meta.FormSurveyCode == surveyCode {
			return rs.Code
		}
	}
	return surveyCode
}

// RulesetFormDimensionKeys returns every dimension key whose field_mapping
// declares a form side equal to formFieldCode (e.g. "age_over_30") — the
// caller uses this to know which dimension(s), if any, need a
// ComputedDimensions entry synthesized from something other than a Q1-Q6
// answer (there is normally exactly one: Aging).
func RulesetFormDimensionKeys(schema, formFieldCode string) []string {
	var parsed struct {
		FieldMapping map[string]fieldMapping `json:"field_mapping"`
	}
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		return nil
	}
	var keys []string
	for dimKey, fm := range parsed.FieldMapping {
		if fm.Form == formFieldCode {
			keys = append(keys, dimKey)
		}
	}
	return keys
}
