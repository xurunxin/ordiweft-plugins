package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ExtensionDefinition describes a port. It cannot install a handler or grant
// access to identities, queues, credentials, host files or network endpoints.
type ExtensionDefinition struct {
	Version int          `json:"version"`
	ID      string       `json:"id"`
	Kind    string       `json:"kind"`
	Content *ContentSpec `json:"content,omitempty"`
	Node    *NodeSpec    `json:"node,omitempty"`
	Action  *ActionSpec  `json:"action,omitempty"`
}

type ContentSpec struct {
	Type  string `json:"type"`
	Entry string `json:"entry"`
}

type DefinitionPort struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	DataType  string `json:"dataType"`
}

type NodeSpec struct {
	Label        string           `json:"label"`
	Version      int              `json:"version"`
	Renderer     string           `json:"renderer"`
	FieldsSchema json.RawMessage  `json:"fieldsSchema"`
	Defaults     json.RawMessage  `json:"defaults"`
	Ports        []DefinitionPort `json:"ports"`
	Actions      []string         `json:"actions"`
}

type ActionSpec struct {
	Label        string            `json:"label"`
	InputSchema  json.RawMessage   `json:"inputSchema"`
	ResultSchema json.RawMessage   `json:"resultSchema"`
	Resources    ActionResources   `json:"resources"`
	InputRoles   []ActionInputRole `json:"inputRoles,omitempty"`
}

// These are requests; installation and admission impose the actual limits.
type ActionResources struct {
	MinInputs      int   `json:"minInputs,omitempty"`
	MinOutputs     int   `json:"minOutputs,omitempty"`
	MaxInputs      int   `json:"maxInputs"`
	MaxOutputs     int   `json:"maxOutputs"`
	MaxInputBytes  int64 `json:"maxInputBytes"`
	MaxOutputBytes int64 `json:"maxOutputBytes"`
	MaxRuntimeMS   int64 `json:"maxRuntimeMs"`
}

// NodeType is a stable derived name, not proof of namespace ownership.
func NodeType(packageID, contributionID string) (string, error) {
	if !ValidPackageID(packageID) || !aigc.IDPattern.MatchString(contributionID) {
		return "", ErrContract
	}
	h := sha256.Sum256([]byte(packageID + "\n" + contributionID))
	return "p-" + hex.EncodeToString(h[:])[:62], nil
}

