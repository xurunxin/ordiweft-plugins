// Package tapnowweb wraps the observed TapNow Web protocol independently of MCP.
package tapnowweb

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

//go:embed catalog.json catalog-legacy-transforms.json
var snapshots embed.FS

var compiledModels = func() map[string]map[string]any {
	b, _ := snapshots.ReadFile("catalog.json")
	var raw rawCatalog
	_ = json.Unmarshal(b, &raw)
	out := map[string]map[string]any{}
	for _, m := range raw.Models {
		out[str(m, "id")] = m
	}
	return out
}()

var compiledTransforms = func() map[string]string {
	out := map[string]string{}
	for id, m := range compiledModels {
		out[id] = str(m, "transformHash")
	}
	return out
}()

// Keep the exact reviewed revisions used by previously saved catalogs.
var legacyTransforms = func() map[string][]string {
	b, _ := snapshots.ReadFile("catalog-legacy-transforms.json")
	out := map[string][]string{}
	_ = json.Unmarshal(b, &out)
	return out
}()

func reviewedTransform(id, hash string) bool {
	return hash != "" && (compiledTransforms[id] == hash || slices.Contains(legacyTransforms[id], hash))
}

type Range struct {
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	TotalMin  float64 `json:"totalMin,omitempty"`
	TotalMax  float64 `json:"totalMax,omitempty"`
	Tolerance float64 `json:"maxTolerance,omitempty"`
}
type Field struct {
	VisibleWhen  map[string]any    `json:"visibleWhen,omitempty"`
	OptionLabels map[string]string `json:"optionLabels,omitempty"`
	Type         string            `json:"type"`
	Enum         []any             `json:"enum,omitempty"`
	Default      any               `json:"default,omitempty"`
	Minimum      float64           `json:"minimum,omitempty"`
	Maximum      float64           `json:"maximum,omitempty"`
	Fixed        bool              `json:"fixed,omitempty"`
	Rule         any               `json:"rule,omitempty"`
}
type Mode struct {
	ID             string           `json:"id"`
	Fields         map[string]Field `json:"fields"`
	Defaults       map[string]any   `json:"defaults"`
	Images         Range            `json:"images"`
	Videos         Range            `json:"videos"`
	Audios         Range            `json:"audios"`
	VideoDuration  *Range           `json:"videoDuration,omitempty"`
	AudioDuration  *Range           `json:"audioDuration,omitempty"`
	MaxInputs      int              `json:"maxInputs,omitempty"`
	PromptOptional bool             `json:"promptOptional,omitempty"`
	AllowAudioOnly bool             `json:"allowAudioOnly,omitempty"`
	AspectPolicy   string           `json:"aspectPolicy,omitempty"`
	Reason         string           `json:"reason,omitempty"`
	AccountLocks   map[string]any   `json:"accountLocks,omitempty"`
}
type Model struct {
	Adapter   string         `json:"-"`
	ID        string         `json:"id"`
	Model     string         `json:"model"`
	Kind      string         `json:"type"`
	Provider  string         `json:"provider"`
	Name      string         `json:"name"`
	Hidden    bool           `json:"hidden"`
	FromDraft bool           `json:"fromDraft,omitempty"`
	Modes     []Mode         `json:"modes"`
	Raw       map[string]any `json:"configuration"`
}
type Catalog struct {
	ObservedAt string                    `json:"observedAt"`
	Source     string                    `json:"source"`
	Asset      string                    `json:"asset"`
	SHA256     string                    `json:"sha256,omitempty"`
	Models     []Model                   `json:"models"`
	Tables     map[string]map[string]any `json:"tables"`
}
type rawCatalog struct {
	ObservedAt, Source, Asset, SHA256 string
	Models                            []map[string]any
	Tables                            map[string]map[string]any
}

func (c Catalog) observedBefore(other Catalog) bool {
	observed, e := time.Parse(time.RFC3339Nano, c.ObservedAt)
	baseline, otherErr := time.Parse(time.RFC3339Nano, other.ObservedAt)
	return otherErr == nil && (e != nil || observed.Before(baseline))
}

