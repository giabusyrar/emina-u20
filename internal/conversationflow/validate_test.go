package conversationflow

import (
	"strings"
	"testing"
)

var questions = QuestionTypes{"q1": "radiogroup", "wears_hijab": "boolean"}

func validFlow() *Flow {
	no := false
	return &Flow{
		BrandID: "wardah", ApplicationID: "skinverse", SurveyCode: "skinverse_q1_q6",
		Status: StatusActive, Version: 1, Locale: "id",
		Persona: Persona{Name: "Kak Gia"},
		Steps: []Step{
			{Engine: EngineScore},
			{Engine: EngineMatch},
			{Engine: EngineColour, Inputs: map[string]Input{
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
		mutate func(*Flow)
		want   string
	}{
		"missing ids":         {func(f *Flow) { f.SurveyCode = "" }, "brand_id, application_id and survey_code are required"},
		"bad status":          {func(f *Flow) { f.Status = "live" }, "status must be draft, active or archived"},
		"no locale":           {func(f *Flow) { f.Locale = "" }, "locale is required"},
		"no persona name":     {func(f *Flow) { f.Persona.Name = " " }, "persona.name is required"},
		"score not first":     {func(f *Flow) { f.Steps = f.Steps[1:] }, `the first step must be "score"`},
		"unknown engine":      {func(f *Flow) { f.Steps = append(f.Steps, Step{Engine: "tarot"}) }, `unknown engine "tarot"`},
		"listed twice":        {func(f *Flow) { f.Steps = append(f.Steps, Step{Engine: EngineMatch}) }, `engine "match" is listed twice`},
		"unexpected input":    {func(f *Flow) { f.Steps[1].Inputs = map[string]Input{"x": {}} }, `engine "match" takes no input "x"`},
		"colour input missing": {func(f *Flow) { delete(f.Steps[2].Inputs, "hairVisible") }, `input "hairVisible" is required`},
		"input both ways": {func(f *Flow) {
			f.Steps[2].Inputs["hijab"] = Input{FromAnswer: "wears_hijab", Value: &yes}
		}, "set exactly one of from_answer or value"},
		"input neither way":     {func(f *Flow) { f.Steps[2].Inputs["hijab"] = Input{} }, "set exactly one of from_answer or value"},
		"answer not in survey":  {func(f *Flow) { f.Steps[2].Inputs["hijab"] = Input{FromAnswer: "nope"} }, `question "nope" is not in the survey`},
		"answer not boolean":    {func(f *Flow) { f.Steps[2].Inputs["hijab"] = Input{FromAnswer: "q1"} }, `question "q1" is "radiogroup", must be boolean`},
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
	f := &Flow{BrandID: " wardah ", ApplicationID: " skinverse", SurveyCode: "s ", Locale: " id ", Status: " ACTIVE "}
	Normalize(f)
	if f.BrandID != "wardah" || f.ApplicationID != "skinverse" || f.SurveyCode != "s" || f.Locale != "id" {
		t.Fatalf("not trimmed: %+v", f)
	}
	if f.Status != StatusActive || f.Version != 1 {
		t.Fatalf("status/version = %q/%d", f.Status, f.Version)
	}
	g := &Flow{}
	Normalize(g)
	if g.Status != StatusDraft {
		t.Fatalf("empty status = %q, want draft", g.Status)
	}
}
