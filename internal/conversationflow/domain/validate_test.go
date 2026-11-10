package domain

import (
	"strings"
	"testing"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/conversationflow/entity"
)

var questions = QuestionTypes{"q1": "radiogroup", "wears_hijab": "boolean"}

func validFlow() *entity.Flow {
	no := false
	return &entity.Flow{
		BrandID: "wardah", ApplicationID: "skinverse", SurveyCode: "skinverse_q1_q6",
		Status: entity.StatusActive, Version: 1, Locale: "id",
		Persona: entity.Persona{Name: "Kak Gia"},
		Steps: []entity.Step{
			{Engine: entity.EngineScore},
			{Engine: entity.EngineMatch},
			{Engine: entity.EngineColour, Inputs: map[string]entity.Input{
				"hijab":       {FromAnswer: "wears_hijab"},
				"hairVisible": {Value: &no},
			}},
		},
	}
}

func anyContains(errs []string, want string) bool {
	for _, e := range errs {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

func TestValidateAcceptsAValidFlow(t *testing.T) {
	if errs := Validate(validFlow(), questions); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateRejects(t *testing.T) {
	yes := true
	cases := map[string]struct {
		mutate func(*entity.Flow)
		want   string
	}{
		"missing ids":          {func(f *entity.Flow) { f.SurveyCode = "" }, "brand_id, application_id and survey_code are required"},
		"bad status":           {func(f *entity.Flow) { f.Status = "live" }, "status must be draft, active or archived"},
		"no locale":            {func(f *entity.Flow) { f.Locale = "" }, "locale is required"},
		"no persona name":      {func(f *entity.Flow) { f.Persona.Name = " " }, "persona.name is required"},
		"score not first":      {func(f *entity.Flow) { f.Steps = f.Steps[1:] }, `the first step must be "score"`},
		"unknown engine":       {func(f *entity.Flow) { f.Steps = append(f.Steps, entity.Step{Engine: "tarot"}) }, `unknown engine "tarot"`},
		"listed twice":         {func(f *entity.Flow) { f.Steps = append(f.Steps, entity.Step{Engine: entity.EngineMatch}) }, `engine "match" is listed twice`},
		"unexpected input":     {func(f *entity.Flow) { f.Steps[1].Inputs = map[string]entity.Input{"x": {}} }, `engine "match" takes no input "x"`},
		"colour input missing": {func(f *entity.Flow) { delete(f.Steps[2].Inputs, "hairVisible") }, `input "hairVisible" is required`},
		"input both ways": {func(f *entity.Flow) {
			f.Steps[2].Inputs["hijab"] = entity.Input{FromAnswer: "wears_hijab", Value: &yes}
		}, "set exactly one of from_answer or value"},
		"input neither way":    {func(f *entity.Flow) { f.Steps[2].Inputs["hijab"] = entity.Input{} }, "set exactly one of from_answer or value"},
		"answer not in survey": {func(f *entity.Flow) { f.Steps[2].Inputs["hijab"] = entity.Input{FromAnswer: "nope"} }, `question "nope" is not in the survey`},
		"answer not boolean":   {func(f *entity.Flow) { f.Steps[2].Inputs["hijab"] = entity.Input{FromAnswer: "q1"} }, `question "q1" is "radiogroup", must be boolean`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := validFlow()
			tc.mutate(f)
			if errs := Validate(f, questions); !anyContains(errs, tc.want) {
				t.Fatalf("errors %v do not contain %q", errs, tc.want)
			}
		})
	}
}

func TestNormalizeTrimsAndDefaults(t *testing.T) {
	f := &entity.Flow{BrandID: " wardah ", ApplicationID: " skinverse", SurveyCode: "s ", Locale: " id ", Status: " ACTIVE "}
	Normalize(f)
	if f.BrandID != "wardah" || f.ApplicationID != "skinverse" || f.SurveyCode != "s" || f.Locale != "id" {
		t.Fatalf("not trimmed: %+v", f)
	}
	if f.Status != entity.StatusActive || f.Version != 1 {
		t.Fatalf("status/version = %q/%d", f.Status, f.Version)
	}
	g := &entity.Flow{}
	Normalize(g)
	if g.Status != entity.StatusDraft {
		t.Fatalf("empty status = %q, want draft", g.Status)
	}
}
