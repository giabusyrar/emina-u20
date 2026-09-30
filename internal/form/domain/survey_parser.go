package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// bound is a theoretical aggregate-score range for one dimension.
type bound struct{ min, max float64 }

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// choiceScoreRange returns the smallest and largest score across a choice list.
func choiceScoreRange(choices []SurveyChoice) (float64, float64) {
	if len(choices) == 0 {
		return 0, 0
	}
	first := 0.0
	if choices[0].Score != nil {
		first = float64(*choices[0].Score)
	}
	mn, mx := first, first
	for _, c := range choices[1:] {
		s := 0.0
		if c.Score != nil {
			s = float64(*c.Score)
		}
		mn, mx = minf(mn, s), maxf(mx, s)
	}
	return mn, mx
}

// dimensionTheoreticalBounds walks every question in the schema and returns the
// theoretical [min,max] aggregate score per dimension, honouring the declared
// calculation method. The Score Engine uses these to normalise a raw score to
// 0-100: normalized = (raw - min) / (max - min) * 100.
func dimensionTheoreticalBounds(survey *SurveyModel) map[string]bound {
	perDim := map[string][]bound{} // per-question contribution ranges, grouped by dimension

	add := func(dim *string, mn, mx float64) {
		if dim == nil || *dim == "" {
			return
		}
		perDim[*dim] = append(perDim[*dim], bound{mn, mx})
	}

	for _, q := range survey.GetAllQuestions() {
		switch {
		case choiceTypeSingle[q.Type]:
			mn, mx := choiceScoreRange(q.Choices)
			add(q.Dimension, mn, mx)
			for _, c := range q.Choices {
				if c.Dimension != nil && *c.Dimension != "" {
					s := 0.0
					if c.Score != nil {
						s = float64(*c.Score)
					}
					add(c.Dimension, minf(0, s), maxf(0, s))
				}
			}

		case q.Type == "checkbox" || q.Type == "tagbox":
			// any subset selectable: sum of negatives .. sum of positives
			var lo, hi float64
			for _, c := range q.Choices {
				s := 0.0
				if c.Score != nil {
					s = float64(*c.Score)
				}
				if s < 0 {
					lo += s
				} else {
					hi += s
				}
			}
			add(q.Dimension, lo, hi)

		case q.Type == "ranking":
			// every item is always ranked: contribution is fixed
			var sum float64
			for _, c := range q.Choices {
				if c.Score != nil {
					sum += float64(*c.Score)
				}
			}
			add(q.Dimension, sum, sum)

		case q.Type == "boolean":
			sT, sF := 1.0, 0.0
			if q.ScoreTrue != nil {
				sT = float64(*q.ScoreTrue)
			}
			if q.ScoreFalse != nil {
				sF = float64(*q.ScoreFalse)
			}
			add(q.Dimension, minf(sT, sF), maxf(sT, sF))

		case q.Type == "matrix":
			mn, mx := choiceScoreRange(q.Columns)
			n := float64(len(q.Rows))
			if n == 0 {
				n = 1
			}
			add(q.Dimension, mn*n, mx*n)

		case q.Type == "rating" || q.Type == "slider" ||
			(q.Type == "text" && q.InputType == "number"):
			lo, hi := 0.0, 100.0
			if q.Scale != nil {
				lo, hi = float64(q.Scale.Min), float64(q.Scale.Max)
			}
			add(q.Dimension, lo, hi)
		}
	}

	out := map[string]bound{}
	for dim, ranges := range perDim {
		method := "sum"
		if m, ok := survey.CalculationMethods[dim]; ok && m != "" {
			method = strings.ToLower(m)
		}
		var b bound
		switch method {
		case "average", "mean", "boolean_or", "boolean_and",
			"highest", "max", "lowest", "min":
			// these never exceed the widest single-question contribution
			b = ranges[0]
			for _, r := range ranges[1:] {
				b.min, b.max = minf(b.min, r.min), maxf(b.max, r.max)
			}
		default: // sum
			for _, r := range ranges {
				b.min += r.min
				b.max += r.max
			}
		}
		out[dim] = b
	}
	return out
}

// ParseSurveySchema parses raw JSON into a SurveyModel
func ParseSurveySchema(rawJSON []byte) (*SurveyModel, error) {
	var model SurveyModel
	if err := json.Unmarshal(rawJSON, &model); err != nil {
		return nil, fmt.Errorf("failed to parse SurveyJS schema: %w", err)
	}
	return &model, nil
}

// choiceTypeMulti is the set of choice question types whose answer is a list.
var choiceTypeMulti = map[string]bool{"checkbox": true, "tagbox": true, "ranking": true}

