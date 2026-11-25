package aigc

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"unicode/utf8"
)

func validateFieldSchema(f Field, depth int) error {
	if depth > 0 && len(f.InputModes) > 0 {
		return fmt.Errorf("input mode control must be a model field")
	}
	if err := validateInputModes(f); err != nil {
		return err
	}
	if f.Section != "" && f.Section != "common" && f.Section != "advanced" {
		return fmt.Errorf("invalid parameter section")
	}
	if f.Group != nil && (f.Group.ID == "" || f.Group.Label == "" || len(f.Group.ID) > 256 || len(f.Group.Label) > 512) {
		return fmt.Errorf("invalid parameter group")
	}
	if f.SumMaximum != nil {
		if f.Type != "object" || math.IsNaN(*f.SumMaximum) || math.IsInf(*f.SumMaximum, 0) || *f.SumMaximum < 0 {
			return fmt.Errorf("invalid sum maximum")
		}
		for _, child := range f.Properties {
			if child.Type != "number" && child.Type != "integer" {
				return fmt.Errorf("sum maximum requires numeric properties")
			}
		}
	}
	if f.Icon != "" && f.Icon != "emotion" && f.Icon != "language" {
		return fmt.Errorf("invalid semantic icon")
	}
	if depth > 8 {
		return fmt.Errorf("schema too deep")
	}
	switch f.Type {
	case "string", "number", "integer", "boolean":
	case "object":
		for _, child := range f.Properties {
			if err := validateFieldSchema(child, depth+1); err != nil {
				return err
			}
		}
	case "array":
		if f.Items == nil {
			return fmt.Errorf("array requires items")
		}
		if err := validateFieldSchema(*f.Items, depth+1); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid type")
	}
	switch f.Control {
	case "", "textarea", "slider", "select", "voice", "ratio", "duration", "resolution", "count", "style", "seed":
	default:
		return fmt.Errorf("invalid control")
	}
	if f.Control == "ratio" && f.Type != "string" || (f.Control == "duration" || f.Control == "count" || f.Control == "slider" || f.Control == "seed") && f.Type != "number" && f.Type != "integer" {
		return fmt.Errorf("incompatible control")
	}
	return nil
}

// ValidateDefaults checks every explicitly configured value, including currently
// hidden fields. Required inputs can remain unset until generation admission.
func ValidateDefaults(model Model, defaults map[string]any) error {
	var strip func(Field) Field
	strip = func(f Field) Field {
		f.VisibleWhen = nil
		f.VisibleWhenAny = nil
		f.Required = false
		children := map[string]Field{}
		for k, v := range f.Properties {
			children[k] = strip(v)
		}
		f.Properties = children
		if f.Items != nil {
			v := strip(*f.Items)
			f.Items = &v
		}
		return f
	}
	fields := map[string]Field{}
	for key := range defaults {
		f, ok := model.Fields[key]
		if !ok {
			return fmt.Errorf("unknown default parameter: %s", key)
		}
		fields[key] = strip(f)
	}
	_, e := NormalizeParameters(Model{Fields: fields}, defaults)
	return e
}

// NormalizeParameters applies defaults and drops inactive fields without mutating the saved draft.
// Unknown keys and invalid active values are rejected before any provider is called.
func NormalizeParameters(model Model, parameters map[string]any) (map[string]any, error) {
	b, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if string(b) != "null" {
		if err = json.Unmarshal(b, &values); err != nil {
			return nil, err
		}
	}
	return normalizeObject(model.Fields, values, values, "")
}
func enabled(f Field, root map[string]any) bool {
	if !matchesConditions(f.VisibleWhen, root) {
		return false
	}
	if len(f.VisibleWhenAny) == 0 {
		return true
	}
	for _, conditions := range f.VisibleWhenAny {
		if matchesConditions(conditions, root) {
			return true
		}
	}
	return false
}

func matchesConditions(conditions, root map[string]any) bool {
	for path, want := range conditions {
		var value any = root
		start := 0
		for i := 0; i <= len(path); i++ {
			if i == len(path) || path[i] == '.' {
				m, ok := value.(map[string]any)
				if !ok {
					return false
				}
				value = m[path[start:i]]
				start = i + 1
			}
		}
		a, _ := json.Marshal(value)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			return false
		}
	}
	return true
}
func normalizeObject(fields map[string]Field, values, root map[string]any, prefix string) (map[string]any, error) {
	out := map[string]any{}
	for k := range values {
		if _, ok := fields[k]; !ok {
			return nil, fmt.Errorf("unknown parameter: %s%s", prefix, k)
		}
	}
	// Defaults are available to visibility conditions, including sibling fields.
	for k, f := range fields {
		if _, ok := values[k]; !ok && f.Default != nil {
			b, _ := json.Marshal(f.Default)
			var value any
			_ = json.Unmarshal(b, &value)
			values[k] = value
		}
	}
	for k, f := range fields {
		if !enabled(f, root) {
			continue
		}
		v, ok := values[k]
		if !ok {
			if f.Required {
				return nil, fmt.Errorf("required parameter: %s%s", prefix, k)
			}
			continue
		}
		value, err := normalizeValue(f, v, root, prefix+k)
		if err != nil {
			return nil, err
		}
		out[k] = value
	}
	return out, nil
}
func normalizeValue(f Field, v any, root map[string]any, path string) (any, error) {
	valid := true
	switch f.Type {
	case "string":
		s, ok := v.(string)
		valid = ok && (f.MaxLength <= 0 || utf8.RuneCountInString(s) <= f.MaxLength)
	case "boolean":
		_, valid = v.(bool)
	case "number", "integer":
		n, ok := v.(float64)
		valid = ok && !math.IsNaN(n) && !math.IsInf(n, 0) && (f.Type != "integer" || n == math.Trunc(n)) && (f.Minimum == nil || n >= *f.Minimum) && (f.Maximum == nil || n <= *f.Maximum)
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			valid = false
			break
		}
		out, err := normalizeObject(f.Properties, m, root, path+".")
		if err != nil {
			return nil, err
		}
		if f.SumMaximum != nil {
			sum := 0.0
			for _, value := range out {
				sum += value.(float64)
			}
			if sum > *f.SumMaximum+0.000001 {
				return nil, fmt.Errorf("parameter sum exceeds maximum: %s", path)
			}
		}
		return out, nil
	case "array":
		a, ok := v.([]any)
		if !ok || f.Items == nil || len(a) > 256 {
			valid = false
			break
		}
		out := make([]any, len(a))
		for i, item := range a {
			n, err := normalizeValue(*f.Items, item, root, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	default:
		valid = false
	}
	if !valid {
		return nil, fmt.Errorf("invalid parameter: %s", path)
	}
	if len(f.Enum) > 0 {
		found := false
		for _, candidate := range f.Enum {
			a, _ := json.Marshal(candidate)
			b, _ := json.Marshal(v)
			found = found || reflect.DeepEqual(a, b)
		}
		if !found {
			return nil, fmt.Errorf("invalid parameter choice: %s", path)
		}
	}
	return v, nil
}
