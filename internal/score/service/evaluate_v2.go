package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/domain"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/entity"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/score/jdm"

	formdomain "gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/form/domain"
)

func round1(x float64) float64 { return math.Round(x*10.0) / 10.0 }

// syntheticDOBFromAge builds a YYYY-MM-DD for someone turning age years old
// today (same month/day as now), so AgeOverThirty — which only accepts a
// DOB, not a bare integer age — can be reused verbatim for a Simulator
// slider that only has an age, not a real date of birth.
func syntheticDOBFromAge(age int, now time.Time) string {
	return now.AddDate(-age, 0, 0).Format("2006-01-02")
}

func clamp100(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 100 {
		return 100
	}
	return x
}

// aggregateAndNormalizeConcern mirrors the legacy Evaluate's "Process Input
// Dimensions" step exactly (same aggregation switch, same min-max formula) —
// raw per-answer scores in, a 0-100 CONCERN figure out (0 = best answer).
func aggregateAndNormalizeConcern(dim formdomain.DimensionRef) float64 {
	rawTotal := 0
	for _, sc := range dim.Answers {
		rawTotal += sc
	}
	minScore := float64(dim.MinScore)
	maxScore := float64(dim.MaxScore)
	if minScore == 0 && maxScore == 0 {
		maxScore = 100
	}
	calcMethod := dim.CalculationMethod
	if calcMethod == "" {
		calcMethod = dim.Resolver
	}

	var agg float64
	switch strings.ToLower(calcMethod) {
	case "average", "mean":
		if len(dim.Answers) > 0 {
			agg = float64(rawTotal) / float64(len(dim.Answers))
		}
	case "highest", "max":
		for i, sc := range dim.Answers {
			if i == 0 || float64(sc) > agg {
				agg = float64(sc)
			}
		}
	case "lowest", "min":
		for i, sc := range dim.Answers {
			if i == 0 || float64(sc) < agg {
				agg = float64(sc)
			}
		}
	case "boolean_or":
		agg = minScore
		for _, sc := range dim.Answers {
			if sc > 0 {
				agg = maxScore
				break
			}
		}
	case "boolean_and":
		agg = maxScore
		if len(dim.Answers) == 0 {
			agg = minScore
		}
		for _, sc := range dim.Answers {
			if sc <= 0 {
				agg = minScore
				break
			}
		}
	default: // "sum"
		agg = float64(rawTotal)
	}

	normalized := agg
	if maxScore > minScore {
		normalized = (agg - minScore) / (maxScore - minScore) * 100.0
	}
	return clamp100(normalized)
}

// rulesetMeta is every schema field stage2/EvaluateV2 read out of a ruleset,
// gathered in one Unmarshal instead of several scattered ones.
type rulesetMeta struct {
	DimensionWeights map[string]float64      `json:"dimension_weights"`
	DimensionFusion  map[string]fusionWeight `json:"dimension_fusion"`
	AxisCodes        map[string]axisCode     `json:"axis_codes"`
	FieldMapping     map[string]struct {
		Form   string `json:"form"`
		Vision string `json:"vision"`
	} `json:"field_mapping"`
	FormSurveyCode   string `json:"form_survey_code"`
	VisionSourceCode string `json:"vision_source_code"`
}

func parseRulesetMeta(schema string) rulesetMeta {
	var m rulesetMeta
	_ = json.Unmarshal([]byte(schema), &m)
	return m
}

// stage2Input is the fully-prepared per-dimension CONCERN-space signal (0 =
// healthy, 100 = worst) — form and vision are tracked separately, per key,
// so the response can report exactly where each dimension's number came
// from. Either map may simply not have a key; stage2Score treats that as
// "this side has no data for this dimension", not zero.
type stage2Input struct {
	Ruleset           *entity.ScoreRuleset
	FormConcern       map[string]float64
	VisionConcern     map[string]float64
	CustomerCondition map[string]bool
}

type stage2Output struct {
	TotalScore        float64
	Dimensions        map[string]domain.DimensionBreakdown
	SkinProfile       domain.SkinProfileV2
	SubClassification map[string]interface{}
	AppliedRules      []string
	Warnings          []string
}

