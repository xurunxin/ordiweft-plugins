package tapnowweb

import (
	"fmt"
	"reflect"
)

// Declarative option expressions are data, never JavaScript execution.
func evalRule(v any, p map[string]any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if key := str(m, "parameter"); key != "" {
		return p[key]
	}
	if test, ok := m["if"]; ok {
		if truth(evalRule(test, p)) {
			return evalRule(m["then"], p)
		}
		return evalRule(m["else"], p)
	}
	a, b := evalRule(m["left"], p), evalRule(m["right"], p)
	switch str(m, "op") {
	case "===":
		return reflect.DeepEqual(a, b)
	case "!==":
		return !reflect.DeepEqual(a, b)
	case "||":
		return truth(a) || truth(b)
	case "&&":
		return truth(a) && truth(b)
	case "!":
		return !truth(a)
	case ">", ">=", "<", "<=":
		x, xok := a.(float64)
		y, yok := b.(float64)
		if !xok || !yok {
			return nil
		}
		switch str(m, "op") {
		case ">":
			return x > y
		case ">=":
			return x >= y
		case "<":
			return x < y
		default:
			return x <= y
		}
	}
	return nil
}
func truth(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	default:
		return v != nil
	}
}
func ruleChoices(v any) []any {
	out := []any{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(x any) {
		switch x := x.(type) {
		case []any:
			for _, v := range x {
				key := fmt.Sprintf("%T:%v", v, v)
				if !seen[key] {
					seen[key] = true
					out = append(out, v)
				}
			}
		case map[string]any:
			walk(x["then"])
			walk(x["else"])
		}
	}
	walk(v)
	return out
}
