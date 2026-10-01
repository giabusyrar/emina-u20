package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/experience-gateway/apps/core-engine/internal/brand"
	"github.com/experience-gateway/apps/core-engine/internal/score/domain"
	"github.com/experience-gateway/apps/core-engine/internal/score/entity"
	"github.com/experience-gateway/apps/core-engine/internal/score/jdm"
	"github.com/experience-gateway/apps/core-engine/internal/score/repository"

	formdomain "github.com/experience-gateway/apps/core-engine/internal/form/domain"
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

// fusionWeight is a ruleset-defined default form/vision split for one
// dimension (e.g. sebum: 100% form, wrinkle: 100% vision). It's the ruleset's
// baseline; a caller's own request-level FusionConfig still overrides it.
type fusionWeight struct {
	Form   float64 `json:"form"`
	Vision float64 `json:"vision"`
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
		if len(axisCodes) > 0 {
			// This ruleset explicitly registered which keys are real Baumann
			// axes — a key it left out (e.g. pore_severity: a
			// sub_classification input, not an axis) gets no letter at all
			// here, rather than a meaningless one derived from generic Score
			// Range bands that happens to change across evaluations with no
			// real significance.
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

// surveyProvider is the one method EvaluateV2 needs from Form Engine's
// SurveyService — a narrow interface so Score Engine depends on a contract,
// not the concrete Form Engine service type.
type surveyProvider interface {
	GetSurvey(ctx context.Context, brandID, applicationID, code string) (*formdomain.SurveyModel, error)
}

type ScoreService struct {
	repo           repository.RulesetRepository
	surveySvc      surveyProvider
	visionRegistry *domain.VisionProviderRegistry
	brands         BrandResolver
}

// BrandResolver turns the brand a request names (ref_brands id or code)
// into the brand row; internal/brand implements it.
type BrandResolver interface {
	Resolve(ctx context.Context, ref string) (brand.Brand, error)
}

// WithBrands sets the resolver that lets callers name a brand by its
// ref_brands id as well as its code. Rulesets are keyed by brand code.
func (s *ScoreService) WithBrands(r BrandResolver) *ScoreService {
	s.brands = r
	return s
}

// rulesetBrand is the key rulesets and surveys are stored under for the
// brand the request named: its lower-case code. "" and "*" (every brand)
// pass through, as does everything when no resolver is configured.
func (s *ScoreService) rulesetBrand(ctx context.Context, ref string) (string, error) {
	if ref == "" || ref == "*" || s.brands == nil {
		return ref, nil
	}
	b, err := s.brands.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	return b.Code, nil
}

func NewScoreService(repo repository.RulesetRepository) *ScoreService {
	return &ScoreService{
		repo: repo,
	}
}

// WithEvaluateV2Deps wires the unified evaluate endpoint's extra
// dependencies (Form Engine's survey lookup, the vision provider registry)
// onto an already-constructed ScoreService — kept separate from
// NewScoreService so every existing caller of that constructor (tests, the
// legacy Evaluate/Simulate paths) is unaffected.
func (s *ScoreService) WithEvaluateV2Deps(surveySvc surveyProvider, visionRegistry *domain.VisionProviderRegistry) *ScoreService {
	s.surveySvc = surveySvc
	s.visionRegistry = visionRegistry
	return s
}

// Evaluate processes questionnaire answers, fuses multi-modal signals, and runs JDM Decision Tables
//
// It needs something to score: dimensions, an answer list or vision signals.
// With none of them the total used to be 0, which the default bands read as
// "Sangat Parah" — a diagnosis made from nothing. That is now a validation
// error.
func (s *ScoreService) Evaluate(ctx context.Context, req domain.ScoreModuleRequest) (*domain.ScoreModuleResponse, error) {
	if len(req.Dimensions) == 0 && len(req.AnswerList) == 0 && len(req.VisionSignals) == 0 {
		verrs := &domain.ValidationErrors{}
		verrs.Add("nothing to score: send dimensions, answer_list or vision_signals")
		return nil, verrs
	}
	brandID, err := s.rulesetBrand(ctx, req.BrandID)
	if err != nil {
		return nil, err
	}
	if brandID == "" {
		brandID = "*"
	}
	appID := req.ApplicationID
	if appID == "" {
		appID = "*"
	}

	dimensionScores := make(map[string]float64)
	// rawDimensionScores is the aggregate BEFORE min-max normalisation (e.g.
	// Q1's raw answer sum) — needed downstream for a hypothesis band (Sub
	// Oily) that reads the raw total directly, not the normalised 0-100
	// concern figure dimensionScores carries.
	rawDimensionScores := make(map[string]float64)
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
			rawDimensionScores[strings.ToLower(dim.Key)] = agg
		}
	} else if len(req.AnswerList) > 0 {
		answerSum := 0
		for _, ans := range req.AnswerList {
			answerSum += ans.Score
		}
		dimensionScores["total"] = float64(answerSum)
	}

	// 2. Fetch the JDM Decision Model early — its dimension_fusion config (set
	// on the ruleset's own Blending tab) is the default form/vision split used
	// by fusion below, before the JDM itself runs.
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
		DimensionWeights map[string]float64      `json:"dimension_weights"`
		DimensionFusion  map[string]fusionWeight `json:"dimension_fusion"`
		ScoreRangeBands  []band                  `json:"score_range_bands"`
		SeverityBands    []band                  `json:"severity_bands"`
		ConcernLabels    map[string]string       `json:"concern_labels"`
		AxisCodes        map[string]axisCode     `json:"axis_codes"`
		// SuppressSeverityLabels opts a ruleset out of score_range/
		// severity_level in the response — for a ruleset like Skinverse
		// whose spec never defines a generic total-score severity layer at
		// all, a mean-of-axes bucket label ("Sangat Parah") reads as an
		// alarming diagnosis the ruleset never actually made. Explicit
		// per-ruleset opt-in (set on its Setup tab), not inferred, so every
		// ruleset that already relies on these labels keeps working exactly
		// as before.
		SuppressSeverityLabels bool `json:"suppress_severity_labels"`
	}
	_ = json.Unmarshal([]byte(ruleset.Schema), &meta)

	// 1.1 Multi-Modal Dynamic Fusion (still in concern space — form + vision
	// signals share the low->high severity convention here). The ruleset's own
	// dimension_fusion is the default per-dimension form/vision split; a
	// caller's request-level FusionConfig, if sent, overrides it entirely
	// (fusion_config is still an all-or-nothing override, not merged field by
	// field, to keep the precedence simple and predictable).
	if len(req.VisionSignals) > 0 {
		fusionCfg := domain.FusionConfig{}
		if req.FusionConfig != nil {
			fusionCfg = *req.FusionConfig
		} else if len(meta.DimensionFusion) > 0 {
			dw := make(map[string]domain.DimensionWeight, len(meta.DimensionFusion))
			for k, fw := range meta.DimensionFusion {
				dw[strings.ToLower(k)] = domain.DimensionWeight{FormWeight: fw.Form, VisionWeight: fw.Vision}
			}
			fusionCfg.DimensionWeights = dw
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

	// Extract ruleset-defined sub-classification fields (e.g. sub_sensitive),
	// written by a decisionTableNode to sub_classification.<key>.
	var subClassification map[string]interface{}
	if sc, ok := evalResult["sub_classification"].(map[string]interface{}); ok {
		subClassification = sc
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
	matchedCode := profileCode

	// The Baumann-style 4-letter code is always the axes themselves, in
	// canonical order — never whatever a Total Score-strategy profile node
	// matched instead (a generic score-range bucket like "CONCERN" has
	// nothing to do with the real O/S/P/T code once this ruleset has axes at
	// all). A Combination Matrix-strategy node's own match, if any, still
	// only ever produces the same letters in the same order, so overriding
	// unconditionally here is safe rather than only filling in when empty.
	if len(axisValues) > 0 {
		// Only a key the ruleset itself registered as a real bipolar axis
		// (axis_codes) contributes a letter to the identity code — e.g.
		// pore_severity has a dimension_scores value and its own
		// sub_classification output, but it is not a Baumann axis and must
		// never turn a 4-letter code into "OSPTO". When a ruleset defines no
		// axis_codes at all, there is no such signal to filter by, so every
		// axisValues key is used instead (unchanged prior behaviour).
		isRealAxis := func(k string) bool {
			if len(meta.AxisCodes) == 0 {
				return true
			}
			_, ok := meta.AxisCodes[strings.ToLower(k)]
			return ok
		}

		var orderedCodes []string
		axisOrder := []string{"SEBUM", "OILINESS", "SENSITIVITY", "PIGMENTATION", "AGING", "WRINKLE", "BARRIER"}
		for _, k := range axisOrder {
			if val, exists := axisValues[k]; exists && val != "" && isRealAxis(k) {
				orderedCodes = append(orderedCodes, val)
			}
		}
		remainingKeys := make([]string, 0)
		for k := range axisValues {
			known := false
			for _, o := range axisOrder {
				if o == k {
					known = true
					break
				}
			}
			if !known && axisValues[k] != "" && isRealAxis(k) {
				remainingKeys = append(remainingKeys, k)
			}
		}
		sort.Strings(remainingKeys) // deterministic order for any axis this list doesn't already know
		for _, k := range remainingKeys {
			orderedCodes = append(orderedCodes, axisValues[k])
		}
		if composed := strings.Join(orderedCodes, ""); composed != "" {
			profileCode = composed
		}
	}
	if profileCode == "" {
		profileCode = "CUSTOM"
	}
	// A name/description the JDM matched only makes sense for the code it
	// was matched against — if axis composition just replaced a Total
	// Score-strategy bucket's code (e.g. "CONCERN" -> "OSPT"), that bucket's
	// name/description ("Perlu Perhatian Khusus", ...) would otherwise be
	// left attached to a code it has nothing to do with. Clearing them here
	// lets the generic fallback below take over instead of showing a
	// mismatched pair. A real Combination Matrix per-code entry (matchedCode
	// == profileCode already) is left untouched.
	if matchedCode != profileCode {
		profileName = ""
		profileCategory = ""
		profileDescription = ""
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

	responseScoreRange, responseSeverityLevel := scoreRange, severityLevel
	if meta.SuppressSeverityLabels {
		responseScoreRange, responseSeverityLevel = "", ""
	}

	return &domain.ScoreModuleResponse{
		Success:            true,
		TotalScore:         totalScore,
		DimensionScores:    dimensionScores,
		ScoreRange:         responseScoreRange,
		SeverityLevel:      responseSeverityLevel,
		SkinConcern:        skinConcern,
		AppliedRules:       appliedRules,
		SkinProfile:        skinProfile,
		SkinGradingTiers:   skinGradingTiers,
		CustomerCondition:  req.CustomerCondition,
		SubClassification:  subClassification,
		RawDimensionScores: rawDimensionScores,
	}, nil
}

// Ruleset Management CRUD

func (s *ScoreService) ListRulesets(ctx context.Context, brandID, appID string) ([]entity.ScoreRuleset, error) {
	brandID, err := s.rulesetBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRulesets(ctx, brandID, appID)
}

// GetActiveRuleset resolves the same (brand, application, code) -> ruleset
// lookup Evaluate itself uses, exposed for callers (e.g. the Form Engine's
// photo-evaluate orchestration) that need the ruleset's field_mapping before
// they can build a vision_signals payload to send to Evaluate.
func (s *ScoreService) GetActiveRuleset(ctx context.Context, brandID, appID, code string) (*entity.ScoreRuleset, error) {
	brandID, err := s.rulesetBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetActiveRuleset(ctx, brandID, appID, code)
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

// Simulate runs a live evaluation against an uncommitted JDM schema — it
// feeds its slider inputs straight into stage2Score (the exact same fusion,
// health-flip, axis-letter and JDM logic the real evaluate endpoint uses),
// converting only the input shape (health-space sliders -> concern space)
// beforehand. This guarantees the Simulator can never drift from what a real
// evaluate call would compute for the same numbers.
func (s *ScoreService) Simulate(ctx context.Context, req entity.RulesetSimulationRequest) (*entity.RulesetSimulationResponse, error) {
	if req.Schema == "" {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   "JDM schema is required",
		}, nil
	}

	start := time.Now()
	if _, err := jdm.NewRunner(req.Schema); err != nil {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   fmt.Sprintf("Compilation error: %v", err),
		}, nil
	}

	meta := parseRulesetMeta(req.Schema)
	ruleset := &entity.ScoreRuleset{Schema: req.Schema, Code: "simulation", Version: 0}

	formConcern := make(map[string]float64, len(req.FormScores))
	for k, healthVal := range req.FormScores {
		formConcern[strings.ToLower(k)] = clamp100(100.0 - healthVal)
	}
	visionConcern := make(map[string]float64, len(req.VisionScores))
	for k, healthVal := range req.VisionScores {
		visionConcern[strings.ToLower(k)] = clamp100(100.0 - healthVal)
	}

	// Age slider: reuse the real AgeOverThirty via a synthetic DOB (born
	// exactly req.AgeYears years ago today) rather than re-deriving the <=30
	// comparison a second time.
	if req.AgeYears != nil {
		dob := syntheticDOBFromAge(*req.AgeYears, time.Now())
		if overThirty, err := formdomain.AgeOverThirty(dob, time.Now()); err == nil {
			ageConcern := 0.0
			if overThirty {
				ageConcern = 100.0
			}
			for dimKey, fm := range meta.FieldMapping {
				if fm.Form == "age_over_30" {
					formConcern[strings.ToLower(dimKey)] = ageConcern
				}
			}
		}
	}

	out, err := s.stage2Score(ctx, stage2Input{
		Ruleset:           ruleset,
		FormConcern:       formConcern,
		VisionConcern:     visionConcern,
		CustomerCondition: req.CustomerCondition,
	})
	duration := time.Since(start)
	if err != nil {
		return &entity.RulesetSimulationResponse{
			Success: false,
			Error:   fmt.Sprintf("Evaluation error: %v", err),
		}, nil
	}

	result := map[string]interface{}{
		"total_score":  out.TotalScore,
		"dimensions":   out.Dimensions,
		"skin_profile": out.SkinProfile,
	}
	if len(out.SubClassification) > 0 {
		result["sub_classification"] = out.SubClassification
	}
	if len(out.Warnings) > 0 {
		result["warnings"] = out.Warnings
	}

	return &entity.RulesetSimulationResponse{
		Success:     true,
		Performance: fmt.Sprintf("%v", duration),
		Result:      result,
	}, nil
}
