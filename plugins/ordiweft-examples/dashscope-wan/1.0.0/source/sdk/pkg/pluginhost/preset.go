package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
)

// Preset selects a host-owned settings profile; it grants no execution or edit rights.
type Preset struct {
	SchemaVersion int             `json:"schemaVersion"`
	Target        string          `json:"target"`
	Name          string          `json:"name"`
	Settings      json.RawMessage `json:"settings"`
}

func DecodePreset(body []byte) (Preset, error) {
	var out Preset
	if len(body) == 0 || len(body) > 64<<10 {
		return out, ErrContract
	}
	value, err := strictJSON(body)
	if err != nil {
		return out, err
	}
	root, ok := value.(map[string]any)
	if !ok || len(root) != 4 {
		return out, ErrContract
	}
	for _, key := range []string{"schemaVersion", "target", "name", "settings"} {
		if _, ok := root[key]; !ok {
			return out, ErrContract
		}
	}
	if _, ok := root["settings"].(map[string]any); !ok {
		return out, ErrContract
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || out.SchemaVersion != 1 || out.Target != "director-environment" || !policyName(out.Name, 120) {
		return out, ErrContract
	}
	return out, nil
}