func Baseline() Catalog {
	b, _ := snapshots.ReadFile("catalog.json")
	c, e := ParseCatalog(b)
	if e != nil {
		panic(e)
	}
	return c
}
func ParseCatalog(b []byte) (Catalog, error) {
	var raw rawCatalog
	if json.Unmarshal(b, &raw) != nil || len(raw.Models) < 1 || len(raw.Models) > 250 || raw.Source != "https://app.tapnow.ai" || !(strings.HasPrefix(raw.Asset, "https://fe-assets.tapnow.media/") || strings.HasPrefix(raw.Asset, "https://app.tapnow.ai/assets/")) {
		return Catalog{}, errors.New("模型目录格式或来源不匹配")
	}
	c := Catalog{ObservedAt: raw.ObservedAt, Source: raw.Source, Asset: raw.Asset, SHA256: raw.SHA256, Tables: raw.Tables, Models: []Model{}}
	seen := map[string]bool{}
	for _, r := range raw.Models {
		id := str(r, "id")
		if id == "" {
			id = str(r, "model")
		}
		if seen[id] || id == "" || len(id) > 120 {
			return Catalog{}, errors.New("模型目录 ID 无效或重复")
		}
		seen[id] = true
		// Runtime-only visibility, such as Flash's site gate, cannot be
		// inferred from the public international catalog.
		hidden := r["hidden"] == true || r["isHidden"] == true || r["isHiddenDynamic"] == true
		m := Model{ID: id, Model: str(r, "model"), Kind: str(r, "type"), Provider: str(r, "provider"), Name: id, Hidden: hidden, Raw: r}
		m.Adapter = modelAdapter(m)
		if name := str(r, "displayName"); name != "" && len(name) <= 200 {
			m.Name = name
		}
		if m.Kind != "image" && m.Kind != "video" || m.Provider == "" {
			return Catalog{}, errors.New("模型类型或品牌缺失")
		}
		variants := arr(r["variants"])
		if len(variants) == 0 {
			for _, mode := range arr(r["supportedModes"]) {
				variants = append(variants, map[string]any{"modelType": mode})
			}
		}
		if len(variants) == 0 {
			return Catalog{}, fmt.Errorf("%s 缺少模式", id)
		}
		for _, v := range variants {
			variant := obj(v)
			modeID := str(variant, "modelType")
			options := merge(obj(r["options"]), obj(variant["options"]))
			defaults := merge(obj(r["defaults"]), obj(variant["defaults"]))
			// Older extracted catalogs could retain the regular H3 defaults.
			if id == "MiniMax-H3-Max" {
				options["resolutions"] = []any{"480P", "768P"}
				durations := []any{}
				for i := 5; i <= 15; i++ {
					durations = append(durations, float64(i))
				}
				options["durations"] = durations
				if defaults["resolution"] != "480P" {
					defaults["resolution"] = "768P"
				}
				if number(defaults["duration"]) < 5 {
					defaults["duration"] = float64(5)
				}
			}
			if id == "wanxiang-plus" && modeID == "text_to_video" && defaults["aspectRatio"] == nil {
				defaults["aspectRatio"] = "16:9"
			}
			promptPolicy := str(variant, "promptPolicy")
			mode := Mode{ID: modeID, Defaults: defaults, Fields: map[string]Field{}, AspectPolicy: str(variant, "aspectRatioPolicy"),
				PromptOptional: promptPolicy == "optional" || promptPolicy == "allow_empty" || modeID == "digital_human",
				MaxInputs:      int(number(variant["totalReferenceLimit"]))}
			keys := map[string]string{"aspectRatio": "aspectRatios", "imageSize": "imageSizeOptions", "resolution": "resolutions", "duration": "durations", "quality": "qualityOptions", "thinking_level": "thinkingLevelOptions", "outputFormat": "outputFormats", "generateMode": "modes", "style": "styles"}
			for key, opt := range keys {
				if choices := arr(options[opt]); len(choices) > 0 {
					mode.Fields[key] = Field{Type: scalarType(choices[0]), Enum: choices}
				}
				if rule, ok := options[opt+"Rule"]; ok {
					choices := ruleChoices(rule)
					if len(choices) > 0 {
						mode.Fields[key] = Field{Type: scalarType(choices[0]), Enum: choices, Rule: rule}
					}
				}
			}
			mode.AccountLocks = map[string]any{}
			for _, opt := range []string{"lockedDurations", "lockedResolutions"} {
				if rule, ok := options[opt+"Rule"]; ok {
					mode.AccountLocks[opt] = rule
				}
			}
			for key, d := range defaults {
				f, has := mode.Fields[key]
				if !has {
					f = Field{Type: scalarType(d)}
					if f.Type == "string" || f.Type == "number" {
						f.Enum = []any{d}
						f.Fixed = true
					}
				}
				f.Default = d
				mode.Fields[key] = f
			}
			if strings.HasPrefix(m.Adapter, "gpt-image-2") {
				mode.Fields["aspectRatio"] = Field{Type: "string", Enum: []any{"auto", "1:1", "2:1", "4:3", "3:4", "5:4", "4:5", "3:2", "2:3", "16:9", "9:16", "21:9", "9:21"}, Default: defaults["aspectRatio"]}
				f := mode.Fields["imageSize"]
				f.Type = "string"
				f.Enum = []any{"1K", "2K", "4K", "auto"}
				f.Rule = map[string]any{"if": map[string]any{"op": "===", "left": map[string]any{"parameter": "aspectRatio"}, "right": "auto"}, "then": []any{"auto"}, "else": []any{"1K", "2K", "4K"}}
				f.Fixed = false
				mode.Fields["imageSize"] = f
				for _, key := range []string{"targetWidth", "targetHeight"} {
					mode.Fields[key] = Field{Type: "integer", Minimum: 16, Maximum: 3840}
				}
				if strings.HasPrefix(m.Adapter, "gpt-image-2.5") {
					mode.Fields["background"] = Field{Type: "string", Enum: []any{"auto", "transparent"}}
				}
				if strings.HasPrefix(m.Adapter, "gpt-image-2.6") {
					mode.Fields["background"] = Field{Type: "string", Enum: []any{"auto", "opaque"}, Default: "auto"}
					mode.Fields["complexity"] = Field{Type: "string", Enum: []any{"auto", "low", "medium", "high"}, Default: "auto"}
					defaults["background"] = "auto"
					defaults["complexity"] = "auto"
					mode.Fields["output_format"] = Field{Type: "string", Enum: []any{"png", "jpeg", "gif", "svg"}, Default: "png"}
					if modeID == "image_to_image" {
						f := mode.Fields["output_format"]
						f.Enum = []any{"png", "jpeg"}
						mode.Fields["output_format"] = f
					}
					defaults["output_format"] = "png"
					defaults["aspectRatio"] = "auto"
					defaults["imageSize"] = "auto"
					for _, key := range []string{"aspectRatio", "imageSize"} {
						f := mode.Fields[key]
						f.Default = "auto"
						mode.Fields[key] = f
					}
					f := mode.Fields["aspectRatio"]
					f.Rule = map[string]any{"if": map[string]any{"op": "===", "left": map[string]any{"parameter": "output_format"}, "right": "gif"}, "then": []any{"auto"}, "else": f.Enum}
					mode.Fields["aspectRatio"] = f
					mode.Fields["frame_count"] = Field{Type: "integer", Minimum: 2, Maximum: 16, Default: float64(9), VisibleWhen: map[string]any{"output_format": "gif"}}
					mode.Fields["frame_duration_ms"] = Field{Type: "integer", Minimum: 20, Maximum: 2000, Default: float64(200), VisibleWhen: map[string]any{"output_format": "gif"}}
				}
			}
			if modeID == "text_to_video" && strings.HasPrefix(id, "seedance-2") && id != "seedance-2.5-flash" {
				mode.Fields["enableWebSearch"] = Field{Type: "boolean", Default: false, Fixed: !supportsSeedanceWebSearch(id)}
			}
			mode.Fields["times"] = Field{Type: "integer", Enum: []any{float64(1)}, Default: float64(1)}
			if times := arr(options["timesOptions"]); len(times) > 0 {
				mode.Fields["times"] = Field{Type: "integer", Enum: times, Default: times[0]}
			}
			// A fixed field is shown read-only rather than invented options for dynamic callbacks.
			if mode.AspectPolicy == "derived_from_reference" || mode.AspectPolicy == "forbidden" {
				delete(mode.Fields, "aspectRatio")
			}
			if modeID == "video_edit" && defaults["duration"] == float64(-1) {
				delete(mode.Fields, "duration")
			}
			imageMax := number(options["maxImages"])
			if imageMax == 0 {
				imageMax = 1
			}
			if m.Kind == "image" {
				if modeID == "image_to_image" {
					mode.Images = Range{Min: 1, Max: imageMax}
				}
			} else {
				switch modeID {
				case "image_to_video":
					mode.Images = Range{Min: 1, Max: 1}
				case "start_end_to_video":
					mode.Images = Range{Min: 2, Max: 2}
					if frames := obj(variant["frameImageRange"]); len(frames) > 0 {
						mode.Images = rangeOf(frames)
					}
				case "reference_to_video":
					mode.Images = Range{Min: 1, Max: imageMax}
				case "video_edit", "video_extend", "reference_video_to_video":
					mode.Videos = Range{Min: 1, Max: 1}
				}
			}
			for key, dst := range map[string]*Range{"referenceImageRange": &mode.Images, "referenceVideoRange": &mode.Videos, "referenceAudioRange": &mode.Audios} {
				if x := obj(variant[key]); len(x) > 0 {
					*dst = rangeOf(x)
				}
			}
			if x := obj(variant["referenceVideoDurationRange"]); len(x) > 0 {
				rr := rangeOf(x)
				mode.VideoDuration = &rr
			}
			if x := obj(variant["referenceAudioDurationRange"]); len(x) > 0 {
				rr := rangeOf(x)
				mode.AudioDuration = &rr
			}
			mode.AllowAudioOnly = mode.Audios.Max > 0 && !strings.HasPrefix(id, "seedance-2") && !strings.HasPrefix(id, "MiniMax-H3")
			mode.Reason = adapterReason(m, mode)
			m.Modes = append(m.Modes, mode)
		}
		c.Models = append(c.Models, m)
	}
	return c, nil
}
func adapterReason(m Model, mode Mode) string {
	if source := compiledModels[m.Adapter]; source != nil && (str(source, "provider") != m.Provider || str(source, "type") != m.Kind) {
		return "官网渠道或输出类型已变化；执行需复核封装"
	}
	if m.Kind == "image" && mode.ID != "text_to_image" && mode.ID != "image_to_image" {
		return "此生成模式尚未适配"
	}
	if m.Adapter != "" && !reviewedTransform(m.Adapter, str(m.Raw, "transformHash")) {
		return "官网转换规则已变化；目录可查看，执行需复核封装"
	}
	if m.Adapter == "" {
		return "已取得参数配置；该模型转换/产物格式尚未完成封装"
	}
	if m.ID == "MiniMax-H3-Max" && mode.ID != "text_to_video" && mode.ID != "image_to_video" {
		return "MiniMax H3 Max 仅支持文生视频和单首帧"
	}
	return ""
}