// stage2Score is the one function both the real evaluate endpoint and the
// ruleset Simulator run: fusion -> health flip -> axis letters -> JDM ->
// response. Evaluate reaches it via stage 1 (raw answers/photo -> concern
// scores); Simulate reaches it directly with its slider values converted to
// concern space beforehand (mathematically identical either order, since
// fusion weights always sum to 1).
func (s *ScoreService) stage2Score(ctx context.Context, in stage2Input) (*stage2Output, error) {
	ruleset := in.Ruleset
	meta := parseRulesetMeta(ruleset.Schema)

	runner, err := jdm.NewRunner(ruleset.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to compile JDM ruleset schema (%s): %w", ruleset.Code, err)
	}

	// Union of every dimension this ruleset registers (field_mapping) plus
	// any extra key a caller happened to send — a registered dimension with
	// no data at all this call still appears, as "source": "none".
	allKeys := map[string]bool{}
	for k := range meta.FieldMapping {
		allKeys[k] = true
	}
	for k := range in.FormConcern {
		allKeys[k] = true
	}
	for k := range in.VisionConcern {
		allKeys[k] = true
	}

	dimensions := make(map[string]domain.DimensionBreakdown, len(allKeys))
	healthScores := make(map[string]float64, len(allKeys))
	warnings := []string{}

	for key := range allKeys {
		fVal, hasForm := in.FormConcern[key]
		vVal, hasVision := in.VisionConcern[key]

		if !hasForm && !hasVision {
			dimensions[key] = domain.DimensionBreakdown{Source: "none"}
			warnings = append(warnings, fmt.Sprintf("%s: no form or vision data available this call", key))
			continue
		}

		var source string
		var weight *domain.DimensionWeight
		var concern float64
		switch {
		case hasForm && hasVision:
			source = "blend"
			fw := domain.DimensionWeight{FormWeight: 0.5, VisionWeight: 0.5}
			if dw, ok := meta.DimensionFusion[strings.ToLower(key)]; ok {
				fw = domain.DimensionWeight{FormWeight: dw.Form, VisionWeight: dw.Vision}
			}
			concern = fVal*fw.FormWeight + vVal*fw.VisionWeight
			weight = &fw
		case hasForm:
			source = "form"
			concern = fVal
		default:
			source = "vision"
			concern = vVal
		}
		concern = round1(clamp100(concern))
		health := round1(100.0 - concern)
		healthScores[key] = health

		var axisPtr *string
		if ac, ok := meta.AxisCodes[strings.ToLower(key)]; ok && (ac.Low != "" || ac.High != "") {
			letter := ac.Low
			if health >= ac.Threshold {
				letter = ac.High
			}
			axisPtr = &letter
		}

		var formPtr, visionPtr *float64
		if hasForm {
			v := round1(100.0 - fVal)
			formPtr = &v
		}
		if hasVision {
			v := round1(100.0 - vVal)
			visionPtr = &v
		}
		finalScore := health

		dimensions[key] = domain.DimensionBreakdown{
			Source:      source,
			FormScore:   formPtr,
			VisionScore: visionPtr,
			Weight:      weight,
			FinalScore:  &finalScore,
			Axis:        axisPtr,
		}
	}

	// total_score: weighted mean over ONLY axis_codes-registered dimensions
	// with actual data — pore_severity (or any other non-axis input) never
	// dilutes it, whether or not it happened to have data this call.
	var weightedSum, weightTotal float64
	for key, health := range healthScores {
		if _, isAxis := meta.AxisCodes[strings.ToLower(key)]; !isAxis {
			continue
		}
		w := 1.0
		if mw, ok := meta.DimensionWeights[strings.ToLower(key)]; ok && mw > 0 {
			w = mw
		}
		weightedSum += health * w
		weightTotal += w
	}
	totalScore := 0.0
	if weightTotal > 0 {
		totalScore = round1(weightedSum / weightTotal)
	}

	axisValuesForJDM := make(map[string]interface{}, len(dimensions))
	for key, b := range dimensions {
		if b.Axis != nil {
			axisValuesForJDM[strings.ToUpper(key)] = *b.Axis
		}
	}

	evalContext := map[string]interface{}{
		"dimension_scores":   healthScores,
		"customer_condition": in.CustomerCondition,
		"total_score":        totalScore,
		"axis_values":        axisValuesForJDM,
	}
	if isLegacyPerDimensionSchema(ruleset.Schema) {
		delete(evalContext, "axis_values")
	}

	evalResult, err := runner.Evaluate(evalContext)
	if err != nil {
		return nil, fmt.Errorf("JDM evaluation failed: %w", err)
	}

	appliedRules := []string{fmt.Sprintf("GoRules JDM Model: %s (v%d)", ruleset.Code, ruleset.Version)}

	var subClassification map[string]interface{}
	if sc, ok := evalResult["sub_classification"].(map[string]interface{}); ok {
		subClassification = sc
	}

	// pore_severity has a real health score but the pore-band rule table
	// deliberately only covers [0..10]/[40..60]/[90..100] — the guideline
	// (4. Skin Diagnosis: Grade 0 = Smooth, 1-2 = Visible, 3-5 = Enlarged)
	// will replace these once Paradev confirms converting its output to
	// that 0-5 grade. Until then, a value landing in the 11-39 or 61-89 gap
	// must surface as an explicit null + warning, not silently vanish from
	// sub_classification as if it were never computed at all.
	if pore, ok := dimensions["pore_severity"]; ok && pore.Source != "none" {
		if subClassification == nil {
			subClassification = map[string]interface{}{}
		}
		if _, has := subClassification["pore_severity"]; !has {
			subClassification["pore_severity"] = nil
			warnings = append(warnings, fmt.Sprintf(
				"PORE_CUTOFF_PENDING: pore_severity health score %.1f falls outside the confirmed pore-band ranges (no official cutoff yet for 11-39/61-89)",
				*pore.FinalScore,
			))
		}
	}

	skinProfile := composeSkinProfile(evalResult, axisValuesForJDM, meta.AxisCodes, dimensions, ruleset)

	return &stage2Output{
		TotalScore:        totalScore,
		Dimensions:        dimensions,
		SkinProfile:       skinProfile,
		SubClassification: subClassification,
		AppliedRules:      appliedRules,
		Warnings:          warnings,
	}, nil
}

