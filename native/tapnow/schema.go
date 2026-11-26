package tapnow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Reference struct {
	Native *NativeImage `json:"native,omitempty"`
	Kind   string       `json:"kind"`
	Role   string       `json:"role"`
	URL    string       `json:"url"`
	Order  int          `json:"order"`
}
type NativeImage struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
}

func AcceptsNativeImage(s map[string]any) bool {
	d := strings.Join(strings.Fields(strings.ToLower(text(s, "description"))), " ")
	return strings.Contains(d, "project") && (strings.Contains(d, "or a node_id") || strings.Contains(d, "or node_id"))
}

var outputVersionID = regexp.MustCompile(`^([A-Za-z0-9_-]{8,64})-([0-9]+)$`)

// NativeImageOfVersion supports the original ordered jobId-outputIndex versions.
// Explicit new associations must agree with the ticket and any canonical old index.
func NativeImageOfVersion(v map[string]any) (*NativeImage, error) {
	if text(v, "endpoint") != "tapnow" || text(v, "kind") == "video" {
		return nil, nil
	}
	t := schema(v, "ticket")
	if t == nil {
		return nil, nil
	}
	project := text(t, "project_id")
	ids := values(t["node_ids"])
	if strings.TrimSpace(project) == "" || len(ids) == 0 {
		return nil, errors.New("原生图片缺少完整票据")
	}
	seen := map[string]bool{}
	for _, value := range ids {
		id, ok := value.(string)
		if !ok || strings.TrimSpace(id) == "" || seen[id] {
			return nil, errors.New("原生图片票据节点不明确")
		}
		seen[id] = true
	}
	id := text(v, "providerNodeId")
	if id != "" && !seen[id] {
		return nil, errors.New("原生图片 ID 不属于该版本票据")
	}
	index := -1
	if m := outputVersionID.FindStringSubmatch(text(v, "id")); m != nil {
		if n, e := strconv.Atoi(m[2]); e == nil && n >= 0 && n < len(ids) {
			index = n
		}
	}
	if len(ids) > 1 && id != "" && index >= 0 && ids[index] != id {
		return nil, errors.New("原生图片 ID 与版本输出序号不一致")
	}
	if id == "" {
		if len(ids) == 1 {
			id = ids[0].(string)
		} else if index >= 0 {
			id = ids[index].(string)
		} else {
			return nil, errors.New("多节点票据缺少当前版本的明确节点关联")
		}
	}
	return &NativeImage{ProjectID: project, NodeID: id}, nil
}

type Request struct {
	Tool         string
	Model        string
	Prompt       string
	Parameters   map[string]any
	References   []Reference
	Kind         string
	VideoConsent bool
}

