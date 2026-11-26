package comfyui

import (
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// present also upgrades persisted definitions in memory. Bindings, defaults and
// execution graphs stay unchanged, including fields hidden in advanced settings.
func present(d Definition) Definition {
	models := append([]aigc.Model(nil), d.Manifest.Models...)
	for i, model := range models {
		if model.Prompt != nil {
			p := *model.Prompt
			model.Prompt = &p
			models[i].Prompt = model.Prompt
		}
		fields := make(map[string]aigc.Field, len(model.Fields))
		for key, f := range model.Fields {
			b, ok := d.Fields[key]
			if !ok {
				fields[key] = f
				continue
			}
			n := d.Graph[b.Node]
			title := n.Meta.Title
			if title == "" {
				title = n.Class
			}
			f.Group = &aigc.FieldGroup{ID: b.Node, Label: title + " [" + b.Node + "]"}
			f.Section = "advanced"
			semantic := b.Input
			internal := strings.ToLower(n.Class + " " + title)
			loader := strings.Contains(internal, "loader") || strings.Contains(internal, "加载") || strings.Contains(internal, "cleanup") || strings.Contains(internal, "备份")
			if b.Input == "value" {
				// Promoted primitive values get their meaning from the original node
				// title, then from direct downstream input names.
				for _, word := range []string{"prompt", "duration", "seed", "width", "height"} {
					if strings.Contains(strings.ToLower(title), word) {
						semantic = word
						break
					}
				}
				if semantic == "value" {
					for _, id := range keys(d.Graph) {
						for _, input := range keys(d.Graph[id].Inputs) {
							if ref, _, linked := link(d.Graph[id].Inputs[input], d.Graph); linked && ref == b.Node && commonLabels[input] != "" {
								semantic = input
								break
							}
						}
						if semantic != "value" {
							break
						}
					}
				}
			}
			if d.Prompt != nil && b.Node == d.Prompt.Node && b.Input == d.Prompt.Input {
				semantic = "prompt"
				if model.Prompt != nil {
					model.Prompt.Parameter = key
					model.Prompt.Label = "提示词"
				}
				f.Placement = "prompt"
			}
			f.Label = b.Input
			if label := commonLabels[semantic]; label != "" && !loader && f.Type != "boolean" {
				f.Section = "common"
				f.Label = label + " · " + b.Input
				if f.Type == "string" && (semantic == "prompt" || semantic == "negative_prompt" || semantic == "lyrics" || semantic == "caption" || semantic == "text") && len(f.Enum) == 0 {
					f.Control = "textarea"
				}
			}
			if semantic == "aspect_ratio" && len(f.Enum) > 0 {
				f.Control = "ratio"
				f.OptionLabels = map[string]string{}
				for _, v := range f.Enum {
					if s, ok := v.(string); ok {
						f.OptionLabels[s] = strings.Fields(s)[0]
					}
				}
			}
			if semantic == "seconds" || semantic == "duration" || semantic == "max_duration" {
				f.Control = "duration"
			}
			if semantic == "resolution" && len(f.Enum) > 0 {
				f.Control = "resolution"
			}
			if semantic == "seed" || semantic == "noise_seed" {
				f.Control = "seed"
				f.Section = "advanced"
			}
			if b.Input == "width" || b.Input == "height" {
				for other, ob := range d.Fields {
					if ob.Node == b.Node && ob.Input == "resolution" {
						f.VisibleWhen = map[string]any{other: "custom"}
					}
				}
			}
			fields[key] = f
		}
		models[i].Fields = fields
		if d.Prompt == nil {
			models[i].Prompt = nil
		}
	}
	d.Manifest.Models = models
	return modelLimits(d)
}

var commonLabels = map[string]string{
	"prompt": "提示词", "negative_prompt": "负向提示词", "caption": "提示词", "text": "提示词", "lyrics": "歌词",
	"width": "宽度", "height": "高度", "resolution": "分辨率", "aspect_ratio": "画面比例", "megapixels": "像素数",
	"duration": "时长（秒）", "duration_seconds": "时长（秒）", "seconds": "时长（秒）", "max_duration": "最长时长（秒）", "length": "长度 / 帧数", "num_frames": "帧数",
	"seed": "种子", "noise_seed": "种子",
}