// composeSkinProfile builds the identity code the same way the legacy
// Evaluate did (canonical axis order, JDM match cleared if it doesn't agree
// with the composed code), plus Complete: false whenever at least one of the
// ruleset's registered Baumann axes has no letter this call.
func composeSkinProfile(
	evalResult map[string]interface{},
	axisValues map[string]interface{},
	axisCodes map[string]axisCode,
	dimensions map[string]domain.DimensionBreakdown,
	ruleset *entity.ScoreRuleset,
) domain.SkinProfileV2 {
	profileCode, profileName, profileCategory, profileDescription := "", "", "", ""
	if sp, ok := evalResult["skin_profile"].(map[string]interface{}); ok {
		if c, exists := sp["code"].(string); exists {
			profileCode = c
		}
		if n, exists := sp["name"].(string); exists {
			profileName = n
		}
		if cat, exists := sp["category"].(string); exists {
			profileCategory = cat
		}
		if d, exists := sp["description"].(string); exists {
			profileDescription = d
		}
	}
	matchedCode := profileCode

	axisValuesStr := make(map[string]string, len(axisValues))
	for k, v := range axisValues {
		axisValuesStr[k] = fmt.Sprintf("%v", v)
	}

	if len(axisValuesStr) > 0 {
		var orderedCodes []string
		axisOrder := []string{"SEBUM", "OILINESS", "SENSITIVITY", "PIGMENTATION", "AGING", "WRINKLE", "BARRIER"}
		for _, k := range axisOrder {
			if val, exists := axisValuesStr[k]; exists && val != "" {
				orderedCodes = append(orderedCodes, val)
			}
		}
		remaining := make([]string, 0)
		for k := range axisValuesStr {
			known := false
			for _, o := range axisOrder {
				if o == k {
					known = true
					break
				}
			}
			if !known && axisValuesStr[k] != "" {
				remaining = append(remaining, k)
			}
		}
		sort.Strings(remaining)
		for _, k := range remaining {
			orderedCodes = append(orderedCodes, axisValuesStr[k])
		}
		if composed := strings.Join(orderedCodes, ""); composed != "" {
			profileCode = composed
		}
	}
	if profileCode == "" {
		profileCode = "CUSTOM"
	}
	if matchedCode != profileCode {
		profileName, profileCategory, profileDescription = "", "", ""
	}
	if profileName == "" {
		profileName = "Diagnostic Profile: " + profileCode
	}
	if profileDescription == "" {
		profileDescription = fmt.Sprintf("Evaluasi profil kulit %s dikomposisi secara dinamis dari aturan GoRules JDM %s.", profileCode, ruleset.Code)
	}

	complete := true
	for key := range axisCodes {
		b, ok := dimensions[key]
		if !ok || b.Axis == nil {
			complete = false
			break
		}
	}

	return domain.SkinProfileV2{
		Code:        profileCode,
		Name:        profileName,
		Category:    profileCategory,
		Description: profileDescription,
		Complete:    complete,
		AxisValues:  axisValuesStr,
	}
}

