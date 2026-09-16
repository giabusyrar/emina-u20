package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/experience-gateway/apps/core-engine/pkg/score/domain"
	"github.com/experience-gateway/apps/core-engine/pkg/score/entity"
	"github.com/experience-gateway/apps/core-engine/pkg/score/jdm"
	"github.com/experience-gateway/apps/core-engine/pkg/score/repository"
)

// The 0-100 score is HEALTH-oriented: 100 = optimal, 0 = critical. Per-dimension
// scores are inverted after normalisation (see Evaluate) so that a higher number
// always means healthier skin. These band edges are the defaults; a ruleset may
// override them via score_range_bands / severity_bands.
type band struct {
	Max   float64 `json:"max"`
	Label string  `json:"label"`
}

// Score Range: a coarse 3-bucket collapse of the Severity Level scale.
var defaultScoreRangeBands = []band{
	{Max: 40, Label: "Perlu Perhatian Khusus"},
	{Max: 60, Label: "Sedang"},
	{Max: 100, Label: "Optimal"},
}

// Severity Level: the clinical 5-level scale (Scoring Method doc), health-oriented.
var defaultSeverityBands = []band{
	{Max: 20, Label: "Sangat Parah"},
	{Max: 40, Label: "Parah"},
	{Max: 60, Label: "Sedang"},
	{Max: 80, Label: "Ringan"},
	{Max: 100, Label: "Sehat"},
}

// bandLabel returns the label of the first band whose Max the score does not exceed.
func bandLabel(s float64, bands []band) string {
	for _, b := range bands {
		if s <= b.Max {
			return b.Label
		}
	}
	if len(bands) > 0 {
		return bands[len(bands)-1].Label
	}
	return ""
}

// optimalFloorOf returns the lower edge of the top (Optimal) band — i.e. the
// score at or above which a dimension is considered healthy. Below it, the
// dimension can surface as a Skin Concern.
func optimalFloorOf(bands []band) float64 {
	if len(bands) >= 2 {
		return bands[len(bands)-2].Max
	}
	return 60
}

// defaultConcernLabels maps a dimension key to its clinical concern name; a
// ruleset may override any of these via concern_labels.
var defaultConcernLabels = map[string]string{
	"sebum":        "Minyak Berlebih",
	"oiliness":     "Minyak Berlebih",
	"sensitivity":  "Kulit Sensitif",
	"pigmentation": "Noda Gelap",
	"dark_spot":    "Noda Gelap",
	"aging":        "Garis Halus & Kerutan",
	"hydration":    "Kulit Kering",
	"acne":         "Jerawat",
	"pores":        "Pori Besar",
	"barrier":      "Barier Kulit Rusak",
}

func concernLabel(dim string, overrides map[string]string) string {
	if overrides != nil {
		if v, ok := overrides[strings.ToLower(dim)]; ok && v != "" {
			return v
		}
	}
	if v, ok := defaultConcernLabels[strings.ToLower(dim)]; ok {
		return v
	}
	return strings.Title(strings.ReplaceAll(dim, "_", " "))
}

// axisCode is a per-dimension bipolar classifier (Baumann-style): score below
// Threshold -> Low letter, at/above -> High letter (e.g. sebum {50, "D", "O"}).
type axisCode struct {
	Threshold float64 `json:"threshold"`
	Low       string  `json:"low"`
	High      string  `json:"high"`
}

// axisValuesFromScores gives each dimension a single-letter code for the
// Combination Matrix profile strategy, keyed by the upper-cased dimension key.
// If the ruleset declares axis_codes for a dimension, use its bipolar threshold
// (Baumann O/D, S/R, ...); otherwise fall back to the Score Range band initial.
func axisValuesFromScores(scores map[string]float64, bands []band, axisCodes map[string]axisCode) map[string]interface{} {
	out := make(map[string]interface{}, len(scores))
	for k, v := range scores {
		if ac, ok := axisCodes[strings.ToLower(k)]; ok && (ac.Low != "" || ac.High != "") {
			if v >= ac.Threshold {
				out[strings.ToUpper(k)] = ac.High
			} else {
				out[strings.ToUpper(k)] = ac.Low
			}
			continue
		}
		label := bandLabel(v, bands)
		letter := ""
		if label != "" {
			letter = strings.ToUpper(label[:1])
		}
		out[strings.ToUpper(k)] = letter
	}
	return out
}

