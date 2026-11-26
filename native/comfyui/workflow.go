// Package comfyui compiles ComfyUI API workflows into public aigc SDK plugins.
package comfyui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type Node struct {
	Class  string         `json:"class_type"`
	Inputs map[string]any `json:"inputs"`
	Meta   struct {
		Title string `json:"title"`
	} `json:"_meta,omitempty"`
}
type Graph map[string]Node
type Object struct {
	Input      map[string]map[string][]any `json:"input"`
	Output     []string                    `json:"output"`
	OutputNode bool                        `json:"output_node"`
}
type ObjectInfo map[string]Object

// Legacy hidden inputs use strings; V3 uses tuples. Neither belongs to the
// editable schema, so decode required/optional only.
func (o *Object) UnmarshalJSON(b []byte) error {
	var raw struct {
		Input      map[string]json.RawMessage `json:"input"`
		Output     []string                   `json:"output"`
		OutputNode bool                       `json:"output_node"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	o.Input = map[string]map[string][]any{}
	o.Output = raw.Output
	o.OutputNode = raw.OutputNode
	for _, section := range []string{"required", "optional"} {
		if b, ok := raw.Input[section]; ok {
			var fields map[string][]any
			if err := json.Unmarshal(b, &fields); err != nil {
				return err
			}
			o.Input[section] = fields
		}
	}
	return nil
}

type Binding struct {
	Node  string `json:"node"`
	Input string `json:"input"`
	Kind  string `json:"kind,omitempty"`
}
type Definition struct {
	Access   *Access             `json:"access,omitempty"`
	Prompt   *Binding            `json:"prompt,omitempty"`
	BaseURL  string              `json:"baseUrl"`
	Graph    Graph               `json:"graph"`
	Manifest aigc.Manifest       `json:"manifest"`
	Fields   map[string]Binding  `json:"fields"`
	Roles    map[string]Binding  `json:"roles"`
	Outputs  map[string][]string `json:"outputs"`
	Warnings []string            `json:"warnings"`
}

func Endpoint(base string) (string, error) {
	u, e := url.Parse(strings.TrimSpace(base))
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("请输入不含凭据、查询串的 ComfyUI HTTP(S) 地址")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func Parse(raw json.RawMessage) (Graph, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return nil, fmt.Errorf("API JSON 无效")
	}
	if p, ok := root["prompt"]; ok {
		raw = p
	} else if root["nodes"] != nil || root["links"] != nil {
		return nil, fmt.Errorf("请使用 ComfyUI 导出 API 格式，而非界面工作流 JSON")
	}
	var g Graph
	if json.Unmarshal(raw, &g) != nil || len(g) == 0 || len(g) > 512 {
		return nil, fmt.Errorf("API 工作流必须包含 1–512 个节点")
	}
	for id, n := range g {
		if id == "" || n.Class == "" || n.Inputs == nil {
			return nil, fmt.Errorf("API 节点缺少 class_type 或 inputs")
		}
		for key, v := range n.Inputs {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "password") || lower == "token" || lower == "access_token" || lower == "auth_token" || lower == "authorization" || strings.Contains(lower, "secret") {
				if v != "" && v != nil {
					return nil, fmt.Errorf("工作流含凭据字段，请移除并在 ComfyUI 服务侧配置")
				}
				delete(n.Inputs, key)
			}
			if a, ok := v.([]any); ok && len(a) == 2 {
				if ref, ok := a[0].(string); ok {
					if _, numeric := a[1].(float64); !numeric {
						continue
					}
					if _, exists := g[ref]; !exists {
						return nil, fmt.Errorf("节点 %s 的连线指向不存在的节点", id)
					}
					f, ok := a[1].(float64)
					if !ok || f < 0 || f != float64(int(f)) {
						return nil, fmt.Errorf("无效连线输出索引")
					}
				}
			}
		}
	}
	// ComfyUI requires a DAG. Reject cycles before any execution or persistence.
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("工作流存在循环连线")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, v := range g[id].Inputs {
			if ref, _, ok := link(v, g); ok {
				if e := visit(ref); e != nil {
					return e
				}
			}
		}
		state[id] = 2
		return nil
	}
	for id := range g {
		if e := visit(id); e != nil {
			return nil, e
		}
	}
	return g, nil
}
func link(v any, g Graph) (string, int, bool) {
	a, ok := v.([]any)
	if !ok || len(a) != 2 {
		return "", 0, false
	}
	id, ok := a[0].(string)
	if !ok {
		return "", 0, false
	}
	index, ok := a[1].(float64)
	_, exists := g[id]
	return id, int(index), ok && exists
}
func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func kind(s string) string {
	switch strings.ToUpper(s) {
	case "IMAGE", "MASK":
		return "image"
	case "VIDEO":
		return "video"
	case "AUDIO":
		return "audio"
	}
	return ""
}

// Compile combines literal API inputs with the target server's node definitions.
// Graph links are never turned into editable literals.
func Compile(name, base string, raw json.RawMessage, info ObjectInfo) (Definition, error) {
	d := Definition{Fields: map[string]Binding{}, Roles: map[string]Binding{}, Outputs: map[string][]string{}, Warnings: []string{}}
	var err error
	if d.BaseURL, err = Endpoint(base); err != nil {
		return d, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return d, fmt.Errorf("名称需要 1–80 个字符")
	}
	if d.Graph, err = Parse(raw); err != nil {
		return d, err
	}
	model := aigc.Model{ID: "workflow", Label: name, Fields: map[string]aigc.Field{}, InputRoles: map[string]aigc.InputRole{}, InputKinds: []string{}, OutputKinds: []string{}, Prompt: &aigc.PromptSpec{Label: "附加说明（工作流参数请在下方编辑）", Required: false}}
	outKinds, inKinds := map[string]bool{}, map[string]bool{}
	for _, id := range keys(d.Graph) {
		n := d.Graph[id]
		obj, ok := info[n.Class]
		if !ok {
			return d, fmt.Errorf("ComfyUI 缺少节点类型：%s", n.Class)
		}
		label := n.Meta.Title
		if label == "" {
			label = n.Class
		}
		label += " [" + id + "]"
		specs := map[string][]any{}
		for _, section := range []string{"required", "optional"} {
			for k, v := range obj.Input[section] {
				specs[k] = v
			}
		}
		// Flatten selected V3 dynamic-combo children (e.g. format.codec, format.quality).
		expandSpecs(specs, n.Inputs, "", 0)
		for _, input := range keys(n.Inputs) {
			value := n.Inputs[input]
			if ref, index, linked := link(value, d.Graph); linked {
				if source, ok := info[d.Graph[ref].Class]; ok && index >= len(source.Output) {
					return d, fmt.Errorf("节点 %s 的连线输出索引越界", id)
				}
				continue
			}
			spec := specs[input]
			opts := map[string]any{}
			if len(spec) > 1 {
				opts, _ = spec[1].(map[string]any)
				if opts == nil {
					opts = map[string]any{}
				}
			}
			// File widgets become named media inputs. Keep original filenames private in the graph.
			mediaKind := ""
			for _, k := range []string{"image", "audio", "video"} {
				if opts[k+"_upload"] == true {
					mediaKind = k
				}
			}
			if mediaKind != "" {
				key := fmt.Sprintf("ref-%d", len(d.Roles)+1)
				d.Roles[key] = Binding{id, input, mediaKind}
				model.InputRoles[key] = aigc.InputRole{Kind: mediaKind, Label: label + " · " + input, Required: value == "" || value == nil, Order: len(d.Roles)}
				inKinds[mediaKind] = true
				continue
			}
			if opts["hidden"] == true {
				continue
			}
			f, ok := field(value, spec)
			if !ok {
				d.Warnings = append(d.Warnings, label+" · "+input+" 保留原值（非标准参数类型）")
				continue
			}
			key := fmt.Sprintf("p%03d", len(d.Fields)+1)
			f.Label = label + " · " + input
			f.Description, _ = opts["tooltip"].(string)
			if len(spec) > 0 && spec[0] == "COMFY_DYNAMICCOMBO_V3" {
				f.Description = "结构分支由 API JSON 固定；切换分支请从 ComfyUI 重新导出。"
			}
			d.Fields[key] = Binding{id, input, ""}
			model.Fields[key] = f
		}
		if obj.OutputNode {
			found := map[string]bool{}
			for input, v := range n.Inputs {
				if s := specs[input]; len(s) > 0 {
					if t, ok := s[0].(string); ok {
						if k := kind(t); k != "" {
							found[k] = true
						}
					}
				}
				if ref, index, ok := link(v, d.Graph); ok {
					source := info[d.Graph[ref].Class]
					if index < len(source.Output) {
						if k := kind(source.Output[index]); k != "" {
							found[k] = true
						}
					}
				}
			}
			if len(found) > 0 {
				d.Outputs[id] = keys(found)
				for k := range found {
					outKinds[k] = true
				}
			}
		}
	}
	if len(outKinds) == 0 {
		return d, fmt.Errorf("未发现图像、视频或音频输出节点，请添加 SaveImage / SaveVideo / SaveAudio 等节点")
	}
	// Prefer a node's prompt/caption, following primitive string links when the
	// workflow promotes its prompt into a separate API node.
	for _, wanted := range []string{"prompt", "caption", "text"} {
		for _, id := range keys(d.Graph) {
			v, exists := d.Graph[id].Inputs[wanted]
			if !exists {
				continue
			}
			b := Binding{Node: id, Input: wanted}
			if ref, _, linked := link(v, d.Graph); linked {
				b = Binding{Node: ref, Input: "value"}
				v = d.Graph[ref].Inputs["value"]
			}
			if _, ok := v.(string); ok {
				d.Prompt = &b
				break
			}
		}
		if d.Prompt != nil {
			break
		}
	}
	model.Prompt.Label = "替换主提示词（可选，留空使用工作流参数）"
	model.OutputKinds = keys(outKinds)
	model.InputKinds = keys(inKinds)
	model.MaxInputs = len(d.Roles)
	model.Capability = "image.generate"
	if outKinds["video"] {
		model.Capability = "video.generate"
	} else if outKinds["audio"] && !outKinds["image"] {
		model.Capability = "music.generate"
	}
	b, _ := json.Marshal(struct {
		Name, Base string
		Graph      Graph
	}{name, d.BaseURL, d.Graph})
	hash := sha256.Sum256(b)
	d.Manifest = aigc.Manifest{APIVersion: 2, ID: "comfy-" + hex.EncodeToString(hash[:10]), Name: "ComfyUI · " + name, Version: "1.0.0", Runtime: "builtin", Auth: "server", Secrets: []string{}, Models: []aigc.Model{model}, Nodes: []aigc.Node{{ID: "workflow", Label: name + "（ComfyUI）", Capability: model.Capability, Model: model.ID}}, Scheduling: aigc.Scheduling{Independent: true, Concurrency: 1, MaxQueued: 20, Cancelable: false}}
	d = present(d)
	if err = d.Manifest.Validate(); err != nil {
		return d, err
	}
	if err = aigc.ValidateDefaults(d.Manifest.Models[0], defaults(d.Manifest.Models[0])); err != nil {
		return d, fmt.Errorf("工作流参数与目标服务定义不兼容：%w", err)
	}
	return d, nil
}
func defaults(m aigc.Model) map[string]any {
	v := map[string]any{}
	for k, f := range m.Fields {
		v[k] = f.Default
	}
	return v
}
func expandSpecs(specs map[string][]any, values map[string]any, prefix string, depth int) {
	if depth > 8 {
		return
	}
	for _, key := range keys(specs) {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		s := specs[key]
		if len(s) < 2 || s[0] != "COMFY_DYNAMICCOMBO_V3" {
			continue
		}
		opts, _ := s[1].(map[string]any)
		options, _ := opts["options"].([]any)
		for _, v := range options {
			o, _ := v.(map[string]any)
			if o["key"] != values[key] {
				continue
			}
			groups, _ := o["inputs"].(map[string]any)
			for _, section := range []string{"required", "optional"} {
				children, _ := groups[section].(map[string]any)
				for child, raw := range children {
					if s, ok := raw.([]any); ok {
						specs[key+"."+child] = s
					}
				}
			}
			expandSpecs(specs, values, key+".", depth+1)
		}
	}
}
func field(value any, spec []any) (aigc.Field, bool) {
	f := aigc.Field{Default: value}
	opts := map[string]any{}
	var typ any
	if len(spec) > 0 {
		typ = spec[0]
	}
	if len(spec) > 1 {
		opts, _ = spec[1].(map[string]any)
	}
	if e, ok := typ.([]any); ok {
		f.Enum = e
		f.Control = "select"
	}
	if typ == "COMBO" {
		f.Enum, _ = opts["options"].([]any)
		f.Control = "select"
	}
	if typ == "COMFY_DYNAMICCOMBO_V3" {
		// Changing a structural branch can introduce entirely new graph inputs.
		// Keep the exported branch fixed; its concrete children remain editable.
		f.Enum = []any{value}
		f.Control = "select"
	}
	switch v := value.(type) {
	case string:
		f.Type = "string"
		if opts["multiline"] == true {
			f.Control = "textarea"
		}
	case bool:
		f.Type = "boolean"
	case float64:
		f.Type = "number"
		if typ == "INT" {
			f.Type = "integer"
		}
	case map[string]any:
		f.Type = "object"
		f.Properties = map[string]aigc.Field{}
		for k, x := range v {
			child, ok := field(x, nil)
			if !ok {
				return f, false
			}
			child.Label = k
			f.Properties[k] = child
		}
	case []any:
		f.Type = "array"
		if len(v) == 0 {
			return f, false
		}
		child, ok := field(v[0], nil)
		if !ok {
			return f, false
		}
		child.Default = nil
		f.Items = &child
	default:
		return f, false
	}
	for k, target := range map[string]**float64{"min": &f.Minimum, "max": &f.Maximum, "step": &f.Step} {
		if v, ok := opts[k].(float64); ok {
			*target = &v
		}
	}
	return f, true
}
