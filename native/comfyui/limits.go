package comfyui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func h3Graph(d Definition) bool {
	for _, n := range d.Graph {
		if strings.HasPrefix(n.Class, "MiniMaxH3") {
			return true
		}
	}
	return false
}
func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}
func maximum(current *float64, cap float64) *float64 {
	if current != nil && *current < cap {
		return current
	}
	return &cap
}

func modelLimits(d Definition) Definition {
	h3 := h3Graph(d)
	for mi, m := range d.Manifest.Models {
		for key, f := range m.Fields {
			b := d.Fields[key]
			n := d.Graph[b.Node]
			semantic := b.Input
			if semantic == "value" {
				title := strings.ToLower(n.Meta.Title)
				if strings.Contains(title, "duration") {
					semantic = "seconds"
				}
				for _, downstream := range d.Graph {
					for input, v := range downstream.Inputs {
						if ref, _, ok := link(v, d.Graph); ok && ref == b.Node && (input == "seconds" || input == "duration" || input == "max_duration") {
							semantic = input
						}
					}
				}
			}
			if h3 {
				if semantic == "seconds" || semantic == "duration" {
					f.Maximum = maximum(f.Maximum, 15)
					f.Description = addDescription(f.Description, "本地 H3：最长 15 秒。")
				}
				if semantic == "megapixels" {
					f.Control = "resolution"
					f.OptionLabels = map[string]string{"0.1": "低清 · 0.1 MP", "0.2": "360P", "0.3": "416P", "0.4": "480P", "0.5": "540P", "0.7": "640P", "0.9": "720P", "1": "768P"}
					f.Maximum = maximum(f.Maximum, 1)
					f.Description = addDescription(f.Description, "本地 H3：768P 原生档，约 1 MP 像素预算。")
				}
				if semantic == "resolution" {
					allowed := []any{}
					for _, v := range f.Enum {
						s, _ := v.(string)
						p, e := strconv.Atoi(strings.TrimSuffix(strings.ToUpper(s), "P"))
						if s == "custom" || e == nil && p <= 768 {
							allowed = append(allowed, v)
						}
					}
					f.Enum = allowed
					f.Description = addDescription(f.Description, "本地 H3：最高 768P。")
				}
				if semantic == "length" && strings.HasPrefix(n.Class, "MiniMaxH3") {
					f.Maximum = maximum(f.Maximum, 362)
				}
			}
			if semantic == "seed" || semantic == "noise_seed" {
				f.Maximum = maximum(f.Maximum, 9007199254740991)
			}
			if n.Class == "MiniMaxMusic3TextEncode" && semantic == "max_duration" {
				f.Maximum = maximum(f.Maximum, 300)
				f.Description = addDescription(f.Description, "Music 3：最长 300 秒（5 分钟），可能提前结束。")
			}
			if n.Class == "MiniMaxMusic3TextEncode" && semantic == "lyrics" {
				f.Placement = "body"
			}
			if h3 && semantic == "fps" {
				f.Maximum = maximum(f.Maximum, 25)
				f.Description = addDescription(f.Description, "H3 原生采样为 24 fps；本地工作流上限 25 fps。")
			}
			m.Fields[key] = f
		}
		d.Manifest.Models[mi] = m
	}
	return d
}
func addDescription(old, s string) string {
	if strings.Contains(old, s) {
		return old
	}
	if old != "" {
		return old + " " + s
	}
	return s
}

// Validate derived geometry as well as literal schema fields, before uploads.
func validateModelGraph(d Definition, graph Graph) error {
	if !h3Graph(d) {
		return nil
	}
	for _, n := range graph {
		if n.Class == "MiniMaxH3Easy" && n.Inputs["resolution"] != "custom" {
			continue
		}
		if strings.HasPrefix(n.Class, "MiniMaxH3") {
			w, h := number(n.Inputs["width"]), number(n.Inputs["height"])
			if w > 0 && h > 0 && (w*h > 1024*1024 || math.Mod(w, 32) != 0 || math.Mod(h, 32) != 0) {
				return fmt.Errorf("本地 H3 自定义尺寸须为 32 的倍数，总像素不超过 1048576（768P 原生预算）")
			}
		}
	}
	return nil
}