// New aliases may reuse a reviewed image contract only when the published
// transform fingerprint, provider, kind and model family all match. A new
// parameter/API transform still requires review; a name alone never enables it.
func modelAdapter(m Model) string {
	if verifiedModel(m.ID) {
		return m.ID
	}
	if m.Kind != "image" {
		return ""
	}
	for _, candidate := range []struct{ prefix, provider, id string }{{"gpt-image-2.6-", "openai", "gpt-image-2.6-flare"}, {"doubao-seedream-5.", "bytedance", "doubao-seedream-5.0-pro"}, {"doubao-seedream-5.", "bytedance", "doubao-seedream-5.0-lite"}} {
		if strings.HasPrefix(m.ID, candidate.prefix) && m.Provider == candidate.provider && reviewedTransform(candidate.id, str(m.Raw, "transformHash")) {
			return candidate.id
		}
	}
	return ""
}
func (c Catalog) Find(id, kind, mode string) (Model, Mode, error) {
	for _, m := range c.Models {
		if m.ID == id && m.Kind == kind {
			for _, v := range m.Modes {
				if v.ID == mode {
					if m.Hidden {
						return m, v, errors.New("官网隐藏的模型禁止执行")
					}
					if v.Reason != "" {
						return m, v, errors.New(v.Reason)
					}
					return m, v, nil
				}
			}
		}
	}
	return Model{}, Mode{}, errors.New("模型或模式不在当前目录中")
}
func scalarType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "string"
	}
}
func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func arr(v any) []any                       { a, _ := v.([]any); return a }
func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func number(v any) float64                  { n, _ := v.(float64); return n }
func merge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
func rangeOf(m map[string]any) Range {
	return Range{Min: number(m["min"]), Max: number(m["max"]), TotalMin: number(m["totalMin"]), TotalMax: number(m["totalMax"]), Tolerance: number(m["maxTolerance"])}
}
func validateFields(mode Mode, p map[string]any) (map[string]any, error) {
	out := merge(mode.Defaults, map[string]any{})
	for key, v := range p {
		f, ok := mode.Fields[key]
		if !ok {
			return nil, fmt.Errorf("%s 不在当前模式参数中", key)
		}
		if scalarType(v) != f.Type && !(f.Type == "integer" && scalarType(v) == "number") {
			return nil, fmt.Errorf("%s 类型错误", key)
		}
		if n, ok := v.(float64); ok {
			if math.IsNaN(n) || math.IsInf(n, 0) || f.Type == "integer" && n != math.Trunc(n) || f.Maximum > 0 && (n < f.Minimum || n > f.Maximum) {
				return nil, fmt.Errorf("%s 数值越界", key)
			}
		}
		if len(f.Enum) > 0 {
			ok := false
			for _, choice := range f.Enum {
				if fmt.Sprint(choice) == fmt.Sprint(v) {
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("%s 不在允许选项中", key)
			}
		}
		if s, ok := v.(string); ok && len(s) > 20000 {
			return nil, errors.New("参数字符串过长")
		}
		out[key] = v
	}
	if out["times"] == nil {
		out["times"] = float64(1)
	}
	for key, f := range mode.Fields {
		if f.Rule != nil {
			choices, ok := evalRule(f.Rule, merge(out, map[string]any{"modelType": mode.ID})).([]any)
			if !ok {
				return nil, fmt.Errorf("%s 动态选项无法解析", key)
			}
			if v, has := out[key]; has {
				found := false
				for _, choice := range choices {
					if fmt.Sprint(choice) == fmt.Sprint(v) {
						found = true
					}
				}
				if !found {
					return nil, fmt.Errorf("%s 与其他参数组合不受支持", key)
				}
			}
		}
	}
	return out, nil
}
