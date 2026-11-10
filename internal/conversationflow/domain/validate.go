package domain

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/entity"
)

// engineInputs lists, per engine, the inputs a survey answer does not give it.
// Colour's are the required form fields of POST /core/colour-engine/analyze.
// Match takes none: it resolves its routine blueprint by brand code and its
// strategy_id field is unused.
var engineInputs = map[string][]string{
	entity.EngineScore:            nil,
	entity.EngineMatch:            nil,
	entity.EngineVision:           nil,
	entity.EngineFaceArchitecture: nil,
	entity.EngineColour:           {"hijab", "hairVisible"},
}

// QuestionTypes maps each question name of a survey to its SurveyJS type.
type QuestionTypes map[string]string

// Normalize trims identifiers and canonicalises status (lowercase, default
// draft) and version (default 1) before validation.
func Normalize(f *entity.Flow) {
	f.BrandID = strings.TrimSpace(f.BrandID)
	f.ApplicationID = strings.TrimSpace(f.ApplicationID)
	f.SurveyCode = strings.TrimSpace(f.SurveyCode)
	f.Locale = strings.TrimSpace(f.Locale)
	f.Status = strings.ToLower(strings.TrimSpace(f.Status))
	if f.Status == "" {
		f.Status = entity.StatusDraft
	}
	if f.Version <= 0 {
		f.Version = 1
	}
}

// Validate returns every reason f cannot be saved against a survey whose
// questions are questions. It does no I/O.
func Validate(f *entity.Flow, questions QuestionTypes) []string {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if f.BrandID == "" || f.ApplicationID == "" || f.SurveyCode == "" {
		add("brand_id, application_id and survey_code are required")
	}
	if !slices.Contains([]string{entity.StatusDraft, entity.StatusActive, entity.StatusArchived}, f.Status) {
		add("status must be draft, active or archived, got %q", f.Status)
	}
	if f.Locale == "" {
		add("locale is required")
	}
	if strings.TrimSpace(f.Persona.Name) == "" {
		add("persona.name is required")
	}
	if len(f.Steps) == 0 || f.Steps[0].Engine != entity.EngineScore {
		add("the first step must be %q", entity.EngineScore)
	}

	seen := map[string]bool{}
	for i, step := range f.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		wanted, known := engineInputs[step.Engine]
		if !known {
			add("%s: unknown engine %q", path, step.Engine)
			continue
		}
		if seen[step.Engine] {
			add("%s: engine %q is listed twice", path, step.Engine)
		}
		seen[step.Engine] = true

		names := make([]string, 0, len(step.Inputs))
		for name := range step.Inputs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if !slices.Contains(wanted, name) {
				add("%s: engine %q takes no input %q", path, step.Engine, name)
			}
		}
		for _, name := range wanted {
			in, ok := step.Inputs[name]
			if !ok {
				add("%s: input %q is required", path, name)
				continue
			}
			errs = append(errs, validateInput(fmt.Sprintf("%s.inputs.%s", path, name), in, questions)...)
		}
	}
	return errs
}

func validateInput(path string, in entity.Input, questions QuestionTypes) []string {
	hasAnswer, hasValue := in.FromAnswer != "", in.Value != nil
	if hasAnswer == hasValue {
		return []string{path + ": set exactly one of from_answer or value"}
	}
	if hasValue {
		return nil
	}
	qType, ok := questions[in.FromAnswer]
	if !ok {
		return []string{fmt.Sprintf("%s: question %q is not in the survey", path, in.FromAnswer)}
	}
	if qType != "boolean" {
		return []string{fmt.Sprintf("%s: question %q is %q, must be boolean", path, in.FromAnswer, qType)}
	}
	return nil
}