func DecodeExtensionDefinition(body []byte, descriptor Descriptor, contribution Contribution) (ExtensionDefinition, error) {
	var out ExtensionDefinition
	member := false
	for _, item := range descriptor.Contributions {
		if item == contribution {
			member = true
		}
	}
	if !member || len(body) == 0 || len(body) > 64<<10 {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Version != Version || out.ID != contribution.ID || out.Kind != contribution.Kind {
		return out, ErrContract
	}
	listed := func(path string) bool {
		for _, file := range descriptor.Files {
			if file.Path == path {
				return true
			}
		}
		return false
	}
	switch out.Kind {
	case "content":
		if out.Content == nil || out.Node != nil || out.Action != nil || !slices.Contains([]string{"json", "prompt-template", "agent-skill", "preset", "asset-bundle"}, out.Content.Type) || !ValidPath(out.Content.Entry) || !listed(out.Content.Entry) {
			return out, ErrContract
		}
		for _, definition := range descriptor.Definitions {
			if definition == out.Content.Entry {
				return out, ErrContract
			}
		}
	case "node":
		if out.Content != nil || out.Action != nil || out.Node == nil || out.Node.Validate(descriptor) != nil {
			return out, ErrContract
		}
	case "processor", "worker":
		if out.Content != nil || out.Node != nil || out.Action == nil || out.Action.Validate() != nil {
			return out, ErrContract
		}
	default:
		return out, ErrContract
	}
	return out, nil
}

func (n NodeSpec) Validate(d Descriptor) error {
	if !policyName(n.Label, 256) || n.Version < 1 || n.Version > 1<<31-1 || n.Renderer != "form" || closedSchema(n.FieldsSchema) != nil || len(n.Ports) > 32 || len(n.Actions) > 32 {
		return ErrContract
	}
	defaults, err := strictJSON(n.Defaults)
	fields, ok := defaults.(map[string]any)
	if err != nil || !ok {
		return ErrContract
	}
	for field := range fields {
		if ProtectedNodeField(field) {
			return ErrContract
		}
	}
	schema, _ := strictJSON(n.FieldsSchema)
	root := schema.(map[string]any)
	properties, _ := root["properties"].(map[string]any)
	for field := range properties {
		if ProtectedNodeField(field) {
			return ErrContract
		}
	}
	seen := map[string]bool{}
	for _, port := range n.Ports {
		if !aigc.IDPattern.MatchString(port.ID) || seen[port.ID] || !slices.Contains([]string{"input", "output"}, port.Direction) || !slices.Contains([]string{"text", "image", "video", "audio", "voice", "script", "document", "json", "file"}, port.DataType) {
			return ErrContract
		}
		seen[port.ID] = true
	}
	seen = map[string]bool{}
	for _, action := range n.Actions {
		found := false
		for _, contribution := range d.Contributions {
			if contribution.ID == action && slices.Contains([]string{"provider", "processor", "worker"}, contribution.Kind) {
				found = true
			}
		}
		if !found || seen[action] {
			return ErrContract
		}
		seen[action] = true
	}
	return nil
}

func (a ActionSpec) Validate() error {
	r := a.Resources
	if r.MinInputs < 0 || r.MinInputs > r.MaxInputs || r.MinOutputs < 0 || r.MinOutputs > r.MaxOutputs {
		return ErrContract
	}
	if !policyName(a.Label, 256) || closedSchema(a.InputSchema) != nil || closedSchema(a.ResultSchema) != nil || r.MaxInputs < 0 || r.MaxInputs > MaxTaskInputs || r.MaxOutputs < 0 || r.MaxOutputs > MaxTaskOutputs || r.MaxInputBytes < 0 || r.MaxInputBytes > MaxTaskRequestBytes || r.MaxOutputBytes < 1 || r.MaxOutputBytes > MaxTaskResponseBytes || r.MaxRuntimeMS < 1 || r.MaxRuntimeMS > MaxTaskRuntimeMS {
		return ErrContract
	}
	return a.validateInputRoles()
}

func closedSchema(raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return ErrContract
	}
	value, err := strictJSON(raw)
	root, ok := value.(map[string]any)
	if err != nil || !ok || root["type"] != "object" || root["additionalProperties"] != false {
		return ErrContract
	}
	remaining := 4096
	var walk func(any, int) bool
	walk = func(value any, depth int) bool {
		remaining--
		if depth > 32 || remaining < 0 {
			return false
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if strings.HasPrefix(key, "$") || !walk(child, depth+1) {
					return false
				}
			}
		case []any:
			for _, child := range v {
				if !walk(child, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !walk(root, 0) {
		return ErrContract
	}
	return nil
}

var protectedNodeFields = map[string]bool{"id": true, "type": true, "updatedAt": true, "deletedAt": true, "locked": true, "collection": true, "policy": true, "policySources": true, "versions": true, "activeVersion": true, "sequenceId": true, "stageId": true, "storyboardId": true, "scriptId": true, "shotId": true, "settingId": true, "owner": true, "team": true, "role": true, "scope": true, "credentials": true, "apiKey": true, "secret": true, "secretEnv": true, "budget": true, "stopEpoch": true, "planHash": true, "approval": true, "jobId": true}

func ProtectedNodeField(name string) bool {
	return name == "pluginDefinition" || protectedNodeFields[name]
}

// DecodeDefinitions validates only declared port metadata. It does not compile
// application schemas, activate consumers, or authorize an execution runtime.
func DecodeDefinitions(d Descriptor, files map[string][]byte) (map[string]ExtensionDefinition, error) {
	out := map[string]ExtensionDefinition{}
	if d.Validate() != nil {
		return nil, ErrContract
	}
	for _, contribution := range d.Contributions {
		name, exists := d.Definitions[contribution.ID]
		if !exists {
			continue
		}
		definition, err := DecodeExtensionDefinition(files[name], d, contribution)
		if err != nil {
			return nil, err
		}
		out[contribution.ID] = definition
	}
	return out, nil
}
