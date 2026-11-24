package pluginhost

import (
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ActionInputRole describes original input data, never asset authority.
type ActionInputRole struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Kinds     []string `json:"kinds"`
	MimeTypes []string `json:"mimeTypes,omitempty"`
	Min       int      `json:"min"`
	Max       int      `json:"max"`
}

func (a ActionSpec) validateInputRoles() error {
	if len(a.InputRoles) > 32 {
		return ErrContract
	}
	seen, minimum, maximum := map[string]bool{}, 0, 0
	for _, role := range a.InputRoles {
		if !aigc.IDPattern.MatchString(role.ID) || seen[role.ID] || !policyName(role.Label, 256) || role.Min < 0 || role.Max < 1 || role.Min > role.Max || role.Max > a.Resources.MaxInputs || len(role.Kinds) == 0 || len(role.Kinds) > 9 || len(role.MimeTypes) > 32 {
			return ErrContract
		}
		seen[role.ID] = true
		kinds, mimes := map[string]bool{}, map[string]bool{}
		for _, kind := range role.Kinds {
			if !validActionInputKind(kind) || kinds[kind] {
				return ErrContract
			}
			kinds[kind] = true
		}
		for _, mime := range role.MimeTypes {
			if !validInputMime(mime) || strings.Contains(mime, "*") || mime != strings.ToLower(mime) || mimes[mime] {
				return ErrContract
			}
			mimes[mime] = true
		}
		minimum += role.Min
		maximum += role.Max
	}
	if len(a.InputRoles) > 0 && (minimum > a.Resources.MaxInputs || maximum < a.Resources.MinInputs) {
		return ErrContract
	}
	return nil
}

// ValidateInputRoles checks fixed host metadata. Byte integrity and current
// permissions remain separate admission and per-read host responsibilities.
func (a ActionSpec) ValidateInputRoles(inputs []RequestInput) error {
	if err := a.validateInputRoles(); err != nil {
		return err
	}
	if len(a.InputRoles) == 0 {
		return nil
	}
	roles := make(map[string]ActionInputRole, len(a.InputRoles))
	counts := make(map[string]int, len(a.InputRoles))
	for _, role := range a.InputRoles {
		roles[role.ID] = role
	}
	for _, input := range inputs {
		role, exists := roles[input.Role]
		if !exists || !slices.Contains(role.Kinds, input.Kind) || len(role.MimeTypes) > 0 && !slices.Contains(role.MimeTypes, input.Mime) {
			return ErrContract
		}
		counts[role.ID]++
	}
	for _, role := range a.InputRoles {
		if counts[role.ID] < role.Min || counts[role.ID] > role.Max {
			return ErrContract
		}
	}
	return nil
}

func validActionInputKind(kind string) bool {
	return slices.Contains([]string{"image", "video", "audio", "voice", "script", "document", "file", "text", "json"}, kind)
}
