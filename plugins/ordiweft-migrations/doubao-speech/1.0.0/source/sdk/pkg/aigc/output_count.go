package aigc

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// OutputCountRule bounds one admitted generation, including outputs selected
// by an optional mode. It cannot grant another submission or widen a budget.
type OutputCountRule struct {
	When      map[string]any `json:"when,omitempty"`
	Parameter string         `json:"parameter,omitempty"`
	Maximum   int            `json:"maximum"`
}

func outputField(model Model, path string) (Field, bool) {
	if len(path) > 256 || path == "" {
		return Field{}, false
	}
	fields := model.Fields
	var field Field
	parts := strings.Split(path, ".")
	if len(parts) > 9 {
		return field, false
	}
	for _, part := range parts {
		var ok bool
		field, ok = fields[part]
		if part == "" || !ok {
			return Field{}, false
		}
		fields = field.Properties
	}
	return field, true
}
func validateOutputCount(model Model) error {
	if len(model.OutputCount) > 8 {
		return fmt.Errorf("too many output count rules")
	}
	for _, rule := range model.OutputCount {
		if rule.Maximum < 1 || rule.Maximum > 100 || len(rule.When) > 16 {
			return fmt.Errorf("invalid output count bound")
		}
		if rule.Parameter != "" {
			field, ok := outputField(model, rule.Parameter)
			if !ok || field.Type != "integer" && field.Type != "number" {
				return fmt.Errorf("invalid output count parameter")
			}
		}
		for path, value := range rule.When {
			field, ok := outputField(model, path)
			if !ok || field.Type == "object" || field.Type == "array" || normalizeOutputCondition(field, value) != nil {
				return fmt.Errorf("invalid output count condition")
			}
		}
	}
	return nil
}
func normalizeOutputCondition(field Field, value any) error {
	body, err := json.Marshal(value)
	if err != nil || len(body) > 4096 {
		return fmt.Errorf("invalid condition")
	}
	var normalized any
	if json.Unmarshal(body, &normalized) != nil {
		return fmt.Errorf("invalid condition")
	}
	_, err = normalizeValue(field, normalized, map[string]any{}, "")
	return err
}

// GenerationOutputLimit resolves only trusted model declarations and validated
// original parameters. An inactive/unknown path never invents an output count.
func GenerationOutputLimit(model Model, parameters map[string]any, count int) (int, error) {
	if count < 1 || count > 100 || validateOutputCount(model) != nil {
		return 0, fmt.Errorf("invalid output count")
	}
	if len(model.OutputCount) == 0 {
		return count, nil
	}
	values, err := NormalizeParameters(model, parameters)
	if err != nil {
		return 0, err
	}
	for _, rule := range model.OutputCount {
		if !matchesConditions(rule.When, values) {
			continue
		}
		bound := rule.Maximum
		if rule.Parameter != "" {
			var value any = values
			for _, part := range strings.Split(rule.Parameter, ".") {
				object, ok := value.(map[string]any)
				if !ok {
					return 0, fmt.Errorf("missing output count")
				}
				value = object[part]
			}
			body, err := json.Marshal(value)
			var number float64
			if err != nil || json.Unmarshal(body, &number) != nil || math.IsNaN(number) || number < 1 || number > float64(rule.Maximum) || math.Trunc(number) != number {
				return 0, fmt.Errorf("invalid output count value")
			}
			bound = int(number)
		}
		count = max(count, bound)
	}
	return count, nil
}
