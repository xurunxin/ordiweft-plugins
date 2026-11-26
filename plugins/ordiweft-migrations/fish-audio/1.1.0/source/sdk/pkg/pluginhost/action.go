package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ActionStart is a private host-created invocation for a non-generation port.
// An argument is data; it cannot select an identity, endpoint or effect budget.
type ActionStart struct {
	HostProtocol  int             `json:"hostProtocol"`
	Kind          string          `json:"kind"`
	PortID        string          `json:"portId"`
	PortKind      string          `json:"portKind"`
	Arguments     json.RawMessage `json:"arguments"`
	Inputs        []ActionInput   `json:"inputs"`
	Compatibility *HostOffer      `json:"compatibility,omitempty"`
}

type ActionInput struct {
	Kind  string      `json:"kind"`
	Role  string      `json:"role"`
	Asset AssetHandle `json:"asset"`
}

func (input ActionInput) Validate() error {
	if !validActionInputKind(input.Kind) || len(input.Role) > 128 || strings.ContainsAny(input.Role, "\x00\r\n") || input.Asset.Validate() != nil {
		return ErrContract
	}
	return nil
}

// ValidActionReadRequirements is a supported offline profile, not an
// authorization decision. The host separately binds current asset authority.
func ValidActionReadRequirements(requirements []Requirement) bool {
	return validActionRequirements(requirements, false)
}

// ValidActionRequirements describes the offline read/output profile. A trusted
// host still has to install each requested implementation and resource policy.
func ValidActionRequirements(requirements []Requirement) bool {
	return validActionRequirements(requirements, true)
}

func validActionRequirements(requirements []Requirement, outputs bool) bool {
	for _, requirement := range requirements {
		if requirement.Version != Version || len(requirement.Operations) == 0 {
			return false
		}
		for _, operation := range requirement.Operations {
			if !(requirement.Service == "assets" && slices.Contains([]string{"describe", "read", "release"}, operation) || requirement.Service == "media" && operation == "inspect" || outputs && requirement.Service == "outputs" && slices.Contains([]string{"begin", "write", "stage", "stage-buffer"}, operation)) {
				return false
			}
		}
	}
	return true
}

func DecodeActionStart(body []byte) (ActionStart, error) {
	var out ActionStart
	if len(body) == 0 || len(body) > MaxControlBytes {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Validate() != nil {
		return out, ErrContract
	}
	return out, nil
}

func (s ActionStart) Validate() error {
	if s.Compatibility != nil && s.Compatibility.Validate() != nil {
		return ErrContract
	}
	if s.HostProtocol != Version || s.Kind != "invoke" || !aigc.IDPattern.MatchString(s.PortID) || !slices.Contains([]string{"processor", "worker"}, s.PortKind) || s.Inputs == nil || len(s.Inputs) > MaxTaskInputs || !actionObject(s.Arguments) {
		return ErrContract
	}
	var total int64
	for _, input := range s.Inputs {
		if input.Validate() != nil {
			return ErrContract
		}
		total += input.Asset.Bytes
		if total > MaxTaskRequestBytes {
			return ErrContract
		}
	}
	body, err := json.Marshal(s)
	if err != nil || len(body) > MaxControlBytes {
		return ErrContract
	}
	return nil
}

type ActionCompletion struct {
	HostProtocol int             `json:"hostProtocol"`
	Kind         string          `json:"kind"`
	Outputs      []string        `json:"outputs"`
	Value        json.RawMessage `json:"value"`
}

func DecodeActionCompletion(body []byte) (ActionCompletion, error) {
	var out ActionCompletion
	if len(body) == 0 || len(body) > MaxControlBytes {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.HostProtocol != Version || out.Kind != "action-complete" || out.Outputs == nil || len(out.Outputs) > MaxTaskOutputs || !actionObject(out.Value) {
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

func actionObject(body []byte) bool {
	if len(body) == 0 || len(body) > 64<<10 {
		return false
	}
	value, err := strictJSON(body)
	_, ok := value.(map[string]any)
	return err == nil && ok
}
