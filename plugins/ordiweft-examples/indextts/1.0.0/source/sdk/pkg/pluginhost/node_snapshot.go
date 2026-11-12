package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
)

// FormNodeSnapshot pins the original form schema for drafts and history.
// A valid snapshot is metadata; installation and business writes still require
// a matching immutable host record and the caller's current document authority.
type FormNodeSnapshot struct {
	Version          int                 `json:"version"`
	PackageID        string              `json:"packageId"`
	ReleaseDigest    string              `json:"releaseDigest"`
	Type             string              `json:"type"`
	DefinitionDigest string              `json:"definitionDigest"`
	Definition       ExtensionDefinition `json:"definition"`
}

func IsPluginNodeType(kind string) bool {
	return len(kind) == 64 && kind[:2] == "p-" && ValidDigest("00"+kind[2:])
}

func NewFormNodeSnapshot(release string, d Descriptor, c Contribution, body []byte) (FormNodeSnapshot, error) {
	var out FormNodeSnapshot
	digest, err := d.Digest()
	if err != nil || digest != release || c.Kind != "node" {
		return out, ErrContract
	}
	path := d.Definitions[c.ID]
	sum := sha256.Sum256(body)
	matched := false
	for _, file := range d.Files {
		if path != "" && file.Path == path && file.Bytes == int64(len(body)) && file.SHA256 == hex.EncodeToString(sum[:]) {
			matched = true
		}
	}
	if !matched {
		return out, ErrContract
	}
	definition, err := DecodeExtensionDefinition(body, d, c)
	if err != nil || definition.Node == nil {
		return out, ErrContract
	}
	typeName, err := NodeType(d.PackageID, c.ID)
	if err != nil {
		return out, err
	}
	canonical, err := canonicalFormDefinition(definition)
	if err != nil {
		return out, err
	}
	sum = sha256.Sum256(canonical)
	if json.Unmarshal(canonical, &definition) != nil {
		return out, ErrContract
	}
	out = FormNodeSnapshot{Version: Version, PackageID: d.PackageID, ReleaseDigest: release, Type: typeName, DefinitionDigest: hex.EncodeToString(sum[:]), Definition: definition}
	return out, out.ValidateDescriptor(d)
}

// ValidateDescriptor checks the pinned metadata profile, not installation trust.
func (s FormNodeSnapshot) ValidateDescriptor(d Descriptor) error {
	digest, err := d.Digest()
	if s.Validate() != nil || err != nil || digest != s.ReleaseDigest || d.PackageID != s.PackageID || d.Generation != nil || d.Definitions[s.Definition.ID] == "" {
		return ErrContract
	}
	declared := false
	for _, contribution := range d.Contributions {
		if contribution.ID == s.Definition.ID && contribution.Kind == "node" {
			declared = true
		}
	}
	if !declared {
		return ErrContract
	}
	actions := s.Definition.Node.Actions
	if d.Runtime == "data" && len(d.Requirements) == 0 && len(actions) == 0 {
		return nil
	}
	if !ValidIsolatedActionDescriptor(d) || len(actions) != 1 || actions[0] != d.Contributions[0].ID {
		return ErrContract
	}
	return nil
}

func DecodeFormNodeSnapshot(body []byte) (FormNodeSnapshot, error) {
	var out FormNodeSnapshot
	if len(body) == 0 || len(body) > MaxControlBytes {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || out.Validate() != nil {
		return FormNodeSnapshot{}, ErrContract
	}
	canonical, err := canonicalFormDefinition(out.Definition)
	if err != nil || json.Unmarshal(canonical, &out.Definition) != nil {
		return FormNodeSnapshot{}, ErrContract
	}
	return out, nil
}

func (s FormNodeSnapshot) Validate() error {
	d := s.Definition
	if s.Version != Version || !ValidPackageID(s.PackageID) || !ValidDigest(s.ReleaseDigest) || !ValidDigest(s.DefinitionDigest) || d.Version != Version || d.Kind != "node" || d.Node == nil || d.Content != nil || d.Action != nil || len(d.Node.Actions) > 1 {
		return ErrContract
	}
	contributions := []Contribution{{ID: d.ID, Kind: "node"}}
	if len(d.Node.Actions) == 1 {
		if d.Node.Actions[0] == d.ID || len(d.Node.Ports) != 0 {
			return ErrContract
		}
		contributions = append(contributions, Contribution{ID: d.Node.Actions[0], Kind: "processor"})
	}
	kind, err := NodeType(s.PackageID, d.ID)
	if err != nil || kind != s.Type || d.Node.Validate(Descriptor{Contributions: contributions}) != nil {
		return ErrContract
	}
	body, err := canonicalFormDefinition(d)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != s.DefinitionDigest {
		return ErrContract
	}
	return nil
}

func canonicalFormDefinition(d ExtensionDefinition) ([]byte, error) {
	body, err := json.Marshal(d)
	if err != nil || len(body) > 64<<10 {
		return nil, ErrContract
	}
	value, err := strictJSON(body)
	if err != nil {
		return nil, err
	}
	var normalize func(any) (any, error)
	normalize = func(v any) (any, error) {
		switch item := v.(type) {
		case json.Number:
			n, err := item.Float64()
			if err != nil || math.Abs(n) > 9007199254740991 {
				return nil, ErrContract
			}
			return n, nil
		case map[string]any:
			for key, child := range item {
				item[key], err = normalize(child)
				if err != nil {
					return nil, err
				}
			}
		case []any:
			for index, child := range item {
				item[index], err = normalize(child)
				if err != nil {
					return nil, err
				}
			}
		}
		return v, nil
	}
	value, err = normalize(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