func values(v any) []any                               { a, _ := v.([]any); return a }
func text(m map[string]any, k string) string           { s, _ := m[k].(string); return s }
func schema(m map[string]any, k string) map[string]any { s, _ := m[k].(map[string]any); return s }
func num(m map[string]any, k string) (float64, bool)   { v, ok := m[k].(float64); return v, ok }
func Validate(s map[string]any, v any, path string) error {
	if s == nil {
		return fmt.Errorf("%s schema 缺失", path)
	}
	if en := values(s["enum"]); len(en) > 0 {
		ok := false
		for _, e := range en {
			ok = ok || reflect.DeepEqual(e, v)
		}
		if !ok {
			return fmt.Errorf("%s 不在支持选项中", path)
		}
	}
	switch text(s, "type") {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("%s 必须是对象", path)
		}
		p := schema(s, "properties")
		for _, k := range values(s["required"]) {
			if m[fmt.Sprint(k)] == nil {
				return fmt.Errorf("%s.%s 必填", path, k)
			}
		}
		for k, x := range m {
			field := schema(p, k)
			if field == nil {
				return fmt.Errorf("%s.%s 是不支持参数", path, k)
			}
			if e := Validate(field, x, path+"."+k); e != nil {
				return e
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s 必须是数组", path)
		}
		if min, ok := num(s, "minItems"); ok && float64(len(a)) < min {
			return fmt.Errorf("%s 项数不足", path)
		}
		if max, ok := num(s, "maxItems"); ok && float64(len(a)) > max {
			return fmt.Errorf("%s 项数超限", path)
		}
		seen := map[string]bool{}
		for i, x := range a {
			if e := Validate(schema(s, "items"), x, fmt.Sprintf("%s[%d]", path, i)); e != nil {
				return e
			}
			b, _ := json.Marshal(x)
			if s["uniqueItems"] == true && seen[string(b)] {
				return fmt.Errorf("%s 不允许重复", path)
			}
			seen[string(b)] = true
		}
	case "string":
		x, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s 必须是文本", path)
		}
		n := float64(utf8.RuneCountInString(x))
		if min, ok := num(s, "minLength"); ok && n < min {
			return fmt.Errorf("%s 过短", path)
		}
		if max, ok := num(s, "maxLength"); ok && n > max {
			return fmt.Errorf("%s 过长", path)
		}
	case "number", "integer":
		x, ok := v.(float64)
		if !ok || math.IsNaN(x) || math.IsInf(x, 0) || text(s, "type") == "integer" && math.Trunc(x) != x {
			return fmt.Errorf("%s 必须是有效数值", path)
		}
		if min, ok := num(s, "minimum"); ok && x < min {
			return fmt.Errorf("%s 小于下限", path)
		}
		if max, ok := num(s, "maximum"); ok && x > max {
			return fmt.Errorf("%s 超过上限", path)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s 必须是布尔值", path)
		}
	default:
		return fmt.Errorf("%s 使用尚不支持的 schema 类型", path)
	}
	return nil
}