// isLegacyPerDimensionSchema reports whether the JDM carries per-dimension
// grading tables (they write tiers.<dim>.* and their own axis_values). For those
// the engine must not pre-seed axis_values or the two sets collide.
func isLegacyPerDimensionSchema(schema string) bool {
	return strings.Contains(schema, "tiers.")
}

type ScoreService struct {
	repo repository.RulesetRepository
}

func NewScoreService(repo repository.RulesetRepository) *ScoreService {
	return &ScoreService{
		repo: repo,
	}
}

// Evaluate processes questionnaire answers, fuses multi-modal signals, and runs JDM Decision Tables
func (s *ScoreService) Evaluate(ctx context.Context, req domain.ScoreModuleRequest) (*domain.ScoreModuleResponse, error) {
	brandID := req.BrandID
	if brandID == "" {
		brandID = "*"
	}
	appID := req.ApplicationID
	if appID == "" {
		appID = "*"
	}

	dimensionScores := make(map[string]float64)
	appliedRules := make([]string, 0)
	totalScore := 0.0

	// 1. Process Input Dimensions
	if len(req.Dimensions) > 0 {
		for _, dim := range req.Dimensions {
			rawTotal := 0
			for _, score := range dim.Answers {
				rawTotal += score
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

			// 1. Aggregate the raw per-answer scores per the declared method.
			var agg float64
			switch strings.ToLower(calcMethod) {
			case "average", "mean":
				if len(dim.Answers) > 0 {
					agg = float64(rawTotal) / float64(len(dim.Answers))
				}
			case "highest", "max":
				for i, s := range dim.Answers {
					if i == 0 || float64(s) > agg {
						agg = float64(s)
					}
				}
			case "lowest", "min":
				for i, s := range dim.Answers {
					if i == 0 || float64(s) < agg {
						agg = float64(s)
					}
				}
			case "boolean_or":
				// any positive answer trips the dimension; map to the top of the range
				agg = minScore
				for _, s := range dim.Answers {
					if s > 0 {
						agg = maxScore
						break
					}
				}
			case "boolean_and":
				// every answer must be positive to trip the dimension
				agg = maxScore
				if len(dim.Answers) == 0 {
					agg = minScore
				}
				for _, s := range dim.Answers {
					if s <= 0 {
						agg = minScore
						break
					}
				}
			default: // "sum"
				agg = float64(rawTotal)
			}

			// 2. Min-Max normalise the raw aggregate to a 0-100 concern figure
			// (raw answers run low->high with severity, 0 = best answer). It is
			// flipped to the health-oriented convention after vision fusion,
			// below — fusing form + vision must happen in the same space.
			normalized := agg
			if maxScore > minScore {
				normalized = (agg - minScore) / (maxScore - minScore) * 100.0
			}
			if normalized < 0 {
				normalized = 0
			}
			if normalized > 100 {
				normalized = 100
			}

			dimensionScores[strings.ToLower(dim.Key)] = math.Round(normalized*10.0) / 10.0
		}
	} else if len(req.AnswerList) > 0 {
		answerSum := 0
		for _, ans := range req.AnswerList {
			answerSum += ans.Score
		}
		dimensionScores["total"] = float64(answerSum)
	}

	// 1.1 Multi-Modal Dynamic Fusion (still in concern space — form + vision
	// signals share the low->high severity convention here).
	if len(req.VisionSignals) > 0 {
		fusionCfg := domain.FusionConfig{}
		if req.FusionConfig != nil {
			fusionCfg = *req.FusionConfig
		}
		dimensionScores = domain.FuseScoresDynamic(dimensionScores, req.VisionSignals, fusionCfg)
		appliedRules = append(appliedRules, "Multi-Modal Dynamic Fusion applied")
	}

	// 1.2 Flip every per-dimension score to the HEALTH-oriented convention
	// (100 = optimal). From here on — total_score, bands, Skin Concern, the JDM
	// evalContext — a higher number always means healthier skin.
	for k, v := range dimensionScores {
		dimensionScores[k] = math.Round((100.0-v)*10.0) / 10.0
	}

	// 2. Fetch and Execute JDM Decision Model
	rulesetCode := req.Code
	if rulesetCode == "" {
		rulesetCode = "baumann_16_types"
	}

	ruleset, err := s.repo.GetActiveRuleset(ctx, brandID, appID, rulesetCode)
	if err != nil || ruleset == nil {
		return nil, fmt.Errorf("active scoring ruleset not found for code %q: %w", rulesetCode, err)
	}

	runner, err := jdm.NewRunner(ruleset.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to compile JDM ruleset schema (%s): %w", ruleset.Code, err)
	}

	// Config carried on the ruleset (compiled from the Skin Grading UI). All
	// optional: dimension weights default to equal, the band definitions and
	// concern labels default to the grand-design values.
	var meta struct {
		DimensionWeights map[string]float64  `json:"dimension_weights"`
		ScoreRangeBands  []band              `json:"score_range_bands"`
		SeverityBands    []band              `json:"severity_bands"`
		ConcernLabels    map[string]string   `json:"concern_labels"`
		AxisCodes        map[string]axisCode `json:"axis_codes"`
	}
	_ = json.Unmarshal([]byte(ruleset.Schema), &meta)

	scoreRangeBands := meta.ScoreRangeBands
	if len(scoreRangeBands) == 0 {
		scoreRangeBands = defaultScoreRangeBands
	}
	severityBands := meta.SeverityBands
	if len(severityBands) == 0 {
		severityBands = defaultSeverityBands
	}

	// total_score is the WEIGHTED MEAN of the per-dimension 0-100 scores — a
	// survey-independent overall figure. (A raw sum would range 0..N*100.)
	if len(dimensionScores) > 0 {
		var weightedSum, weightTotal float64
		for key, v := range dimensionScores {
			w := 1.0
			if mw, ok := meta.DimensionWeights[strings.ToLower(key)]; ok && mw > 0 {
				w = mw
			}
			weightedSum += v * w
			weightTotal += w
		}
		if weightTotal > 0 {
			totalScore = weightedSum / weightTotal
		}
	}
	totalScore = math.Round(totalScore*10.0) / 10.0

	// Skin Grading facets — all derived from the health-oriented total_score:
	// Score Range (coarse 3-bucket), Severity Level (clinical 5-level), Skin Concern.
	scoreRange := bandLabel(totalScore, scoreRangeBands)
	severityLevel := bandLabel(totalScore, severityBands)

	// Skin Concern = the dominant concern (argmin, since lower health = worse),
	// surfaced only when the weakest dimension falls below the Optimal band.
	var skinConcern *domain.SkinConcernFocus
	optimalFloor := optimalFloorOf(scoreRangeBands)
	if len(dimensionScores) > 0 {
		worstDim, worstScore := "", 101.0
		for k, v := range dimensionScores {
			if v < worstScore {
				worstScore, worstDim = v, k
			}
		}
		if worstDim != "" && worstScore < optimalFloor {
			skinConcern = &domain.SkinConcernFocus{
				Dimension: worstDim,
				Label:     concernLabel(worstDim, meta.ConcernLabels),
				Score:     worstScore,
			}
		}
	}

	// primary_concern.* is still exposed to the JDM for the Primary Concern
	// profile strategy — now argmin, keyed on the 5-level severity label.
	primaryConcern := map[string]interface{}{}
	if skinConcern != nil {
		primaryConcern = map[string]interface{}{
			"dimension": skinConcern.Dimension,
			"label":     skinConcern.Label,
			"score":     skinConcern.Score,
			"severity":  bandLabel(skinConcern.Score, severityBands),
		}
	}

	// Evaluation context passed to JDM Decision Table nodes
	evalContext := map[string]interface{}{
		"dimension_scores":   dimensionScores,
		"vision_signals":     req.VisionSignals,
		"customer_condition": req.CustomerCondition,
		"total_score":        totalScore,
		"score_range":        scoreRange,
		"severity_level":     severityLevel,
		"primary_concern":    primaryConcern,
		"axis_values":        axisValuesFromScores(dimensionScores, scoreRangeBands, meta.AxisCodes),
	}
	if isLegacyPerDimensionSchema(ruleset.Schema) {
		delete(evalContext, "axis_values")
	}

	evalResult, err := runner.Evaluate(evalContext)
	if err != nil {
		return nil, fmt.Errorf("JDM evaluation failed: %w", err)
	}

	appliedRules = append(appliedRules, fmt.Sprintf("GoRules JDM Model: %s (v%d)", ruleset.Code, ruleset.Version))

	// 3. Extract output variables from JDM evaluation result
	axisValues := make(map[string]string)
	if av, ok := evalResult["axis_values"].(map[string]interface{}); ok {
		for k, v := range av {
			axisValues[k] = fmt.Sprintf("%v", v)
		}
	}

	// Extract Traits
	traitsList := make([]string, 0)
	if tr, ok := evalResult["traits"].(map[string]interface{}); ok {
		for _, v := range tr {
			strVal := fmt.Sprintf("%v", v)
			if strVal != "" && strVal != "<nil>" {
				traitsList = append(traitsList, strVal)
			}
		}
	}
	sort.Strings(traitsList)

	// Per-dimension grading: each dimension's 0-100 concern score bucketed into
	// its Score Range band. Computed here, not in the JDM.
	dimKeys := make([]string, 0, len(dimensionScores))
	for k := range dimensionScores {
		dimKeys = append(dimKeys, k)
	}
	sort.Strings(dimKeys)
	skinGradingTiers := make([]domain.SkinGradingTier, 0, len(dimKeys))
	for _, dimKey := range dimKeys {
		score := dimensionScores[dimKey]
		label := bandLabel(score, scoreRangeBands)
		skinGradingTiers = append(skinGradingTiers, domain.SkinGradingTier{
			DimensionKey: dimKey,
			Score:        score,
			GradeName:    label,
			Severity:     label,
		})
	}

	// 4. Extract Skin Profile entity directly from JDM evaluation if available
	profileCode := ""
	profileName := ""
	profileCategory := ""
	profileDescription := ""

	if sp, ok := evalResult["skin_profile"].(map[string]interface{}); ok {
		if c, exists := sp["code"].(string); exists && c != "" {
			profileCode = c
		}
		if n, exists := sp["name"].(string); exists && n != "" {
			profileName = n
		}
		if cat, exists := sp["category"].(string); exists && cat != "" {
			profileCategory = cat
		}
		if desc, exists := sp["description"].(string); exists && desc != "" {
			profileDescription = desc
		}
	}

	// Fallback to ordered axis codes if no explicit profile code was matched
	if profileCode == "" {
		var orderedCodes []string
		axisOrder := []string{"OILINESS", "SENSITIVITY", "PIGMENTATION", "AGING", "BARRIER"}
		for _, k := range axisOrder {
			if val, exists := axisValues[k]; exists && val != "" {
				orderedCodes = append(orderedCodes, val)
			}
		}
		for k, val := range axisValues {
			found := false
			for _, o := range axisOrder {
				if o == k {
					found = true
					break
				}
			}
			if !found && val != "" {
				orderedCodes = append(orderedCodes, val)
			}
		}
		profileCode = strings.Join(orderedCodes, "")
		if profileCode == "" {
			profileCode = "CUSTOM"
		}
	}

	if profileName == "" {
		profileName = strings.Join(traitsList, ", ")
		if profileName == "" {
			profileName = "Diagnostic Profile: " + profileCode
		}
	}

	if profileDescription == "" {
		profileDescription = fmt.Sprintf("Evaluasi profil kulit %s dikomposisi secara dinamis dari aturan GoRules JDM %s.", profileCode, ruleset.Code)
	}

	skinProfile := domain.SkinProfileType{
		Code:        profileCode,
		Name:        profileName,
		Category:    profileCategory,
		Description: profileDescription,
		AxisValues:  axisValues,
		Traits:      traitsList,
	}

	return &domain.ScoreModuleResponse{
		Success:           true,
		TotalScore:        totalScore,
		DimensionScores:   dimensionScores,
		ScoreRange:        scoreRange,
		SeverityLevel:     severityLevel,
		SkinConcern:       skinConcern,
		AppliedRules:      appliedRules,
		SkinProfile:       skinProfile,
		SkinGradingTiers:  skinGradingTiers,
		CustomerCondition: req.CustomerCondition,
	}, nil
}

// Ruleset Management CRUD

func (s *ScoreService) ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error) {
	return s.repo.ListRulesets(ctx, brandID, appID)
}

