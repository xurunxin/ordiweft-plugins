package comfyui

import (
	"encoding/json"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var sdkMention = regexp.MustCompile(`\{\{(Image|Video|Audio) ([0-9]+)\}\}`)
var nativeMention = regexp.MustCompile(`<(?i:Picture|Video|Audio)\s+([0-9]+)>`)
var easyMention = regexp.MustCompile(`__MINIMAX_H3_REF_([0-9]+)__`)

func reaches(g Graph, from, to string, seen map[string]bool) bool {
	if from == to {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, v := range g[from].Inputs {
		if ref, _, ok := link(v, g); ok && reaches(g, ref, to, seen) {
			return true
		}
	}
	return false
}

type promptSlot struct {
	kind      string
	index     int
	role      string
	easyIndex int
}

func h3Slots(d Definition) ([]promptSlot, error) {
	var slots []promptSlot
	add := func(value any, kind string, index int) error {
		ref, _, ok := link(value, d.Graph)
		if !ok {
			return nil
		}
		slot := promptSlot{kind: kind, index: index}
		for role, b := range d.Roles {
			if b.Kind != "" && b.Kind != kind {
				continue
			}
			if reaches(d.Graph, ref, b.Node, map[string]bool{}) {
				if slot.role != "" && slot.role != role {
					return fmt.Errorf("一个 H3 引用插槽连接了多份上传素材，无法确定提示词映射")
				}
				slot.role = role
			}
		}
		slots = append(slots, slot)
		return nil
	}
	for _, id := range keys(d.Graph) {
		n := d.Graph[id]
		if n.Class == "MiniMaxH3Easy" {
			counts := map[string]int{}
			for i := 1; i <= 15; i++ {
				key := fmt.Sprintf("media_%d", i)
				if v, ok := n.Inputs[key]; ok {
					kind, _ := n.Inputs[fmt.Sprintf("media_type_%d", i)].(string)
					if kind == "" {
						kind = "image"
					}
					counts[kind]++
					if e := add(v, kind, counts[kind]); e != nil {
						return nil, e
					}
					if _, _, linked := link(v, d.Graph); linked {
						slots[len(slots)-1].easyIndex = i
					}
				}
			}
		} else if strings.HasPrefix(n.Class, "MiniMaxH3") {
			audioIndex := 0
			for _, pair := range []struct{ prefix, kind string }{{"ref_images.ref_image_", "image"}, {"ref_videos.ref_video_", "video"}, {"ref_audios.ref_audio_", "audio"}} {
				indexes := []int{}
				for key, v := range n.Inputs {
					if strings.HasPrefix(key, pair.prefix) {
						if i, e := strconv.Atoi(strings.TrimPrefix(key, pair.prefix)); e == nil {
							if _, _, ok := link(v, d.Graph); ok {
								indexes = append(indexes, i)
							}
						}
					}
				}
				sort.Ints(indexes)
				for ordinal, i := range indexes {
					if pair.kind == "video" {
						v := n.Inputs[fmt.Sprintf("ref_video_audios.ref_video_audio_%d", i)]
						if _, _, ok := link(v, d.Graph); ok {
							audioIndex++
							if e := add(v, "audio", audioIndex); e != nil {
								return nil, e
							}
						}
					}
					index := ordinal + 1
					if pair.kind == "audio" {
						audioIndex++
						index = audioIndex
					}
					if e := add(n.Inputs[fmt.Sprintf("%s%d", pair.prefix, i)], pair.kind, index); e != nil {
						return nil, e
					}
				}
			}
			frameIndex := 0
			for _, key := range []string{"first_frame", "last_frame"} {
				if _, ok := n.Inputs[key]; ok {
					frameIndex++
					if e := add(n.Inputs[key], "image", frameIndex); e != nil {
						return nil, e
					}
				}
			}
		}
	}
	return slots, nil
}
func slotTag(s promptSlot) string {
	return fmt.Sprintf("<%s %d>", map[string]string{"image": "Picture", "video": "Video", "audio": "Audio"}[s.kind], s.index)
}

func projectH3Prompt(d Definition, text string, inputs []aigc.Input) (string, error) {
	if !h3Graph(d) {
		if sdkMention.MatchString(text) {
			return "", fmt.Errorf("此工作流未定义多模态提示词引用格式")
		}
		return text, nil
	}
	slots, err := h3Slots(d)
	if err != nil {
		return "", err
	}
	roleTags := map[string]string{}
	valid := map[string]bool{}
	for _, s := range slots {
		tag := slotTag(s)
		valid[tag] = true
		if s.easyIndex > 0 {
			tag = fmt.Sprintf("__MINIMAX_H3_REF_%d__", s.easyIndex)
		}
		if s.role != "" {
			if old := roleTags[s.role]; old != "" && old != tag {
				return "", fmt.Errorf("同一素材连入多个 H3 插槽，提示词引用存在歧义")
			}
			roleTags[s.role] = tag
		}
	}
	counters := map[string]int{}
	mentions := map[string]string{}
	for _, in := range inputs {
		counters[in.Kind]++
		name := map[string]string{"image": "Image", "video": "Video", "audio": "Audio"}[in.Kind]
		mentions[fmt.Sprintf("{{%s %d}}", name, counters[in.Kind])] = roleTags[in.Role]
	}
	var failure error
	text = sdkMention.ReplaceAllStringFunc(text, func(token string) string {
		if tag := mentions[token]; tag != "" {
			return tag
		}
		failure = fmt.Errorf("提示词引用 %s 未连接到 H3 的对应插槽", token)
		return token
	})
	// Easy resolves mixed-media placeholders itself after inspecting video
	// soundtracks. Keeping them avoids guessing standalone audio offsets.
	if easyMention.MatchString(text) {
		text = easyMention.ReplaceAllStringFunc(text, func(token string) string {
			m := easyMention.FindStringSubmatch(token)
			i, _ := strconv.Atoi(m[1])
			for _, s := range slots {
				if i > 0 && s.easyIndex == i {
					return token
				}
			}
			failure = fmt.Errorf("工作流提示词含未连接的 H3 引用")
			return token
		})
	}
	for _, token := range nativeMention.FindAllString(text, -1) {
		if !valid[token] {
			failure = fmt.Errorf("提示词引用 %s 不在 H3 工作流输入中", token)
		}
	}
	if strings.Contains(text, "@{") || strings.Contains(text, "{{node/") {
		failure = fmt.Errorf("提示词包含未转换的画布引用")
	}
	return text, failure
}

func (p *Plugin) prepareGraph(r aigc.Request) (Graph, error) {
	var g aigc.Generation
	_ = json.Unmarshal(r.Payload, &g)
	model := p.Manifest().Models[0]
	params, e := aigc.NormalizeParameters(model, g.Parameters)
	if e != nil {
		return nil, e
	}
	var graph Graph
	data, _ := json.Marshal(p.Definition.Graph)
	_ = json.Unmarshal(data, &graph)
	for key, value := range params {
		b := p.Definition.Fields[key]
		n := graph[b.Node]
		n.Inputs[b.Input] = value
	}
	if e = validateModelGraph(p.Definition, graph); e != nil {
		return nil, e
	}
	inputs, e := aigc.ResolveInputRoles(model, params, r.Inputs)
	if e != nil {
		return nil, e
	}
	if b := p.Definition.Prompt; b != nil {
		text, _ := graph[b.Node].Inputs[b.Input].(string)
		if strings.TrimSpace(g.Prompt) != "" {
			text = g.Prompt
		}
		text, e = projectH3Prompt(p.Definition, text, inputs)
		if e != nil {
			return nil, e
		}
		graph[b.Node].Inputs[b.Input] = text
	}
	return graph, nil
}
