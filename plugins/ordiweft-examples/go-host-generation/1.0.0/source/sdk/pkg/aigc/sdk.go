// Package aigc is the versioned, transport-independent channel SDK.
package aigc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const APIVersion = 2

func SupportedVersion(v int) bool { return v == 1 || v == 2 }

type Field struct {
	Section        string            `json:"section,omitempty"`
	Group          *FieldGroup       `json:"group,omitempty"`
	SumMaximum     *float64          `json:"sumMaximum,omitempty"`
	OptionLabels   map[string]string `json:"optionLabels,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	Description    string            `json:"description,omitempty"`
	Placement      string            `json:"placement,omitempty"`
	Properties     map[string]Field  `json:"properties,omitempty"`
	Items          *Field            `json:"items,omitempty"`
	MaxLength      int               `json:"maxLength,omitempty"`
	Step           *float64          `json:"step,omitempty"`
	VisibleWhen    map[string]any    `json:"visibleWhen,omitempty"`
	VisibleWhenAny []map[string]any  `json:"visibleWhenAny,omitempty"`
	Type           string            `json:"type"`
	Label          string            `json:"label"`
	Control        string            `json:"control,omitempty"` // original host control: ratio, duration, count
	Default        any               `json:"default,omitempty"`
	Enum           []any             `json:"enum,omitempty"`
	Required       bool              `json:"required,omitempty"`
	Minimum        *float64          `json:"minimum,omitempty"`
	Maximum        *float64          `json:"maximum,omitempty"`
}
type FieldGroup struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type PromptSpec struct {
	Parameter   string `json:"parameter,omitempty"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required"`
	MaxLength   int    `json:"maxLength,omitempty"`
}
type Model struct {
	MaxVoices   int                  `json:"maxVoices,omitempty"`
	VoiceOutput string               `json:"voiceOutput,omitempty"` // none: stateless voice-design audio candidates
	OutputKinds []string             `json:"outputKinds,omitempty"`
	Hidden      bool                 `json:"hidden,omitempty"`
	VoiceModels []string             `json:"voiceModels,omitempty"`
	QueueLimit  *ModelQueueLimit     `json:"queueLimit,omitempty"`
	InputRoles  map[string]InputRole `json:"inputRoles,omitempty"`
	Prompt      *PromptSpec          `json:"prompt,omitempty"`
	ID          string               `json:"id"`
	Label       string               `json:"label"`
	Capability  string               `json:"capability"`
	InputKinds  []string             `json:"inputKinds"`
	MaxInputs   int                  `json:"maxInputs"`
	Fields      map[string]Field     `json:"fields"`
}

// InputRole describes a single, named reference slot. Hidden slots remain in the
// saved graph but are excluded from execution, like inactive parameter drafts.
type InputRole struct {
	Kind           string           `json:"kind"`
	Label          string           `json:"label"`
	Description    string           `json:"description,omitempty"`
	Required       bool             `json:"required,omitempty"`
	Order          int              `json:"order,omitempty"`
	VisibleWhen    map[string]any   `json:"visibleWhen,omitempty"`
	VisibleWhenAny []map[string]any `json:"visibleWhenAny,omitempty"`
}

// RemoteHostConcurrency is a local resource ceiling, not a supplier quota.
// Account/model quotas remain authoritative and must not be inferred from it.
const RemoteHostConcurrency = 64

