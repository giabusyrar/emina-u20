package jdm

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DecisionModel represents a standard GoRules JDM Decision Model
type DecisionModel struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"` // "inputNode", "decisionTableNode", "expressionNode", "outputNode"
	Content  json.RawMessage `json:"content,omitempty"`
	Position *Position       `json:"position,omitempty"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
}

type DecisionTableContent struct {
	HitPolicy string              `json:"hitPolicy"` // "first", "collect"
	Inputs    []TableColumn       `json:"inputs"`
	Outputs   []TableColumn       `json:"outputs"`
	Rules     []map[string]string `json:"rules"`
}

type TableColumn struct {
	ID    string `json:"id"`
	Field string `json:"field"`
	Label string `json:"label"`
}

type Runner struct {
	model DecisionModel
}

func NewRunner(schemaJSON string) (*Runner, error) {
	var model DecisionModel
	if err := json.Unmarshal([]byte(schemaJSON), &model); err != nil {
		return nil, fmt.Errorf("invalid JDM schema: %w", err)
	}
	return &Runner{model: model}, nil
}

// Evaluate executes the JDM decision graph in topological order
func (r *Runner) Evaluate(ctx map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	// Copy input context
	for k, v := range ctx {
		output[k] = v
	}

	// Find and evaluate decision table nodes
	for _, node := range r.model.Nodes {
		if node.Type == "decisionTableNode" && len(node.Content) > 0 {
			var dt DecisionTableContent
			if err := json.Unmarshal(node.Content, &dt); err != nil {
				continue
			}

			err := r.evaluateDecisionTable(dt, output)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate table node %s: %w", node.Name, err)
			}
		}
	}

	return output, nil
}

func (r *Runner) evaluateDecisionTable(dt DecisionTableContent, data map[string]interface{}) error {
	for _, rule := range dt.Rules {
		matchedAll := true

		for _, inCol := range dt.Inputs {
			ruleVal, exists := rule[inCol.ID]
			if !exists || ruleVal == "" || ruleVal == "-" {
				continue // wildcard matches anything
			}

			fieldVal := resolveField(data, inCol.Field)
			if !matchUnaryTest(fieldVal, ruleVal) {
				matchedAll = false
				break
			}
		}

		if matchedAll {
			// Apply outputs
			for _, outCol := range dt.Outputs {
				if outVal, exists := rule[outCol.ID]; exists && outVal != "" && outVal != "-" {
					parsedVal := parseOutputValue(outVal)
					setField(data, outCol.Field, parsedVal)
				}
			}

			if dt.HitPolicy == "first" || dt.HitPolicy == "" {
				break
			}
		}
	}
	return nil
}

// matchUnaryTest evaluates unary tests like [0..25], [26..49], >=50, "O", true, etc.
func matchUnaryTest(actual interface{}, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "-" {
		return true
	}

	// Interval test: [min..max], [min..max), (min..max]
	if (strings.HasPrefix(pattern, "[") || strings.HasPrefix(pattern, "(")) &&
		(strings.HasSuffix(pattern, "]") || strings.HasSuffix(pattern, ")")) &&
		strings.Contains(pattern, "..") {
		return matchInterval(actual, pattern)
	}

	// Comparison tests: >=x, <=x, >x, <x, !=x
	numVal, isNum := toFloat64(actual)
	if isNum {
		if strings.HasPrefix(pattern, ">=") {
			target, err := strconv.ParseFloat(strings.TrimSpace(pattern[2:]), 64)
			return err == nil && numVal >= target
		}
		if strings.HasPrefix(pattern, "<=") {
			target, err := strconv.ParseFloat(strings.TrimSpace(pattern[2:]), 64)
			return err == nil && numVal <= target
		}
		if strings.HasPrefix(pattern, ">") {
			target, err := strconv.ParseFloat(strings.TrimSpace(pattern[1:]), 64)
			return err == nil && numVal > target
		}
		if strings.HasPrefix(pattern, "<") {
			target, err := strconv.ParseFloat(strings.TrimSpace(pattern[1:]), 64)
			return err == nil && numVal < target
		}
		if strings.HasPrefix(pattern, "!=") {
			target, err := strconv.ParseFloat(strings.TrimSpace(pattern[2:]), 64)
			return err == nil && numVal != target
		}
	}

	// String equality
	cleanPattern := strings.Trim(pattern, `"'`)
	actualStr := fmt.Sprintf("%v", actual)
	return strings.EqualFold(actualStr, cleanPattern)
}

func matchInterval(actual interface{}, interval string) bool {
	numVal, ok := toFloat64(actual)
	if !ok {
		return false
	}

	inclusiveStart := strings.HasPrefix(interval, "[")
	inclusiveEnd := strings.HasSuffix(interval, "]")

	inner := interval[1 : len(interval)-1]
	parts := strings.Split(inner, "..")
	if len(parts) != 2 {
		return false
	}

	minVal, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	maxVal, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return false
	}

	if inclusiveStart {
		if numVal < minVal {
			return false
		}
	} else {
		if numVal <= minVal {
			return false
		}
	}

	if inclusiveEnd {
		if numVal > maxVal {
			return false
		}
	} else {
		if numVal >= maxVal {
			return false
		}
	}

	return true
}

func parseOutputValue(val string) interface{} {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
		return strings.Trim(val, `"`)
	}
	if strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`) {
		return strings.Trim(val, `'`)
	}
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	if num, err := strconv.ParseFloat(val, 64); err == nil {
		return num
	}
	return val
}

func resolveField(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			// Try map[string]float64
			if mf, okf := current.(map[string]float64); okf {
				if val, has := mf[part]; has {
					return val
				}
			}
			// Try map[string]bool (e.g. customer_condition, built as
			// map[string]bool end to end — a rule reading
			// customer_condition.has_acne otherwise always resolves nil).
			if mb, okb := current.(map[string]bool); okb {
				if val, has := mb[part]; has {
					return val
				}
			}
			return nil
		}
		current = m[part]
	}
	return current
}

func setField(data map[string]interface{}, path string, val interface{}) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		data[parts[0]] = val
		return
	}

	var current map[string]interface{} = data
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		sub, exists := current[part]
		if !exists || sub == nil {
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		} else if subMap, ok := sub.(map[string]interface{}); ok {
			current = subMap
		} else {
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}
	current[parts[len(parts)-1]] = val
}

func toFloat64(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// RoundFloat helper
func RoundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
