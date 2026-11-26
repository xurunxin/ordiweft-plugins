package aigc

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const MaxPluginOptionBytes = 128 << 10

// OptionSchema describes non-secret, administrator-controlled adapter options.
// The host owns credentials, scheduling, costs and the network policy separately.
type OptionSchema struct {
	Version int           `json:"version"`
	Groups  []OptionGroup `json:"groups"`
}

type OptionGroup struct {
	ID     string           `json:"id"`
	Label  string           `json:"label"`
	Fields map[string]Field `json:"fields"`
}

func (s *OptionSchema) Validate() error {
	if s.Version != 1 || len(s.Groups) < 1 || len(s.Groups) > 8 {
		return fmt.Errorf("invalid plugin options schema")
	}
	groups, fields := map[string]bool{}, map[string]bool{}
	for _, group := range s.Groups {
		if !IDPattern.MatchString(group.ID) || groups[group.ID] || !optionText(group.Label, 120) || len(group.Fields) == 0 {
			return fmt.Errorf("invalid plugin option group")
		}
		groups[group.ID] = true
		for id, field := range group.Fields {
			if !IDPattern.MatchString(id) || fields[id] || !validOptionField(field) || !validOptionConditions(id, field, group.Fields) {
				return fmt.Errorf("invalid plugin option: %s", id)
			}
			fields[id] = true
		}
	}
	if len(fields) > 48 {
		return fmt.Errorf("too many plugin options")
	}
	return nil
}

func optionText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len([]rune(value)) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func validOptionField(f Field) bool {
	if !slices.Contains([]string{"boolean", "integer", "number", "string"}, f.Type) ||
		!optionText(f.Label, 120) || f.Description != "" && !optionText(f.Description, 512) ||
		f.Default == nil || len(f.Enum) > 32 || f.Type == "string" && (f.MaxLength < 1 || f.MaxLength > 512) ||
		!slices.Contains([]string{"", "select", "textarea", "slider"}, f.Control) ||
		f.Control == "select" && len(f.Enum) == 0 || f.Control == "textarea" && f.Type != "string" ||
		f.Control == "slider" && (f.Minimum == nil || f.Maximum == nil) ||
		f.Minimum != nil && f.Maximum != nil && *f.Minimum > *f.Maximum ||
		f.Step != nil && (*f.Step <= 0 || math.IsNaN(*f.Step) || math.IsInf(*f.Step, 0)) ||
		len(f.InputModes) != 0 || len(f.Properties) != 0 || f.Items != nil || f.Group != nil ||
		len(f.VisibleWhenAny) > 8 || validateFieldSchema(f, 0) != nil {
		return false
	}
	if !ValidOption(f, f.Default) {
		return false
	}
	for _, value := range f.Enum {
		if !ValidOption(f, value) {
			return false
		}
	}
	return true
}

func validOptionConditions(id string, field Field, group map[string]Field) bool {
	conditions := append([]map[string]any{field.VisibleWhen}, field.VisibleWhenAny...)
	for _, condition := range conditions {
		for dependency, value := range condition {
			parent, ok := group[dependency]
			if !ok || dependency == id || len(parent.VisibleWhen) != 0 || len(parent.VisibleWhenAny) != 0 || !ValidOption(parent, value) {
				return false
			}
		}
	}
	return true
}

func ValidOption(field Field, value any) bool {
	if value == nil {
		return false
	}
	return ValidateDefaults(Model{Fields: map[string]Field{"option": field}}, map[string]any{"option": value}) == nil
}

func (s *OptionSchema) Fields() map[string]Field {
	fields := map[string]Field{}
	if s != nil {
		for _, group := range s.Groups {
			for id, field := range group.Fields {
				fields[id] = field
			}
		}
	}
	return fields
}

// Effective validates saved overrides even when their controls are hidden.
// Only currently visible values are included in the adapter request.
func (s *OptionSchema) Effective(overrides map[string]any) (map[string]any, error) {
	fields := s.Fields()
	for id, value := range overrides {
		if field, ok := fields[id]; !ok || !ValidOption(field, value) {
			return nil, fmt.Errorf("invalid plugin option: %s", id)
		}
	}
	return NormalizeParameters(Model{Fields: fields}, overrides)
}