// ReferenceProperty recognizes only typed media roles advertised by tools/list.
func ReferenceProperty(name string, s map[string]any) (kind string) {
	if strings.Contains(name, "image") && (strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_ids")) {
		return "image"
	}
	if strings.Contains(name, "video") && (strings.HasSuffix(name, "_url") || strings.HasSuffix(name, "_urls")) {
		return "video"
	}
	return ""
}
func (c *Client) Prepare(ctx context.Context, r Request) (map[string]any, Tool, error) {
	c.mu.Lock()
	var tool Tool
	pollAvailable, uploadAvailable := false, false
	for _, t := range c.tools {
		pollAvailable = pollAvailable || t.Name == "get_production_result"
		uploadAvailable = uploadAvailable || t.Name == "upload_image"
		if t.Name == r.Tool {
			tool = t
		}
	}
	c.mu.Unlock()
	if !pollAvailable {
		return nil, tool, errors.New("提供方未发现结果轮询工具，不能安全提交制作")
	}
	kind, _ := serviceKind(r.Tool)
	if tool.Name == "" || kind == "" || r.Kind != kind {
		return nil, tool, errors.New("未发现匹配的 TapNow 执行服务")
	}
	if kind == "video" && !r.VideoConsent {
		return nil, tool, errors.New("视频渲染会产生费用，请先明确同意")
	}
	props := schema(tool.InputSchema, "properties")
	args := map[string]any{}
	for k, v := range r.Parameters {
		if k == "model" || k == "prompt" || k == "project_id" || ReferenceProperty(k, schema(props, k)) != "" {
			return nil, tool, fmt.Errorf("%s 必须通过模型或类型化引用设置", k)
		}
		args[k] = v
	}
	if props["prompt"] != nil {
		args["prompt"] = r.Prompt
	} else if strings.TrimSpace(r.Prompt) != "" {
		return nil, tool, errors.New("此服务不接收通用提示词，请使用它的专属参数")
	}
	if props["model"] != nil {
		args["model"] = r.Model
	} else if r.Model != "" {
		return nil, tool, errors.New("此服务没有 model 参数")
	}
	refs := append([]Reference{}, r.References...)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Order < refs[j].Order })
	roles := map[string][]any{}
	for _, ref := range refs {
		if ref.Kind == "image" && ref.Native == nil && !uploadAvailable {
			return nil, tool, errors.New("提供方未发现 upload_image，图片引用不可用")
		}
		field := schema(props, ref.Role)
		if field == nil || ReferenceProperty(ref.Role, field) != ref.Kind {
			return nil, tool, fmt.Errorf("当前模型不支持 %s 引用角色 %s；请保留素材并选择可用服务", ref.Kind, ref.Role)
		}
		if ref.Order < 0 {
			return nil, tool, errors.New("引用顺序无效")
		}
		value := "reference-placeholder"
		if ref.Native != nil {
			if ref.Kind != "image" || !AcceptsNativeImage(field) {
				return nil, tool, errors.New("此引用字段只接受上传图片 ID，不支持原生节点")
			}
			if !strings.HasPrefix(ref.URL, "/files/") || strings.TrimSpace(ref.Native.NodeID) == "" || strings.TrimSpace(ref.Native.ProjectID) == "" {
				return nil, tool, errors.New("原生引用缺少本地资产、节点或项目")
			}
			if props["project_id"] == nil {
				return nil, tool, errors.New("制作工具缺少原生项目上下文")
			}
			if old, ok := args["project_id"]; ok && old != ref.Native.ProjectID {
				return nil, tool, errors.New("原生引用不能跨项目混用")
			}
			args["project_id"] = ref.Native.ProjectID
			value = ref.Native.NodeID
		} else if e := c.PublicURL(ctx, ref.URL); e != nil {
			return nil, tool, e
		}
		roles[ref.Role] = append(roles[ref.Role], value)
	}
	for role, list := range roles {
		field := schema(props, role)
		if text(field, "type") == "array" {
			args[role] = list
		} else {
			if len(list) != 1 {
				return nil, tool, fmt.Errorf("%s 仅支持一份引用，不能丢弃其余引用", role)
			}
			args[role] = list[0]
		}
	}
	if e := Validate(tool.InputSchema, args, "parameters"); e != nil {
		return nil, tool, e
	}
	return args, tool, nil
}
func (c *Client) Arguments(ctx context.Context, r Request) (map[string]any, error) {
	args, tool, e := c.Prepare(ctx, r)
	if e != nil {
		return nil, e
	}
	props := schema(tool.InputSchema, "properties")
	roles := map[string][]any{}
	refs := append([]Reference{}, r.References...)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Order < refs[j].Order })
	project := ""
	for _, ref := range refs {
		value := ref.URL
		if ref.Native != nil {
			value = ref.Native.NodeID
			if project != "" && project != ref.Native.ProjectID {
				return nil, errors.New("原生引用不能跨项目混用")
			}
			project = ref.Native.ProjectID
		} else if ref.Kind == "image" {
			upload, e := c.Call(ctx, "upload_image", map[string]any{"image_url": ref.URL})
			if e != nil {
				return nil, e
			}
			value = text(upload, "image_id")
			p := text(upload, "project_id")
			if value == "" || p == "" {
				return nil, errors.New("upload_image 缺少 image_id/project_id")
			}
			if project != "" && project != p {
				return nil, errors.New("多参考图片属于不同项目；不能静默混用")
			}
			project = p
		}
		roles[ref.Role] = append(roles[ref.Role], value)
	}
	for k, list := range roles {
		if text(schema(props, k), "type") == "array" {
			args[k] = list
		} else {
			args[k] = list[0]
		}
	}
	if project != "" {
		if props["project_id"] == nil {
			return nil, errors.New("制作工具缺少 project_id，无法安全定位引用")
		}
		args["project_id"] = project
	}
	return args, Validate(tool.InputSchema, args, "arguments")
}
