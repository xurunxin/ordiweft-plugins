package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type TaskInput struct {
	Kind  string      `json:"kind"`
	Role  string      `json:"role"`
	Asset AssetHandle `json:"asset"`
}

const MaxTaskInputs = 80   // same accepted reference count as the existing AIGC SDK
const MaxTaskOutputs = 100 // existing host output-count admission ceiling

func (input TaskInput) Validate() error {
	if !slices.Contains([]string{"image", "video", "audio", "voice"}, input.Kind) || len(input.Role) > 128 || strings.ContainsAny(input.Role, "\x00\r\n") || input.Asset.Validate() != nil {
		return ErrContract
	}
	return nil
}

// TaskStart deliberately excludes identity, credentials, URLs and raw media.
type TaskStart struct {
	HostProtocol int             `json:"hostProtocol"`
	APIVersion   int             `json:"apiVersion"`
	Generation   aigc.Generation `json:"generation"`
	Inputs       []TaskInput     `json:"inputs"`
}

func DecodeTaskStart(body []byte) (TaskStart, error) {
	var out TaskStart
	if len(body) == 0 || len(body) > MaxControlBytes {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Validate() != nil {
		return TaskStart{}, ErrContract
	}
	return out, nil
}

func (s TaskStart) Validate() error {
	if s.HostProtocol != Version || !aigc.SupportedVersion(s.APIVersion) || len(s.Inputs) > MaxTaskInputs || !aigc.IDPattern.MatchString(s.Generation.Provider) || s.Generation.Model == "" {
		return ErrContract
	}
	for _, input := range s.Inputs {
		if input.Validate() != nil {
			return ErrContract
		}
	}
	body, err := json.Marshal(s)
	if err != nil || len(body) > MaxControlBytes {
		return ErrContract
	}
	return nil
}

type TaskCompletion struct {
	HostProtocol int             `json:"hostProtocol"`
	Kind         string          `json:"kind"`
	Outputs      []string        `json:"outputs"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

func DecodeTaskCompletion(body []byte) (TaskCompletion, error) {
	var out TaskCompletion
	if len(body) == 0 || len(body) > MaxControlBytes {
		return out, ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.HostProtocol != Version || out.Kind != "complete" || out.Outputs == nil || len(out.Outputs) > MaxTaskOutputs || len(out.Metadata) > 64<<10 || len(out.Metadata) > 0 && out.Metadata[0] != '{' {
		return out, ErrContract
	}
	seen := map[string]bool{}
	for _, id := range out.Outputs {
		if !opaqueID.MatchString(id) || seen[id] {
			return out, ErrContract
		}
		seen[id] = true
	}
	return out, nil
}