// choiceTypeSingle is the set of choice question types whose answer is one value.
var choiceTypeSingle = map[string]bool{
	"radiogroup": true, "dropdown": true, "buttongroup": true, "imagepicker": true,
}

// isAnswerEmpty reports whether a submitted answer value counts as "not
// answered" for isRequired purposes — an absent key, nil, an empty string, or
// an empty slice (checkbox/tagbox/ranking with nothing selected).
func isAnswerEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return t == ""
	case []interface{}:
		return len(t) == 0
	default:
		return false
	}
}

// ValidateRequiredAnswers checks every question the schema marks isRequired
// has a non-empty answer in answers, collecting every violation rather than
// failing on the first one. Callers only run this when answers were actually
// submitted at all (data is optional on a vision-only evaluate call) — an
// empty/omitted data payload is not itself a validation failure here.
func ValidateRequiredAnswers(survey *SurveyModel, answers map[string]interface{}) *ValidationErrors {
	verrs := &ValidationErrors{}
	for _, q := range survey.GetAllQuestions() {
		if !q.IsRequired {
			continue
		}
		v, ok := answers[q.Name]
		if !ok || isAnswerEmpty(v) {
			verrs.Add(fmt.Sprintf("%s is required", q.Name))
		}
	}
	return verrs
}