// EvaluateV2Input is the unified evaluate endpoint's input — :code from the
// URL is always a ruleset code. data/photo are each independently optional;
// stage2Score's per-dimension fallback (100% of whichever side is present)
// handles every combination without a separate "mode" concept.
type EvaluateV2Input struct {
	RulesetCode   string
	BrandID       string
	ApplicationID string
	CustomerID    string
	// Data is the only input customer_condition flags come from (via each
	// answered choice's condition_map, resolved by ExtractEvaluationData) —
	// there is deliberately no separate customer_conditions override field.
	// A second, ungoverned channel for setting flags would bypass the
	// ref_conditions catalog entirely.
	Data map[string]interface{}

	FullName              string
	PhoneNumber           string
	Email                 string
	DateOfBirth           string
	ConsentDataProcessing bool
	ConsentMarketing      bool
	Photo                 io.Reader
	PhotoFilename         string
}

func (s *ScoreService) EvaluateV2(ctx context.Context, in EvaluateV2Input) (*domain.EvaluateV2Response, error) {
	verrs := &domain.ValidationErrors{}
	if in.BrandID == "" {
		verrs.Add("brand_id is required")
	}
	if in.ApplicationID == "" {
		verrs.Add("application_id is required")
	}
	if in.CustomerID == "" {
		verrs.Add("customer_id is required")
	}
	if in.Photo != nil {
		if in.FullName == "" {
			verrs.Add("full_name is required when a photo is submitted")
		}
		if in.PhoneNumber == "" {
			verrs.Add("phone_number is required when a photo is submitted")
		}
		if in.Email == "" {
			verrs.Add("email is required when a photo is submitted")
		}
		if in.DateOfBirth == "" {
			verrs.Add("date_of_birth is required when a photo is submitted")
		}
		if !in.ConsentDataProcessing {
			verrs.Add("consent_data_processing must be true when a photo is submitted")
		}
	}
	if in.Photo == nil && len(in.Data) == 0 {
		verrs.Add("at least one of data or photo is required")
	}
	if verrs.HasErrors() {
		return nil, verrs
	}
	// Rulesets and surveys are keyed by brand code; the assessment record
	// below keeps the brand exactly as the caller sent it.
	lookupBrand, err := s.rulesetBrand(ctx, in.BrandID)
	if err != nil {
		return nil, err
	}

	ruleset, err := s.repo.GetActiveRuleset(ctx, lookupBrand, in.ApplicationID, in.RulesetCode)
	if err != nil || ruleset == nil {
		return nil, fmt.Errorf("active scoring ruleset not found for code %q: %w", in.RulesetCode, err)
	}
	meta := parseRulesetMeta(ruleset.Schema)

	formConcern := make(map[string]float64)
	visionConcern := make(map[string]float64)
	customerCondition := make(map[string]bool)
	var answerList []domain.AnswerEntry
	var labels map[string]interface{}
	var visionTxID string
	warnings := []string{}

	if len(in.Data) > 0 {
		if meta.FormSurveyCode == "" {
			verrs.Add("this ruleset has no linked questionnaire (form_survey_code) — form answers cannot be interpreted")
			return nil, verrs
		}
		survey, err := s.surveySvc.GetSurvey(ctx, lookupBrand, in.ApplicationID, meta.FormSurveyCode)
		if err != nil {
			return nil, fmt.Errorf("loading linked survey %q: %w", meta.FormSurveyCode, err)
		}

		if rverrs := formdomain.ValidateRequiredAnswers(survey, in.Data); rverrs.HasErrors() {
			verrs.Errors = append(verrs.Errors, rverrs.Errors...)
			return nil, verrs
		}

		fAnswers, dimensionRefs, fConds, fLabels, err := formdomain.ExtractEvaluationData(survey, in.Data)
		if err != nil {
			return nil, fmt.Errorf("parsing form answers: %w", err)
		}
		for k, v := range fConds {
			customerCondition[k] = v
		}
		labels = fLabels
		for _, a := range fAnswers {
			answerList = append(answerList, domain.AnswerEntry{Question: a.Question, Answer: a.Answer, Score: a.Score})
		}
		for _, dref := range dimensionRefs {
			formConcern[strings.ToLower(dref.Key)] = aggregateAndNormalizeConcern(dref)
		}
	}

	if in.Photo != nil {
		providerCode := meta.VisionSourceCode
		provider, ok := s.visionRegistry.Resolve(providerCode)
		if !ok {
			return nil, fmt.Errorf("%w: no vision provider registered for vision_source_code %q", domain.ErrVisionUnavailable, providerCode)
		}
		result, err := provider.Submit(ctx, domain.VisionSubmission{
			FullName:              in.FullName,
			PhoneNumber:           in.PhoneNumber,
			Email:                 in.Email,
			DateOfBirth:           in.DateOfBirth,
			ConsentDataProcessing: in.ConsentDataProcessing,
			ConsentMarketing:      in.ConsentMarketing,
			Image:                 in.Photo,
			ImageFilename:         in.PhotoFilename,
		})
		if err != nil {
			return nil, err
		}
		visionTxID = result.TransactionID

		for dimKey, fm := range meta.FieldMapping {
			if fm.Vision == "" {
				continue
			}
			healthVal, ok := result.Flattened[fm.Vision]
			if !ok {
				// Paradev answered, but the field this dimension's
				// field_mapping.vision names isn't in its response — this
				// falls back to form-only for this dimension exactly the way
				// it would if no photo had been sent at all, but that must
				// never happen silently: a wrong/stale mapping path (a real
				// bug found in this session) would otherwise look identical
				// to "this ruleset just doesn't have a vision side here."
				warnings = append(warnings, fmt.Sprintf(
					"VISION_FIELD_MISSING: %s expected vision field %q but it was not present in the vendor response",
					dimKey, fm.Vision,
				))
				continue
			}
			visionConcern[dimKey] = clamp100(100.0 - healthVal)
		}
	}

	// Aging (W/T) is vision+DOB — per the guideline a photo (wrinkle
	// reading) is mandatory; date_of_birth alone can never resolve it, no
	// matter how complete the questionnaire is. Both a missing photo and a
	// missing DOB are reported independently (either can be true without
	// the other), and formConcern is only ever set when BOTH are present.
	var ageMappedDims []string
	for dimKey, fm := range meta.FieldMapping {
		if fm.Form == "age_over_30" {
			ageMappedDims = append(ageMappedDims, strings.ToLower(dimKey))
		}
	}
	if len(ageMappedDims) > 0 {
		if in.Photo == nil {
			for _, dimKey := range ageMappedDims {
				warnings = append(warnings, fmt.Sprintf(
					"VISION_REQUIRED: %s requires a photo (wrinkle reading) — date_of_birth alone is not sufficient per the guideline",
					dimKey,
				))
			}
		}
		if in.DateOfBirth == "" {
			// Must never quietly read as "age <= 30" (ageConcern 0) — the
			// dimension stays unset (source: "none" unless vision covers
			// it), and the caller is told explicitly why.
			for _, dimKey := range ageMappedDims {
				warnings = append(warnings, fmt.Sprintf(
					"DOB_MISSING: %s expects date_of_birth to compute its form-side value, none was provided",
					dimKey,
				))
			}
		} else if in.Photo != nil {
			overThirty, ageErr := formdomain.AgeOverThirty(in.DateOfBirth, time.Now())
			if ageErr != nil {
				warnings = append(warnings, fmt.Sprintf("invalid date_of_birth: %v", ageErr))
			} else {
				ageConcern := 0.0
				if overThirty {
					ageConcern = 100.0
				}
				for _, dimKey := range ageMappedDims {
					formConcern[dimKey] = ageConcern
				}
			}
		}
	}

	out, err := s.stage2Score(ctx, stage2Input{
		Ruleset:           ruleset,
		FormConcern:       formConcern,
		VisionConcern:     visionConcern,
		CustomerCondition: customerCondition,
	})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, out.Warnings...)

	return &domain.EvaluateV2Response{
		Success:             true,
		Code:                ruleset.Code,
		BrandID:             in.BrandID,
		ApplicationID:       in.ApplicationID,
		CustomerID:          in.CustomerID,
		TotalScore:          out.TotalScore,
		Dimensions:          out.Dimensions,
		SkinProfile:         out.SkinProfile,
		SubClassification:   out.SubClassification,
		CustomerCondition:   customerCondition,
		AnswerList:          answerList,
		Labels:              labels,
		AppliedRules:        out.AppliedRules,
		Warnings:            warnings,
		VisionTransactionID: visionTxID,
		EvaluatedAt:         time.Now().UTC().Format(time.RFC3339),
	}, nil
}