func (s *ScoreService) GetRuleset(ctx context.Context, id string) (*entity.ScoreRuleset, error) {
	return s.repo.GetRulesetByID(ctx, id)
}

func (s *ScoreService) SaveRuleset(ctx context.Context, ruleset *entity.ScoreRuleset) error {
	// Validate schema syntax using JDM runner
	_, err := jdm.NewRunner(ruleset.Schema)
	if err != nil {
		return fmt.Errorf("invalid JDM decision model schema: %w", err)
	}

	return s.repo.SaveRuleset(ctx, ruleset)
}

func (s *ScoreService) DeleteRuleset(ctx context.Context, id string) error {
	return s.repo.DeleteRuleset(ctx, id)
}

// Simulate runs a live evaluation simulation against an uncommitted JDM schema
func (s *ScoreService) Simulate(ctx context.Context, req entity.RulesetSimulationRequest) (*entity.RulesetSimulationResponse, error) {
	if req.Schema == "" {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   "JDM schema is required",
		}, nil
	}

	start := time.Now()
	runner, err := jdm.NewRunner(req.Schema)
	if err != nil {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   fmt.Sprintf("Compilation error: %v", err),
		}, nil
	}

	// Mirror Evaluate's grading derivation so the Simulator shows the same
	// Score Range / Severity Level / Skin Concern the real pipeline would.
	var meta struct {
		DimensionWeights map[string]float64  `json:"dimension_weights"`
		ScoreRangeBands  []band              `json:"score_range_bands"`
		SeverityBands    []band              `json:"severity_bands"`
		ConcernLabels    map[string]string   `json:"concern_labels"`
		AxisCodes        map[string]axisCode `json:"axis_codes"`
	}
	_ = json.Unmarshal([]byte(req.Schema), &meta)
	scoreRangeBands := meta.ScoreRangeBands
	if len(scoreRangeBands) == 0 {
		scoreRangeBands = defaultScoreRangeBands
	}
	severityBands := meta.SeverityBands
	if len(severityBands) == 0 {
		severityBands = defaultSeverityBands
	}

	var totalScore float64
	if len(req.DimensionScores) > 0 {
		var ws, wt float64
		for k, v := range req.DimensionScores {
			w := 1.0
			if mw, ok := meta.DimensionWeights[strings.ToLower(k)]; ok && mw > 0 {
				w = mw
			}
			ws += v * w
			wt += w
		}
		if wt > 0 {
			totalScore = math.Round((ws/wt)*10.0) / 10.0
		}
	}

	scoreRange := bandLabel(totalScore, scoreRangeBands)
	severityLevel := bandLabel(totalScore, severityBands)
	optimalFloor := optimalFloorOf(scoreRangeBands)
	primaryConcern := map[string]interface{}{}
	var concernOut map[string]interface{}
	if len(req.DimensionScores) > 0 {
		worstDim, worstScore := "", 101.0
		for k, v := range req.DimensionScores {
			if v < worstScore {
				worstScore, worstDim = v, k
			}
		}
		if worstDim != "" && worstScore < optimalFloor {
			concernOut = map[string]interface{}{
				"dimension": worstDim,
				"label":     concernLabel(worstDim, meta.ConcernLabels),
				"score":     worstScore,
			}
			primaryConcern = map[string]interface{}{
				"dimension": worstDim,
				"label":     concernLabel(worstDim, meta.ConcernLabels),
				"score":     worstScore,
				"severity":  bandLabel(worstScore, severityBands),
			}
		}
	}

	evalContext := map[string]interface{}{
		"dimension_scores":   req.DimensionScores,
		"vision_signals":     req.VisionSignals,
		"customer_condition": req.CustomerCondition,
		"total_score":        totalScore,
		"score_range":        scoreRange,
		"severity_level":     severityLevel,
		"primary_concern":    primaryConcern,
		"axis_values":        axisValuesFromScores(req.DimensionScores, scoreRangeBands, meta.AxisCodes),
	}
	if isLegacyPerDimensionSchema(req.Schema) {
		delete(evalContext, "axis_values")
	}

	evalResult, err := runner.Evaluate(evalContext)
	duration := time.Since(start)

	if err != nil {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   fmt.Sprintf("Evaluation error: %v", err),
		}, nil
	}

	if evalResult == nil {
		evalResult = map[string]interface{}{}
	}
	evalResult["total_score"] = totalScore
	evalResult["score_range"] = scoreRange
	evalResult["severity_level"] = severityLevel
	if concernOut != nil {
		evalResult["skin_concern"] = concernOut
	}

	return &entity.RulesetSimulationResponse{
		Success:     true,
		Performance: fmt.Sprintf("%v", duration),
		Result:      evalResult,
	}, nil
}