// Trusted provider policy. Jobs cannot set or override these fields.
type ModelQueueLimit struct {
	Bucket      string `json:"bucket"`
	RPM         int    `json:"rpm,omitempty"`
	RPS         int    `json:"rps,omitempty"`
	IntervalMS  int    `json:"intervalMs"`
	Concurrency int    `json:"concurrency"`
}
type Node struct {
	// A capability/default-model hint, not a supplier-specific canvas node type.
	// Hosts group these by capability and share their node renderer and parameter form.
	ID         string `json:"id"`
	Label      string `json:"label"`
	Capability string `json:"capability"`
	Model      string `json:"model"`
}
type Scheduling struct {
	Independent bool `json:"independent"`
	Concurrency int  `json:"concurrency"`
	MaxQueued   int  `json:"maxQueued"`
	Cancelable  bool `json:"cancelable"`
}
type Manifest struct {
	VoiceCatalog *VoiceCatalogSpec `json:"voiceCatalog,omitempty"`
	APIVersion   int               `json:"apiVersion"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Runtime      string            `json:"runtime"`
	Auth         string            `json:"auth"`
	Secrets      []string          `json:"secrets"`
	Nodes        []Node            `json:"nodes"`
	Models       []Model           `json:"models"`
	Scheduling   Scheduling        `json:"scheduling"`
}

type VoiceCatalogSpec struct {
	Search  bool             `json:"search"`
	Filters map[string]Field `json:"filters,omitempty"`
}
type VoiceQuery struct {
	Search    string         `json:"search"`
	Page      int            `json:"page"`
	PageSize  int            `json:"pageSize"`
	PageToken string         `json:"pageToken,omitempty"`
	Filters   map[string]any `json:"filters,omitempty"`
}
type VoicePage struct {
	Items         []VoiceOption `json:"items"`
	Total         int           `json:"total"`
	Page          int           `json:"page"`
	HasMore       bool          `json:"hasMore"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
}
type SearchVoiceCatalog interface {
	SearchVoices(context.Context, map[string]string, VoiceQuery) (VoicePage, error)
}
type Identity struct {
	User  string `json:"user"`
	Team  string `json:"team"`
	Scope string `json:"scope"` // v1: global; reserved user/team scopes
}
type Input struct {
	Kind string `json:"kind"`
	Role string `json:"role"`
	Data []byte `json:"data,omitempty"` // base64 in JSON, in stable reference order
	URL  string `json:"url,omitempty"`
}
type Generation struct {
	Provider   string         `json:"provider"`
	Capability string         `json:"capability"`
	Model      string         `json:"model"`
	Prompt     string         `json:"prompt"`
	Parameters map[string]any `json:"parameters"`
}
type Request struct {
	Voices   []Voice           `json:"voices,omitempty"`
	Voice    *Voice            `json:"voice,omitempty"`
	JobID    string            `json:"jobId"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Payload  json.RawMessage   `json:"payload"`
	Identity Identity          `json:"identity"`
	Inputs   []Input           `json:"inputs,omitempty"`
	Secrets  map[string]string `json:"secrets,omitempty"`
}
type Voice struct {
	Kind             string   `json:"kind,omitempty"` // design (default) or clone
	Provider         string   `json:"provider"`
	ID               string   `json:"id"`
	Label            string   `json:"label,omitempty"`
	CompatibleModels []string `json:"compatibleModels,omitempty"`
}
type VoiceOption struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Category         string   `json:"category"`
	Language         string   `json:"language,omitempty"`
	Description      string   `json:"description,omitempty"`
	PreviewURL       string   `json:"previewUrl,omitempty"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	CompatibleModels []string `json:"compatibleModels,omitempty"`
}
type VoiceCatalog interface {
	Voices(context.Context, map[string]string) ([]VoiceOption, error)
}
type Media struct {
	Kind  string `json:"kind,omitempty"`
	Voice *Voice `json:"voice,omitempty"`
	Data  []byte `json:"data,omitempty"`
	URL   string `json:"url,omitempty"`
}
type Result struct {
	Media    []Media         `json:"media"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
type Plugin interface {
	Manifest() Manifest
	Validate(context.Context, Request) error                        // MUST NOT submit paid work
	Execute(context.Context, Request, func(Result)) (Result, error) // no automatic retry
}

var IDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var EnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func (m Manifest) Validate() error {
	if m.VoiceCatalog != nil {
		if m.APIVersion < 2 || !m.VoiceCatalog.Search || len(m.VoiceCatalog.Filters) > 16 {
			return fmt.Errorf("invalid voice catalog capability")
		}
		for key, field := range m.VoiceCatalog.Filters {
			if !EnvPattern.MatchString(key) || field.Type != "string" || len(field.Enum) == 0 {
				return fmt.Errorf("invalid voice catalog filter")
			}
			if err := validateFieldSchema(field, 0); err != nil {
				return err
			}
		}
	}
	if !SupportedVersion(m.APIVersion) || !IDPattern.MatchString(m.ID) || m.Name == "" || m.Version == "" {
		return fmt.Errorf("invalid plugin identity or SDK version")
	}
	// The native shared lane retains the existing host configuration's limits.
	if m.Scheduling.Concurrency < 1 || m.Scheduling.MaxQueued < 1 || m.Scheduling.Independent && (m.Scheduling.Concurrency > 64 || m.Scheduling.MaxQueued > 1000) {
		return fmt.Errorf("invalid plugin scheduling limits")
	}
	seen := map[string]bool{}
	for _, key := range m.Secrets {
		if !EnvPattern.MatchString(key) || seen[key] {
			return fmt.Errorf("invalid or duplicate secret name")
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, model := range m.Models {
		if model.MaxVoices < 0 || model.MaxVoices > 16 || model.MaxVoices > 0 && model.Capability != "speech.generate" || model.VoiceOutput != "" && (model.VoiceOutput != "none" || model.Capability != "voice.design") {
			return fmt.Errorf("invalid voice capability")
		}
		for _, kind := range model.OutputKinds {
			if kind != "image" && kind != "video" && kind != "audio" {
				return fmt.Errorf("invalid output kind")
			}
		}
		if v := model.QueueLimit; v != nil && (v.Bucket == "" || len(v.Bucket) > 256 || strings.ContainsAny(v.Bucket, "\x00\r\n") || v.Concurrency < 1 || v.Concurrency > 64 || v.IntervalMS < 1 || v.IntervalMS > 3600000 || v.RPM < 0 || v.RPS < 0) {
			return fmt.Errorf("invalid model queue limit")
		}
		if model.ID == "" || seen[model.ID] || !capability(model.Capability) || model.MaxInputs < 0 || model.MaxInputs > 80 {
			return fmt.Errorf("invalid plugin model")
		}
		seen[model.ID] = true
		if len(model.VoiceModels) > 16 || len(model.VoiceModels) > 0 && model.Capability != "speech.generate" {
			return fmt.Errorf("invalid voice model aliases")
		}
		for _, alias := range model.VoiceModels {
			if alias == "" || len(alias) > 128 || strings.ContainsAny(alias, "\x00\r\n") {
				return fmt.Errorf("invalid voice model alias")
			}
		}
		for _, kind := range model.InputKinds {
			if kind != "image" && kind != "video" && kind != "audio" && kind != "voice" {
				return fmt.Errorf("invalid input kind")
			}
		}
		for key, f := range model.Fields {
			if key == "" || validateFieldSchema(f, 0) != nil {
				return fmt.Errorf("invalid parameter field: %s", key)
			}
		}
		if model.Prompt != nil && model.Prompt.Parameter != "" {
			if f, ok := model.Fields[model.Prompt.Parameter]; !ok || f.Type != "string" {
				return fmt.Errorf("invalid prompt parameter binding")
			}
		}
		for key, role := range model.InputRoles {
			if !IDPattern.MatchString(strings.ReplaceAll(key, "_", "-")) || role.Label == "" || role.Order < 0 || role.Order > 80 {
				return fmt.Errorf("invalid input role")
			}
			found := false
			for _, kind := range model.InputKinds {
				found = found || role.Kind == kind
			}
			if !found {
				return fmt.Errorf("unsupported input role kind")
			}
		}

	}
	seen = map[string]bool{}
	for _, n := range m.Nodes {
		if n.ID == "" || seen[n.ID] || !capability(n.Capability) {
			return fmt.Errorf("invalid plugin node")
		}
		seen[n.ID] = true
	}
	return nil
}
func capability(s string) bool {
	return s == "image.generate" || s == "video.generate" || s == "music.generate" || s == "speech.generate" || s == "voice.design"
}

// ValidateGeneration is shared by the host and process plugins.
func ValidateGeneration(m Manifest, r Request) error {
	var g Generation
	if json.Unmarshal(r.Payload, &g) != nil || g.Provider != m.ID {
		return fmt.Errorf("invalid plugin payload")
	}
	var model *Model
	for i := range m.Models {
		if m.Models[i].ID == g.Model {
			model = &m.Models[i]
			break
		}
	}
	if model == nil || model.Capability != g.Capability {
		return fmt.Errorf("model or capability unavailable")
	}
	if len(r.Inputs) > model.MaxInputs {
		return fmt.Errorf("too many references")
	}
	for _, in := range r.Inputs {
		found := false
		for _, kind := range model.InputKinds {
			found = found || kind == in.Kind
		}
		if !found {
			return fmt.Errorf("reference kind unsupported")
		}
	}
	if model.Prompt != nil && model.Prompt.Required {
		if strings.TrimSpace(g.Prompt) == "" {
			return fmt.Errorf("prompt required")
		}
	}
	if model.Prompt != nil && model.Prompt.MaxLength > 0 && utf8.RuneCountInString(g.Prompt) > model.Prompt.MaxLength {
		return fmt.Errorf("prompt too long")
	}
	parameters, err := NormalizeParameters(*model, g.Parameters)
	if err != nil {
		return err
	}
	if _, err := ResolveInputRoles(*model, parameters, r.Inputs); err != nil {
		return err
	}
	if r.Voice != nil && (r.Voice.Provider != m.ID || r.Voice.ID == "") {
		return fmt.Errorf("incompatible voice")
	}
	if r.Voice != nil {
		if g.Capability != "speech.generate" {
			return fmt.Errorf("voice binding requires speech generation")
		}
		if !AcceptsVoice(*model, r.Voice) {
			return fmt.Errorf("voice model incompatible")
		}
	}
	limit := model.MaxVoices
	if limit == 0 {
		limit = 1
	}
	if len(r.Voices) > limit || len(r.Voices) > 0 && (model.MaxVoices == 0 || r.Voice != nil || g.Capability != "speech.generate") {
		return fmt.Errorf("invalid voice inputs")
	}
	for i := range r.Voices {
		v := &r.Voices[i]
		if v.Provider != m.ID || v.ID == "" || !AcceptsVoice(*model, v) {
			return fmt.Errorf("incompatible voice")
		}
	}

	return nil
}

// AcceptsVoice checks only model-declared aliases, never a client-selected route.
func AcceptsVoice(model Model, voice *Voice) bool {
	if voice == nil || len(voice.CompatibleModels) == 0 {
		return true
	}
	for _, id := range voice.CompatibleModels {
		if id == model.ID {
			return true
		}
		for _, alias := range model.VoiceModels {
			if id == alias {
				return true
			}
		}
	}
	return false
}

// WireCall/WireReply form one bounded request/reply, without a shell.
type WireCall struct {
	APIVersion int     `json:"apiVersion"`
	Method     string  `json:"method"`
	Request    Request `json:"request"`
}
type WireReply struct {
	APIVersion int    `json:"apiVersion"`
	Kind       string `json:"kind,omitempty"`
	Result     Result `json:"result"`
	Error      string `json:"error,omitempty"`
}

// Serve handles exactly one invocation. Use stderr for local diagnostics only.
func Serve(ctx context.Context, in io.Reader, out io.Writer, p Plugin) error {
	var call WireCall
	if err := json.NewDecoder(io.LimitReader(in, 64<<20)).Decode(&call); err != nil {
		return err
	}
	reply := WireReply{APIVersion: call.APIVersion, Kind: "result"}
	encoder := json.NewEncoder(out)
	var err error
	if !SupportedVersion(call.APIVersion) {
		err = fmt.Errorf("unsupported SDK version")
	} else {
		switch call.Method {
		case "validate":
			err = p.Validate(ctx, call.Request)
		case "execute":
			reply.Result, err = p.Execute(ctx, call.Request, func(r Result) {
				_ = encoder.Encode(WireReply{APIVersion: call.APIVersion, Kind: "checkpoint", Result: r})
			})
		case "voices":
			catalog, ok := p.(VoiceCatalog)
			if !ok || call.APIVersion < 2 {
				err = fmt.Errorf("voice catalog unsupported")
				break
			}
			var items []VoiceOption
			items, err = catalog.Voices(ctx, call.Request.Secrets)
			if err == nil {
				reply.Result.Metadata, err = json.Marshal(struct {
					Items []VoiceOption `json:"items"`
				}{items})
			}
		case "voices.search":
			catalog, ok := p.(SearchVoiceCatalog)
			if !ok || call.APIVersion < 2 {
				err = fmt.Errorf("voice search unsupported")
				break
			}
			var query VoiceQuery
			body, _ := json.Marshal(call.Request.Metadata["query"])
			if json.Unmarshal(body, &query) != nil {
				err = fmt.Errorf("invalid voice query")
				break
			}
			var page VoicePage
			page, err = catalog.SearchVoices(ctx, call.Request.Secrets, query)
			if err == nil {
				reply.Result.Metadata, err = json.Marshal(page)
			}
		default:
			err = fmt.Errorf("unknown method")
		}
	}
	if err != nil {
		reply.Error = "plugin request failed"
	} // upstream errors may contain credentials
	return encoder.Encode(reply)
}
