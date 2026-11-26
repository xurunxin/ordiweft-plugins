// SPDX-License-Identifier: Apache-2.0
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"github.com/google/jsonschema-go/jsonschema"
)

const providerID = "higgsfield"
const credentialName = "API_KEY"
const magiPrefix = "sand/magi-2/"
const hostMaxReferences = 80

//go:embed catalog.json
var catalogJSON []byte

var endpointPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)+$`)

type catalogEntry struct {
	ID       string          `json:"id,omitempty"`
	Endpoint string          `json:"endpoint"`
	Title    string          `json:"title"`
	Variant  string          `json:"variant"`
	Output   string          `json:"output"`
	Schema   json.RawMessage `json:"schema"`
}

type mediaField struct {
	name, kind      string
	array, required bool
	max             int
}

type modelSpec struct {
	model         aigc.Model
	schema        *jsonschema.Resolved
	endpoint      string
	output        string
	promptKey     string
	promptDefault string
	media         []mediaField
	roleFields    map[string]string
	encodedFields []string
}

type rawField struct {
	Type      any      `json:"type"`
	Default   any      `json:"default"`
	Enum      []any    `json:"enum"`
	Minimum   *float64 `json:"minimum"`
	Maximum   *float64 `json:"maximum"`
	MaxLength int      `json:"maxLength"`
	MaxItems  *int     `json:"maxItems"`
}

func loadPlugin() (*plugin, error) {
	var entries []catalogEntry
	if err := json.Unmarshal(catalogJSON, &entries); err != nil {
		return nil, err
	}
	p := &plugin{
		manifest: aigc.Manifest{
			APIVersion: 2, ID: providerID, Name: "Higgsfield", Version: "1.1.0",
			Runtime: "process", Auth: "api-key", Secrets: []string{credentialName},
			Scheduling: aigc.Scheduling{Independent: true, Concurrency: 2, MaxQueued: 20},
			Nodes: []aigc.Node{
				{ID: "image", Label: "Higgsfield 图像生成", Capability: "image.generate", Model: "bytedance/seedream/v5.0/pro/text-to-image"},
				{ID: "video", Label: "Higgsfield 视频生成", Capability: "video.generate", Model: magiPrefix + "text-to-video"},
			},
		},
		models: make(map[string]modelSpec, len(entries)),
	}
	for _, entry := range entries {
		spec, err := compileModel(entry)
		if err != nil {
			return nil, fmt.Errorf("Higgsfield model %s: %w", entry.Endpoint, err)
		}
		if _, exists := p.models[spec.model.ID]; exists {
			return nil, fmt.Errorf("duplicate Higgsfield model %s", spec.model.ID)
		}
		p.models[spec.model.ID] = spec
		p.manifest.Models = append(p.manifest.Models, spec.model)
	}
	return p, p.manifest.Validate()
}

func compileModel(entry catalogEntry) (modelSpec, error) {
	p := modelSpec{endpoint: entry.Endpoint, output: entry.Output, roleFields: map[string]string{}}
	if entry.ID == "" {
		entry.ID = entry.Endpoint
	}
	if !endpointPattern.MatchString(entry.Endpoint) || !endpointPattern.MatchString(entry.ID) || entry.Output != "image" && entry.Output != "video" {
		return p, fmt.Errorf("unsupported endpoint or output")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(entry.Schema, &schema); err != nil {
		return p, err
	}
	var err error
	p.schema, err = schema.Resolve(nil)
	if err != nil {
		return p, err
	}
	var raw struct {
		Properties map[string]rawField `json:"properties"`
		Required   []string            `json:"required"`
	}
	if err = json.Unmarshal(entry.Schema, &raw); err != nil {
		return p, err
	}
	label := entry.Title
	if entry.Variant != "" && entry.Variant != entry.Title && entry.Variant != "Default" {
		label += " · " + entry.Variant
	}
	p.model = aigc.Model{
		ID: entry.ID, Label: label, Capability: entry.Output + ".generate",
		InputKinds: []string{}, Fields: map[string]aigc.Field{},
		Prompt: &aigc.PromptSpec{Label: "提示词"},
	}
	if strings.HasPrefix(entry.Endpoint, magiPrefix) {
		p.model.QueueLimit = &aigc.ModelQueueLimit{Bucket: "magi-2", Concurrency: 1, IntervalMS: 1000}
		p.model.Pricing = &aigc.PricingSpec{
			Provider: providerID, Model: entry.Endpoint, Currency: "USD", CheckedAt: "2026-11-27",
			SourceURL: "https://open.higgsfield.ai/models/" + entry.Endpoint + "/api-reference",
			Rules:     []aigc.PricingRule{{Components: []aigc.PricingComponent{{Unit: "video_second", Rate: "0.08", Per: "1"}}}},
		}
	}
	for _, key := range slices.Sorted(maps.Keys(raw.Properties)) {
		prop := raw.Properties[key]
		required := slices.Contains(raw.Required, key)
		if key == "prompt" || key == "brief" {
			p.promptKey = key
			p.promptDefault, _ = prop.Default.(string)
			p.model.Prompt.Required = required && p.promptDefault == ""
			p.model.Prompt.MaxLength = prop.MaxLength
			continue
		}
		if kind := referenceKind(key); kind != "" {
			maximum := 1
			array := prop.Type == "array"
			if array {
				maximum = hostMaxReferences
				if prop.MaxItems != nil {
					maximum = *prop.MaxItems
				}
			}
			if maximum < 1 || maximum > hostMaxReferences {
				return p, fmt.Errorf("unsupported reference limit for %s", key)
			}
			if !slices.Contains(p.model.InputKinds, kind) {
				p.model.InputKinds = append(p.model.InputKinds, kind)
			}
			p.model.MaxInputs += maximum
			p.media = append(p.media, mediaField{key, kind, array, required, maximum})
			continue
		}
		field, encoded := parameterField(key, prop, required)
		if encoded {
			p.encodedFields = append(p.encodedFields, key)
		}
		p.model.Fields[key] = field
		if (key == "batch_size" || key == "series_amount") && prop.Maximum != nil {
			p.model.OutputCount = []aigc.OutputCountRule{{Parameter: key, Maximum: int(*prop.Maximum)}}
		}
	}
	p.model.MaxInputs = min(p.model.MaxInputs, hostMaxReferences)
	p.addInputRoles()
	return p, nil
}

func referenceKind(key string) string {
	switch key {
	case "image_url", "image_urls", "item_image_urls", "end_image_url", "last_image_url", "first_frame_url", "last_frame_url":
		return "image"
	case "video_url", "video_urls":
		return "video"
	case "audio_url", "audio_urls":
		return "audio"
	default:
		return ""
	}
}

func parameterField(key string, prop rawField, required bool) (aigc.Field, bool) {
	label := map[string]string{
		"aspect_ratio": "比例", "duration": "时长（秒）", "resolution": "分辨率",
		"seed": "随机种子", "generate_audio": "生成音频", "batch_size": "生成数量",
		"series_amount": "组图数量", "negative_prompt": "负面提示词",
	}[key]
	if label == "" {
		label = strings.ReplaceAll(key, "_", " ")
	}
	field := aigc.Field{Label: label, Required: required, Default: prop.Default, Enum: prop.Enum, Minimum: prop.Minimum, Maximum: prop.Maximum, MaxLength: prop.MaxLength}
	field.Type, _ = prop.Type.(string)
	if types, ok := prop.Type.([]any); ok && len(types) == 2 && types[1] == "null" {
		field.Type, _ = types[0].(string)
	}
	switch field.Type {
	case "integer", "number", "string", "boolean":
	default:
		field = aigc.Field{Type: "string", Control: "textarea", Label: label + "（JSON）", Required: required, MaxLength: 24000}
		if prop.Default != nil {
			value, _ := json.Marshal(prop.Default)
			field.Default = string(value)
		}
		return field, true
	}
	switch key {
	case "aspect_ratio":
		field.Control = "ratio"
	case "duration":
		if field.Type != "string" {
			field.Control = "duration"
		}
	case "resolution":
		field.Control = "resolution"
	case "seed":
		field.Control = "seed"
	case "batch_size", "series_amount":
		field.Control = "count"
	}
	return field, false
}

func (p *modelSpec) addInputRoles() {
	totals := map[string]int{}
	for _, f := range p.media {
		totals[f.kind]++
	}
	if len(p.media) == len(totals) {
		return
	}
	// Single references take precedence over optional arrays. The first and last
	// frame remain separately addressable even when canvas edges change order.
	slices.SortStableFunc(p.media, func(a, b mediaField) int {
		return rolePriority(a.name) - rolePriority(b.name)
	})
	p.model.InputRoles = map[string]aigc.InputRole{}
	for _, f := range p.media {
		for i := 0; i < f.max; i++ {
			role, label := f.name, f.name
			switch f.name {
			case "image_url", "first_frame_url":
				label = "首帧 / 主图"
			case "end_image_url", "last_image_url", "last_frame_url":
				label = "尾帧"
			case "video_url":
				label = "源视频"
			}
			if f.array {
				role += "_" + strconv.Itoa(i+1)
				label = f.kind + " " + strconv.Itoa(i+1)
			}
			p.roleFields[role] = f.name
			p.model.InputRoles[role] = aigc.InputRole{Kind: f.kind, Label: label, Required: f.required && i == 0, Order: min(len(p.roleFields), hostMaxReferences)}
		}
	}
}

func rolePriority(name string) int {
	switch name {
	case "image_url", "first_frame_url", "video_url", "audio_url":
		return 0
	case "end_image_url", "last_image_url", "last_frame_url":
		return 1
	default:
		return 2
	}
}
