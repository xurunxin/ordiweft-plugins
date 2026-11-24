package pluginhost

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// AllowsEffect constrains where a reviewed request mapping can be installed.
// It does not grant that effect or replace the original task's authorization.
func (p JSONRequestPolicy) AllowsEffect(effect string) bool {
	return (!p.QueryOnly || effect == "query") && (!p.UploadOnly || effect == "upload")
}

func canonicalRequestConditions(groups []map[string][]json.RawMessage) []map[string][]json.RawMessage {
	if len(groups) == 0 {
		return nil
	}
	result := make([]map[string][]json.RawMessage, len(groups))
	for i, group := range groups {
		result[i] = make(map[string][]json.RawMessage, len(group))
		for pointer, values := range group {
			for _, raw := range values {
				value, _ := strictJSON(raw)
				canonical, _ := json.Marshal(value)
				result[i][pointer] = append(result[i][pointer], canonical)
			}
		}
	}
	return result
}

func validateRequestConditions(groups []map[string][]json.RawMessage) error {
	if len(groups) > 16 {
		return ErrContract
	}
	for _, group := range groups {
		if len(group) == 0 || len(group) > 16 {
			return ErrContract
		}
		for pointer, values := range group {
			parts, err := pointerParts(pointer)
			if err != nil || len(parts) == 0 || len(pointer) > 256 || !originalConditionPointer(pointer, parts) || len(values) == 0 || len(values) > 16 {
				return ErrContract
			}
			seen := map[string]bool{}
			for _, raw := range values {
				if len(raw) == 0 || len(raw) > 2048 {
					return ErrContract
				}
				value, err := strictJSON(raw)
				switch v := value.(type) {
				case string:
					if len(v) > 1024 || strings.ContainsAny(v, "\x00\r\n") {
						return ErrContract
					}
				case bool, json.Number:
				default:
					return ErrContract
				}
				canonical, encodeErr := json.Marshal(value)
				if err != nil || encodeErr != nil || seen[string(canonical)] {
					return ErrContract
				}
				seen[string(canonical)] = true
			}
		}
	}
	return nil
}

func originalConditionPointer(pointer string, parts []string) bool {
	if pointer == "/model" || pointer == "/inputCount" || pointer == "/count" || len(parts) > 1 && parts[0] == "parameters" {
		return true
	}
	if len(parts) != 3 || parts[0] != "inputs" {
		return false
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil || index < 0 || index >= 256 || strconv.Itoa(index) != parts[1] {
		return false
	}
	switch parts[2] {
	case "kind", "role", "mime", "bytes", "sha256":
		return true
	}
	return false
}

func matchesRequestConditions(groups []map[string][]json.RawMessage, values JSONRequestValues) bool {
	if len(groups) == 0 {
		return true
	}
	// Re-decode with the same strict number representation used by request
	// rendering, without an additional conversion of precise JSON numbers.
	inputs := make([]map[string]any, len(values.Inputs))
	for i, input := range values.Inputs {
		inputs[i] = map[string]any{"kind": input.Kind, "role": input.Role, "mime": input.Mime, "bytes": input.Bytes, "sha256": input.SHA256}
	}
	raw, err := json.Marshal(map[string]any{"model": values.Generation.Model, "parameters": values.Generation.Parameters, "inputCount": len(values.Inputs), "count": values.Count, "inputs": inputs})
	if err != nil {
		return false
	}
	original, err := strictJSON(raw)
	if err != nil {
		return false
	}
	for _, group := range groups {
		matched := true
		for pointer, allowed := range group {
			value, present, err := pointerValue(original, pointer)
			if err != nil || !present {
				matched = false
				break
			}
			actual, err := json.Marshal(value)
			found := false
			for _, raw := range allowed {
				candidate, decodeErr := strictJSON(raw)
				canonical, encodeErr := json.Marshal(candidate)
				if err == nil && decodeErr == nil && encodeErr == nil && bytes.Equal(actual, canonical) {
					found = true
					break
				}
			}
			if !found {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