// ExtractEvaluationData extracts AnswerList, DimensionRefs, and CustomerConditions from survey answers.
//
// It resolves per-answer scores per dimension and tags each DimensionRef with the
// aggregation method declared in the schema's calculation_methods map (default
// "sum"); the Score Module performs the actual aggregation. Supported question
// types: radiogroup, dropdown, buttongroup, imagepicker, checkbox, tagbox,
// ranking, boolean, rating, slider, and text(number).
func ExtractEvaluationData(survey *SurveyModel, answers map[string]interface{}) ([]AnswerEntry, []DimensionRef, map[string]bool, map[string]interface{}, error) {
	questions := survey.GetAllQuestions()
	questionsByName := make(map[string]SurveyQuestion, len(questions))
	for _, q := range questions {
		questionsByName[q.Name] = q
	}

	var answerList []AnswerEntry
	customerCondition := make(map[string]bool)
	dimensionAnswers := make(map[string][]int)
	// labels collects the selected text of every question that carries no
	// dimension binding at all (question or choice) — e.g. Q6's main concern,
	// or a lifestyle checklist not yet wired into any scored axis. These
	// still answer_list (for audit/debug), but are metadata, not a score
	// contributing to any dimension, so they're also surfaced here by
	// question name for a caller that wants them cleanly, not mixed in with
	// real scored answers.
	labels := make(map[string][]string)

	// record files one resolved score under its dimension (choice dimension wins
	// over question dimension), or — when the question/choice declares no
	// dimension at all — files its text as a label instead. Dimension score
	// bounds come from the schema as a whole (dimensionTheoreticalBounds),
	// not from what was answered.
	record := func(qDim, choiceDim *string, score, _, _ int, qName, text string) {
		dim := qDim
		if choiceDim != nil && *choiceDim != "" {
			dim = choiceDim
		}
		if dim == nil || *dim == "" {
			labels[qName] = append(labels[qName], text)
			return
		}
		dimensionAnswers[*dim] = append(dimensionAnswers[*dim], score)
	}

	scaleBounds := func(q SurveyQuestion) (int, int) {
		if q.Scale != nil {
			return q.Scale.Min, q.Scale.Max
		}
		return 0, 0
	}

	// resolveChoice applies one selected choice value to a choice-style question.
	resolveChoice := func(q SurveyQuestion, valStr string) {
		for i := range q.Choices {
			if q.Choices[i].Value != valStr {
				continue
			}
			ch := q.Choices[i]
			score := 0
			if ch.Score != nil {
				score = *ch.Score
			}
			minScore, maxScore := scaleBounds(q)
			answerList = append(answerList, AnswerEntry{
				Question: q.Name,
				Answer:   ch.Text,
				Score:    score,
				MinScore: minScore,
				MaxScore: maxScore,
			})
			for k, v := range ch.ConditionMap {
				customerCondition[k] = v
			}
			record(q.Dimension, ch.Dimension, score, minScore, maxScore, q.Name, ch.Text)
			return
		}
	}

	toStringSlice := func(v interface{}) []string {
		switch t := v.(type) {
		case []interface{}:
			out := make([]string, 0, len(t))
			for _, e := range t {
				out = append(out, fmt.Sprintf("%v", e))
			}
			return out
		case []string:
			return t
		default:
			return []string{fmt.Sprintf("%v", v)}
		}
	}

	toInt := func(v interface{}) (int, bool) {
		switch n := v.(type) {
		case float64:
			return int(n), true
		case int:
			return n, true
		case json.Number:
			f, err := n.Float64()
			if err != nil {
				return 0, false
			}
			return int(f), true
		default:
			return 0, false
		}
	}

	// Iterate questions in SCHEMA order, not answers map order — Go map
	// iteration is randomized, which was silently shuffling answer_list on
	// every single call.
	for _, q := range questions {
		ansVal, ok := answers[q.Name]
		if !ok {
			continue // this question wasn't answered
		}

		switch {
		case choiceTypeSingle[q.Type]:
			resolveChoice(q, fmt.Sprintf("%v", ansVal))

		case choiceTypeMulti[q.Type]:
			// checkbox / tagbox / ranking: score every selected item (order is ignored).
			for _, valStr := range toStringSlice(ansVal) {
				resolveChoice(q, valStr)
			}

		case q.Type == "boolean":
			b, _ := ansVal.(bool)
			score := 0
			if b {
				if q.ScoreTrue != nil {
					score = *q.ScoreTrue
				} else {
					score = 1
				}
			} else if q.ScoreFalse != nil {
				score = *q.ScoreFalse
			}
			minScore, maxScore := scaleBounds(q)
			if maxScore == 0 {
				maxScore = 1
			}
			answerList = append(answerList, AnswerEntry{
				Question: q.Name,
				Answer:   fmt.Sprintf("%t", b),
				Score:    score,
				MinScore: minScore,
				MaxScore: maxScore,
			})
			record(q.Dimension, nil, score, minScore, maxScore, q.Name, fmt.Sprintf("%t", b))

		case q.Type == "matrix":
			// answer is { rowValue: columnValue }; each answered row adds its
			// picked column's score to the question's dimension.
			rowMap, ok := ansVal.(map[string]interface{})
			if !ok {
				continue
			}
			minScore, maxScore := scaleBounds(q)
			for _, colVal := range rowMap {
				colStr := fmt.Sprintf("%v", colVal)
				for i := range q.Columns {
					if q.Columns[i].Value != colStr {
						continue
					}
					score := 0
					if q.Columns[i].Score != nil {
						score = *q.Columns[i].Score
					}
					answerList = append(answerList, AnswerEntry{
						Question: q.Name,
						Answer:   q.Columns[i].Text,
						Score:    score,
						MinScore: minScore,
						MaxScore: maxScore,
					})
					record(q.Dimension, q.Columns[i].Dimension, score, minScore, maxScore, q.Name, q.Columns[i].Text)
					break
				}
			}

		case q.Type == "rating" || q.Type == "slider" ||
			(q.Type == "text" && q.InputType == "number"):
			score, ok := toInt(ansVal)
			if !ok {
				continue
			}
			minScore, maxScore := scaleBounds(q)
			answerList = append(answerList, AnswerEntry{
				Question: q.Name,
				Answer:   fmt.Sprintf("%d", score),
				Score:    score,
				MinScore: minScore,
				MaxScore: maxScore,
			})
			record(q.Dimension, nil, score, minScore, maxScore, q.Name, fmt.Sprintf("%d", score))
		}
	}

	bounds := dimensionTheoreticalBounds(survey)
	var dimensionRefs []DimensionRef
	for dimKey, scores := range dimensionAnswers {
		method := "sum"
		if m, ok := survey.CalculationMethods[dimKey]; ok && m != "" {
			method = m
		}
		b, ok := bounds[dimKey]
		if !ok || b.max <= b.min {
			// schema yields no usable range: fall back to a 0-100 assumption
			b = bound{min: 0, max: 100}
		}
		dimensionRefs = append(dimensionRefs, DimensionRef{
			Key:               dimKey,
			MinScore:          int(math.Round(b.min)),
			MaxScore:          int(math.Round(b.max)),
			CalculationMethod: method,
			Answers:           scores,
		})
	}

	// A label is a plain string for a question that can only ever have one
	// selected answer (radiogroup, dropdown, boolean, a single number, ...)
	// — e.g. Q6's main concern is one string, not a one-element array. It
	// stays a list only for a question type that can genuinely carry more
	// than one value (checkbox, tagbox, ranking, matrix).
	out := make(map[string]interface{}, len(labels))
	for qName, values := range labels {
		q := questionsByName[qName]
		if choiceTypeMulti[q.Type] || q.Type == "matrix" {
			out[qName] = values
		} else if len(values) > 0 {
			out[qName] = values[0]
		}
	}

	return answerList, dimensionRefs, customerCondition, out, nil
}
