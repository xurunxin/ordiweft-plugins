// SPDX-License-Identifier: Apache-2.0
package pluginhost

import (
	"encoding/json"
	"strings"
)

type requestPromptSuffix struct {
	conditions []map[string][]json.RawMessage
	text       string
}

func requestSourceConditions(raw any) ([]map[string][]json.RawMessage, error) {
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, ErrContract
	}
	var groups []map[string][]json.RawMessage
	if json.Unmarshal(body, &groups) != nil || validateRequestConditions(groups) != nil {
		return nil, ErrContract
	}
	return groups, nil
}

func parseParameterMap(value map[string]any, out *requestSource) error {
	parameters, ok := value["parameters"].([]any)
	if !ok || len(parameters) == 0 || len(parameters) > 8 {
		return ErrContract
	}
	seen := map[string]bool{}
	for _, raw := range parameters {
		name, ok := raw.(string)
		if !ok || !policyName(name, 128) || seen[name] {
			return ErrContract
		}
		seen[name] = true
		out.parameters = append(out.parameters, name)
	}
	entries, ok := value["cases"].([]any)
	if !ok || len(entries) == 0 || len(entries) > 256 {
		return ErrContract
	}
	out.parameterMap = make(map[string]string, len(entries))
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok || len(entry) != 2 {
			return ErrContract
		}
		from, ok := entry["from"].([]any)
		to, valid := entry["to"].(string)
		if !ok || !valid || len(to) > 4096 || len(from) != len(out.parameters) {
			return ErrContract
		}
		keys := make([]string, len(from))
		for i, raw := range from {
			text, ok := raw.(string)
			if !ok || !validParameterMapKey(text) {
				return ErrContract
			}
			keys[i] = text
		}
		key := parameterMapKey(keys)
		if _, present := out.parameterMap[key]; present {
			return ErrContract
		}
		out.parameterMap[key] = to
	}
	if raw, present := value["whenAny"]; present {
		groups, err := requestSourceConditions(raw)
		if err != nil {
			return err
		}
		out.conditions = groups
	}
	if raw, present := value["fallbackParameter"]; present {
		name, ok := raw.(string)
		if !ok || !policyName(name, 128) {
			return ErrContract
		}
		out.fallbackParameter = name
	}
	return nil
}

func validParameterMapKey(text string) bool {
	return len(text) <= 1024 && !strings.ContainsAny(text, "\x00\r\n")
}

func parameterMapKey(keys []string) string {
	// A JSON tuple keeps punctuation, empty strings and Unicode unambiguous.
	body, _ := json.Marshal(keys)
	return string(body)
}

func renderParameterMap(source requestSource, values JSONRequestValues) (any, bool, error) {
	if !matchesRequestConditions(source.conditions, values) {
		if source.fallbackParameter == "" {
			return nil, false, nil
		}
		value, present := values.Generation.Parameters[source.fallbackParameter]
		if !present {
			return nil, false, nil
		}
		text, ok := value.(string)
		if !ok || len(text) > 4096 {
			return nil, false, ErrContract
		}
		return text, true, nil
	}
	keys := make([]string, len(source.parameters))
	for i, name := range source.parameters {
		text, ok := values.Generation.Parameters[name].(string)
		if !ok || !validParameterMapKey(text) {
			return nil, false, ErrContract
		}
		keys[i] = text
	}
	result, present := source.parameterMap[parameterMapKey(keys)]
	if !present {
		return nil, false, ErrContract
	}
	return result, true, nil
}

func parsePromptSuffixes(raw any) ([]requestPromptSuffix, error) {
	entries, ok := raw.([]any)
	if !ok || len(entries) > 8 {
		return nil, ErrContract
	}
	out := make([]requestPromptSuffix, 0, len(entries))
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrContract
		}
		for name := range entry {
			if name != "text" && name != "whenAny" {
				return nil, ErrContract
			}
		}
		text, ok := entry["text"].(string)
		if !ok || len(text) > 8192 {
			return nil, ErrContract
		}
		var groups []map[string][]json.RawMessage
		if raw, present := entry["whenAny"]; present {
			var err error
			groups, err = requestSourceConditions(raw)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, requestPromptSuffix{conditions: groups, text: text})
	}
	return out, nil
}

func renderRequestPrompt(source requestSource, values JSONRequestValues) string {
	if len(source.suffixes) == 0 {
		return values.Generation.Prompt
	}
	var out strings.Builder
	out.WriteString(values.Generation.Prompt)
	for _, suffix := range source.suffixes {
		if matchesRequestConditions(suffix.conditions, values) {
			out.WriteString(suffix.text)
		}
	}
	return out.String()
}
