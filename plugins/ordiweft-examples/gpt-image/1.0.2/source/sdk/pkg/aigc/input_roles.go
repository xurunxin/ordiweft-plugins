package aigc

import (
	"fmt"
	"sort"
)

// ResolveInputRoles assigns legacy unlabelled edges deterministically before
// excluding inactive slots. Explicit roles always reserve their slots first.
func ResolveInputRoles(model Model, parameters map[string]any, inputs []Input) ([]Input, error) {
	if len(model.InputRoles) == 0 {
		return append([]Input(nil), inputs...), nil
	}
	keys := make([]string, 0, len(model.InputRoles))
	for key := range model.InputRoles {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := model.InputRoles[keys[i]], model.InputRoles[keys[j]]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return keys[i] < keys[j]
	})
	assigned := append([]Input(nil), inputs...)
	used := map[string]bool{}
	for _, in := range assigned {
		if in.Role == "" || in.Role == in.Kind || in.Role == "reference" {
			continue
		}
		role, ok := model.InputRoles[in.Role]
		if !ok || role.Kind != in.Kind || used[in.Role] {
			return nil, fmt.Errorf("invalid or duplicate input role")
		}
		used[in.Role] = true
	}
	for i := range assigned {
		in := &assigned[i]
		if in.Role != "" && in.Role != in.Kind && in.Role != "reference" {
			continue
		}
		found := false
		for _, key := range keys {
			if !used[key] && model.InputRoles[key].Kind == in.Kind {
				in.Role = key
				used[key] = true
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("too many references for input roles")
		}
	}
	out := make([]Input, 0, len(assigned))
	for _, in := range assigned {
		role := model.InputRoles[in.Role]
		if enabled(Field{VisibleWhen: role.VisibleWhen, VisibleWhenAny: role.VisibleWhenAny}, parameters) {
			out = append(out, in)
		}
	}
	for key, role := range model.InputRoles {
		if role.Required && enabled(Field{VisibleWhen: role.VisibleWhen, VisibleWhenAny: role.VisibleWhenAny}, parameters) && !used[key] {
			return nil, fmt.Errorf("required reference: %s", role.Label)
		}
	}
	return out, nil
}
