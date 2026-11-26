package tapnowweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type Request struct {
	PromptBlocks                json.RawMessage `json:"-"` // Editor snapshot retained with the local receipt.
	CameraControl               *CameraControl
	Model, Mode, Kind, Prompt   string
	Parameters                  map[string]any
	References                  []tapnow.Reference
	VideoConsent, UploadConsent bool
	MaxCost                     float64
	UseEstimatedCost            bool
	Draft                       *DraftContinuation `json:"-"`
}

func (c Catalog) Prepare(r Request) (map[string]any, Mode, error) {
	if e := validateCamera(r); e != nil {
		return nil, Mode{}, e
	}
	if isDraftFinal(r) {
		return prepareDraftFinal(r)
	}
	m, mode, e := c.Find(r.Model, r.Kind, r.Mode)
	if e != nil {
		return nil, mode, e
	}
	if r.Kind == "video" && !r.VideoConsent {
		return nil, mode, errors.New("视频生成必须确认费用")
	}
	if r.MaxCost < 0 || math.IsNaN(r.MaxCost) || math.IsInf(r.MaxCost, 0) {
		return nil, mode, errors.New("费用上限无效")
	}
	if !mode.PromptOptional && strings.TrimSpace(r.Prompt) == "" || len(r.Prompt) > 20000 || strings.Contains(r.Prompt, "{{Asset:") {
		return nil, mode, errors.New("提示词为空、过长或含未授权素材占位符")
	}
	p, e := validateFields(mode, r.Parameters)
	if e != nil {
		return nil, mode, e
	}
	if r.CameraControl != nil && r.CameraControl.Enabled && m.ID == "tamar-google-gemini-pro" && p["enableGoogleSearch"] != true {
		return nil, mode, errors.New("此模型的 Lens 需要开启 Google 搜索")
	}
	refs := append([]tapnow.Reference(nil), r.References...)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Order < refs[j].Order })
	media := map[string][]string{"image": {}, "video": {}, "audio": {}}
	orders := map[int]bool{}
	for _, ref := range refs {
		if orders[ref.Order] || ref.Order < 0 {
			return nil, mode, errors.New("引用顺序重复或无效")
		}
		orders[ref.Order] = true
		if _, ok := media[ref.Kind]; !ok || ref.Native != nil {
			return nil, mode, errors.New("Web 引用必须使用素材 URL，而非 MCP node_id")
		}
		if ref.Role != "" && ref.Role != ref.Kind {
			return nil, mode, errors.New("引用角色与模型模式不匹配")
		}
		if strings.HasPrefix(ref.URL, "/files/") {
			if !r.UploadConsent {
				return nil, mode, errors.New("请确认将私人引用素材上传 TapNow")
			}
		} else if e = publicURL(ref.URL); e != nil {
			return nil, mode, e
		}
		media[ref.Kind] = append(media[ref.Kind], ref.URL)
	}
	for kind, limit := range map[string]Range{"image": mode.Images, "video": mode.Videos, "audio": mode.Audios} {
		count := float64(len(media[kind]))
		if count < limit.Min || count > limit.Max {
			return nil, mode, fmt.Errorf("%s 引用数量必须在 %.0f..%.0f", kind, limit.Min, limit.Max)
		}
	}
	if mode.MaxInputs > 0 && len(refs) > mode.MaxInputs {
		return nil, mode, fmt.Errorf("引用素材合计最多 %d 项", mode.MaxInputs)
	}
	for _, u := range media["audio"] {
		if !strings.HasPrefix(u, "/files/") {
			return nil, mode, errors.New("音频参考须导入本地 PCM WAV，以测量真实时长")
		}
	}
	if !mode.AllowAudioOnly && len(media["audio"]) > 0 && len(media["image"])+len(media["video"]) == 0 {
		return nil, mode, errors.New("参考音频不能单独使用，请接入图片或视频")
	}
	if strings.Contains(r.Prompt, "@{") || strings.Contains(r.Prompt, "{{node/") {
		return nil, mode, errors.New("提示词包含未转换引用")
	}
	for _, match := range seedPrompt.FindAllStringSubmatch(r.Prompt, -1) {
		kind := map[string]string{"Image": "image", "Video": "video", "Audio": "audio"}[match[1]]
		var n int
		_, _ = fmt.Sscan(match[2], &n)
		if n < 1 || n > len(media[kind]) {
			return nil, mode, errors.New("提示词引用序号不在当前输入中")
		}
	}
	if len(media["video"]) > 0 {
		for _, u := range media["video"] {
			if !strings.HasPrefix(u, "/files/") {
				return nil, mode, errors.New("视频参考须先导入本地 MP4，以测量真实时长")
			}
		}
	}
	b, e := c.transform(m, mode, p, r.Prompt, media)
	if e != nil {
		return nil, mode, e
	}
	if len(media["audio"]) > 0 && b["audios"] == nil && b["audio_url"] == nil && b["reference_audio_urls"] == nil && b["kling_audio_refs"] == nil {
		return nil, mode, errors.New("此模型的音频参数转换尚未验证")
	}
	b["prompt"] = humanPrompt.ReplaceAllString(r.Prompt, "$1 $2")
	if s, ok := b["_prompt"].(string); ok {
		b["prompt"] = s
		delete(b, "_prompt")
	}
	if mode.ID == "digital_human" && strings.TrimSpace(r.Prompt) == "" {
		delete(b, "prompt")
	}
	b["times"] = p["times"]
	if r.CameraControl != nil && r.CameraControl.Enabled {
		b["cameraControl"] = r.CameraControl
	}
	if b["scene"] == nil {
		b["scene"] = "generation"
	}
	for key, v := range b {
		if v == nil {
			delete(b, key)
		} else if a, ok := v.([]any); ok && len(a) == 0 && key != "kling_contents" {
			delete(b, key)
		} else if a, ok := v.([]string); ok && len(a) == 0 {
			delete(b, key)
		}
	}
	return b, mode, nil
}
func (c Catalog) transform(m Model, v Mode, p map[string]any, prompt string, media map[string][]string) (map[string]any, error) {
	if m.Adapter != "" {
		m.ID = m.Adapter
	}
	b := map[string]any{"provider": m.Provider, "model": m.Model}
	images, videos := media["image"], media["video"]
	put := func(dst, src string) {
		if x, ok := p[src]; ok {
			b[dst] = x
		}
	}
	ratio := func() {
		s := str(p, "aspectRatio")
		if s != "" && !strings.EqualFold(s, "auto") {
			b["aspect_ratio"] = s
		}
	}
	if m.Kind == "image" {
		if len(images) > 0 {
			b["images"] = images
		}
		switch {
		case strings.HasPrefix(m.ID, "gpt-image-2"):
			if len(images) > 0 {
				b["scene"] = "image-edit"
			}
			w, h := number(p["targetWidth"]), number(p["targetHeight"])
			if w > 0 || h > 0 {
				if w < 16 || h < 16 || w > 3840 || h > 3840 || int(w)%16 != 0 || int(h)%16 != 0 || w*h > 8294400 {
					return nil, errors.New("自定义像素需双边 16..3840、16 倍数且总像素不超过 8294400")
				}
				if p["aspectRatio"] != "auto" || p["imageSize"] != "auto" {
					return nil, errors.New("自定义像素需同时选 auto 比例与尺寸")
				}
				b["size"] = fmt.Sprintf("%.0fx%.0f", w, h)
			} else {
				sizes := obj(c.Tables["O2"][str(p, "imageSize")])
				size := str(sizes, str(p, "aspectRatio"))
				if size == "" {
					return nil, errors.New("尺寸与比例组合不在网页表格中")
				}
				b["size"] = size
			}
			put("quality", "quality")
			if strings.HasPrefix(m.ID, "gpt-image-2.6") {
				if p["complexity"] != "auto" {
					put("complexity", "complexity")
				}
				put("background", "background")
				put("output_format", "output_format")
				if p["output_format"] == "gif" || p["output_format"] == "svg" {
					if len(images) > 0 {
						return nil, errors.New("TapNow GPT Image 2.6 GIF / SVG 仅支持文生图")
					}
					if p["output_format"] == "gif" && b["size"] != "auto" {
						return nil, errors.New("GIF 输出需要自适应尺寸")
					}
					if p["output_format"] == "gif" {
						b["frame_count"] = p["frame_count"]
						b["frame_duration_ms"] = p["frame_duration_ms"]
					}
				}
			}
			if p["background"] == "transparent" {
				b["background"] = "transparent"
				b["output_format"] = "png"
			}
		case m.ID == "gpt-image-1":
			b["size"] = c.Tables["SY"][str(p, "aspectRatio")]
		case m.ID == "hunyuan-image-3.5-preview":
			ratio()
			b["resolution"] = strings.ToLower(str(p, "imageSize"))
			if len(images) > 0 {
				b["scene"] = "image-edit"
			}
		case strings.Contains(m.ID, "banana") || m.ID == "tamar-google-gemini-pro":
			ratio()
			put("image_size", "imageSize")
			if b["image_size"] == "512P" {
				b["image_size"] = "0.5K"
			}
			put("enable_google_search", "enableGoogleSearch")
			put("enable_image_search", "enable_image_search")
			put("thinking_level", "thinking_level")
		case strings.HasPrefix(m.ID, "grok"):
			b["resolution"] = "1k"
			if p["imageSize"] == "2K" {
				b["resolution"] = "2k"
			}
			ratio()
			if len(images) > 0 {
				b["scene"] = "image-edit"
			}
		case m.ID == "reve-2.1":
			b["aspect_ratio"] = strings.ToLower(str(p, "aspectRatio"))
			b["num_images"] = p["times"]
			put("output_format", "outputFormat")
		case strings.HasPrefix(m.ID, "doubao-seedream"):
			if m.ID == "doubao-seedream-4.0" {
				b["size"] = c.Tables["hY"][str(p, "aspectRatio")]
				if b["size"] == nil {
					b["size"] = p["aspectRatio"]
				}
			} else {
				size, aspect := str(p, "imageSize"), str(p, "aspectRatio")
				if strings.EqualFold(aspect, "auto") && (m.ID == "doubao-seedream-5.0-pro" || m.ID == "doubao-seedream-5.5") {
					b["size"] = size
				} else {
					dimensions, e := dimension(size, aspect)
					if e != nil {
						return nil, e
					}
					b["size"] = dimensions
				}
			}
			put("sequential_image_generation", "sequential_image_generation")
			if m.ID == "doubao-seedream-5.0-lite" {
				b["sequential_image_generation"] = "disabled"
			}
			if m.ID == "doubao-seedream-5.0-pro" || m.ID == "doubao-seedream-5.5" {
				b["resolution"] = strings.ToLower(str(p, "imageSize"))
				if len(images) > 0 {
					b["scene"] = "image-edit"
				}
			}
		case strings.HasPrefix(m.ID, "midjourney"):
			ratio()
			if len(images) > 0 {
				b["images"] = images
			}
		case strings.HasPrefix(m.ID, "flux-kontext"):
			ratio()
			size := obj(c.Tables["mY"][str(p, "aspectRatio")])
			if len(size) == 0 {
				return nil, errors.New("Flux 比例查表失败")
			}
			b["width"] = size["width"]
			b["height"] = size["height"]
		case m.ID == "flux-2-pro":
			size, e := dimension(str(p, "imageSize"), str(p, "aspectRatio"))
			if e != nil {
				return nil, e
			}
			var w, h int
			fmt.Sscanf(size, "%dx%d", &w, &h)
			b["width"] = w
			b["height"] = h
		case m.ID == "fal-recraft-v4" || m.ID == "fal-recraft-v4-vector":
			stem := "fal-recraft-v4"
			if p["generateMode"] == "pro" {
				stem += "-pro"
			}
			if m.ID == "fal-recraft-v4-vector" {
				stem += "-vector"
			}
			b["model"] = stem
			size, e := dimension("1K", str(p, "aspectRatio"))
			if e != nil {
				return nil, e
			}
			b["image_size"] = size
		case m.ID == "world-edit":
			ratio()
			put("image_size", "imageSize")
		default:
			return nil, errors.New("图像模型转换尚未封装")
		}
		return b, nil
	}
	put("duration", "duration")
	put("resolution", "resolution")
	switch {
	case m.ID == "omnihuman-1.5":
		if v.ID != "digital_human" || len(images) != 1 || len(media["audio"]) != 1 {
			return nil, errors.New("OmniHuman 需要一张图片和一段 WAV 音频")
		}
		b["image_url"] = images[0]
		b["audio_url"] = media["audio"][0]
	case strings.HasPrefix(m.ID, "seedance-2"):
		mappings := map[string]string{"seedance-2.0": "agent-video-generation-01", "seedance-2.0-fast": "agent-video-generation-01-fast", "seedance-2.0-mini": "agent-video-generation-01-mini", "seedance-2.5": seedance25WireModel, "seedance-2.5-4k": seedance25WireModel, "seedance-2.5-draft": seedance25WireModel, "seedance-2.5-flash": "agent-video-generation-02-flash"}
		b["model"] = mappings[m.ID]
		put("generate_audio", "generateAudio")
		switch v.ID {
		case "image_to_video":
			b["images"] = roleURLs(images, "first_frame")
		case "start_end_to_video":
			b["images"] = []any{map[string]any{"url": images[0], "role": "first_frame", "is_verified": false}, map[string]any{"url": images[1], "role": "last_frame", "is_verified": false}}
		case "reference_to_video", "video_edit":
			b["images"] = roleURLs(images, "reference_image")
			b["videos"] = roleURLs(videos, "reference_video")
			b["audios"] = roleURLs(media["audio"], "reference_audio")
		}
		switch v.ID {
		case "text_to_video", "reference_to_video":
			b["ratio"] = p["aspectRatio"]
		case "video_edit":
			b["ratio"] = "adaptive"
			b["duration"] = -1
		}
		if v.ID == "text_to_video" && supportsSeedanceWebSearch(m.ID) && p["enableWebSearch"] == true {
			b["enable_web_search"] = true
		}
		if m.ID == "seedance-2.5-draft" {
			b["draft"] = true
			b["resolution"] = "480p"
		}
		if m.ID == "seedance-2.5-4k" {
			b["resolution"] = "4k"
		}
		b["_prompt"] = seedPrompt.ReplaceAllString(prompt, "$1$2")
	case strings.HasPrefix(m.ID, "MiniMax-H3"):
		if len(media["audio"]) > 0 {
			b["reference_audio_urls"] = media["audio"]
		}
		switch v.ID {
		case "image_to_video":
			b["first_frame_image_url"] = images[0]
		case "start_end_to_video":
			b["first_frame_image_url"] = images[0]
			b["last_frame_image_url"] = images[1]
		case "reference_to_video":
			if len(images) > 0 {
				b["reference_image_urls"] = images
			}
			if len(videos) > 0 {
				b["reference_video_urls"] = videos
			}
			b["ratio"] = p["aspectRatio"]
		default:
			b["ratio"] = p["aspectRatio"]
		}
		b["_prompt"] = h3WebPrompt(prompt)
	case m.ID == "minimax" || strings.HasPrefix(m.ID, "MiniMax-Hailuo"):
		if len(images) > 0 {
			b["first_frame_image"] = images[0]
		}
		if len(images) > 1 {
			b["last_frame_image"] = images[1]
		}
	case strings.HasPrefix(m.ID, "veo"):
		ratio()
		put("generate_audio", "generateAudio")
		if m.ID == "veo3.1-lite-extend" {
			b["model"] = "veo3.1-lite"
			b["videos"] = videos
		} else if v.ID == "reference_to_video" {
			b["reference_images"] = images
		} else if len(images) > 0 {
			b["images"] = images
		}
	case strings.HasPrefix(m.ID, "gemini-omni"):
		delete(b, "resolution")
		ratio()
		task := "text_to_video"
		if len(videos) > 0 {
			task = "edit"
			delete(b, "duration")
		}
		if len(images) > 0 {
			task = "image_to_video"
			if len(images) > 1 || len(videos) > 0 {
				task = "reference_to_video"
			}
			b["images"] = images
		}
		if len(videos) > 0 {
			b["videos"] = videos
		}
		b["interaction_task"] = task
	case m.ID == "seedance-1.5-pro":
		b["model"] = "doubao-seedance-pro"
		b["ratio"] = p["aspectRatio"]
		put("generate_audio", "generateAudio")
		if len(images) > 0 {
			b["image_url"] = images[0]
		}
	case strings.HasPrefix(m.ID, "doubao-seedance"):
		if len(images) > 0 {
			b["ratio"] = p["aspectRatio"]
			if m.ID == "doubao-seedance-lite" && v.ID == "reference_to_video" {
				b["reference_images"] = images
			} else {
				b["images"] = images
			}
		} else {
			ratio()
		}
	case strings.HasPrefix(m.ID, "vidu"):
		b["mode"] = v.ID
		if len(images) > 0 {
			b["images"] = images
		}
		if v.ID == "text_to_video" || v.ID == "reference_to_video" {
			ratio()
		}
		if m.ID == "viduq3-pro" {
			b["audio"] = p["generateAudio"]
			put("style", "style")
		}
	case m.ID == "flux-3":
		put("generate_audio", "generateAudio")
		if len(images) > 0 {
			b["image_url"] = images[0]
		} else {
			ratio()
		}
	case m.ID == "depth-anything-video":
		delete(b, "duration")
		delete(b, "resolution")
		b["video_url"] = videos[0]
	case strings.HasPrefix(m.ID, "grok-imagine-video"):
		if v.ID == "video_edit" {
			delete(b, "duration")
			delete(b, "resolution")
			b["video_url"] = videos[0]
		} else if len(images) > 0 {
			if v.ID == "reference_to_video" {
				b["reference_images"] = images
				ratio()
			} else {
				b["image_url"] = images[0]
			}
		} else {
			ratio()
		}
	case strings.HasPrefix(m.ID, "sora-2"):
		b["resolution"] = strings.ToLower(str(p, "resolution"))
		name := "sora-2-text-to-video"
		if len(images) > 0 {
			name = "sora-2-image-to-video"
			b["images"] = images
		}
		if m.ID == "sora-2-pro" {
			name += "-pro"
		}
		b["model"] = name
		ratio()
	case m.ID == "midjourney-video":
		delete(b, "duration")
		b["images"] = images
		if v.ID != "start_end_to_video" {
			put("loop", "loop")
		}
	case m.ID == "wan3.0-video":
		b["provider"] = "alibaba"
		if p["turbo"] == true {
			b["model"] = "wan3.0-video-prime"
		}
		b["resolution"] = strings.ToUpper(str(p, "resolution"))
		put("generate_audio", "generateAudio")
		b["reference_audio_urls"] = media["audio"]
		b["_prompt"] = wan3Prompt(prompt)
		switch v.ID {
		case "image_to_video":
			b["image_url"] = images[0]
		case "reference_to_video":
			b["reference_image_urls"] = images
			b["ratio"] = p["aspectRatio"]
		case "reference_video_to_video":
			b["reference_video_urls"] = videos
			if len(images) > 0 {
				b["reference_image_urls"] = images
			}
			b["ratio"] = p["aspectRatio"]
		default:
			b["ratio"] = p["aspectRatio"]
		}
	case strings.HasPrefix(m.ID, "happyhorse"):
		b["provider"] = "alibaba"
		b["resolution"] = strings.ToUpper(str(p, "resolution"))
		stem := "happyhorse-1.0"
		if m.ID == "happyhorse-1.1" {
			stem = "happyhorse-1.1"
		}
		switch v.ID {
		case "text_to_video":
			b["model"] = stem + "-t2v"
			b["ratio"] = p["aspectRatio"]
		case "image_to_video":
			b["model"] = stem + "-i2v"
			b["image_url"] = images[0]
		case "reference_to_video":
			b["model"] = stem + "-r2v"
			b["reference_image_urls"] = images
			b["ratio"] = p["aspectRatio"]
		case "video_edit":
			b["model"] = "happyhorse-1.0-video-edit"
			delete(b, "duration")
			b["video_url"] = videos[0]
			if len(images) > 0 {
				b["reference_image_urls"] = images
			}
			if p["generateAudio"] == true {
				b["audio_setting"] = "auto"
			} else if p["generateAudio"] == false {
				b["audio_setting"] = "origin"
			}
		}
	case strings.HasPrefix(m.ID, "pixverse"):
		delete(b, "resolution")
		b["quality"] = strings.ToLower(str(p, "resolution"))
		if v.ID == "text_to_video" {
			ratio()
		}
		if len(images) > 0 {
			b["images"] = images
		}
		if m.ID == "pixverse-v5.5" {
			put("generate_audio_switch", "generateAudio")
			if v.ID != "start_end_to_video" {
				put("generate_multi_clip_switch", "generateMultiClip")
			}
		} else {
			put("sound_effect_switch", "soundEffect")
		}
	case m.ID == "kling-4.0-preview":
		if err := transformKling4(b, v.ID, p, prompt, media); err != nil {
			return nil, err
		}
	case strings.HasPrefix(m.ID, "kling"):
		delete(b, "resolution")
		b["_prompt"] = klingPrompt.ReplaceAllStringFunc(prompt, func(s string) string {
			parts := klingPrompt.FindStringSubmatch(s)
			return "<<<" + strings.ToLower(parts[1]) + "_" + parts[2] + ">>>"
		})
		switch m.ID {
		case "kling-motion-control":
			delete(b, "duration")
			b["image_url"] = images[0]
			b["video_url"] = videos[0]
			put("character_orientation", "character_orientation")
			put("mode", "generateMode")
			put("keep_original_sound", "keep_original_sound")
		case "kling-o3-video-edit":
			delete(b, "duration")
			b["video_url"] = videos[0]
			if len(images) > 0 {
				b["image_urls"] = images
			}
			put("keep_audio", "keepOriginalAudio")
		case "kling-video-o1", "kling-video-o3", "kling-video-o3-edit", "kling-video-o1-edit":
			if strings.Contains(m.ID, "o3") {
				b["model"] = "kling-v3-omni-official"
				put("mode", "generateMode")
			}
			if v.ID == "text_to_video" {
				ratio()
			} else if v.ID == "image_to_video" || v.ID == "start_end_to_video" {
				list := []any{map[string]any{"image_url": images[0], "type": "first_frame"}}
				if len(images) > 1 {
					list = append(list, map[string]any{"image_url": images[1], "type": "end_frame"})
				}
				b["image_list"] = list
			} else {
				if len(images) > 0 {
					list := []any{}
					for _, u := range images {
						list = append(list, map[string]any{"image_url": u})
					}
					b["image_list"] = list
				}
				if len(videos) > 0 {
					role := "feature"
					if v.ID == "video_edit" {
						role = "base"
					}
					list := []any{}
					for _, u := range videos {
						list = append(list, map[string]any{"video_url": u, "refer_type": role, "keep_original_sound": "yes"})
					}
					b["video_list"] = list
				}
				if v.ID != "video_edit" {
					ratio()
				}
			}
			if p["generateAudio"] == true && len(videos) == 0 {
				b["sound"] = "on"
			}
		case "kling-v3-pro":
			b["model"] = "kling-v3-official"
			put("mode", "generateMode")
			if len(images) > 0 {
				b["images"] = images
			}
			if v.ID == "text_to_video" || v.ID == "reference_to_video" {
				ratio()
			}
			if p["generateAudio"] == true {
				b["sound"] = "on"
			}
		default:
			if m.ID == "kling" {
				b["model"] = "kling-v2-1-master"
				if len(images) > 0 {
					b["model"] = "kling-v2-1"
				}
				if len(images) == 2 {
					b["mode"] = "pro"
				}
			}
			if len(images) > 0 {
				b["images"] = images
			} else {
				ratio()
			}
			if m.ID == "kling-v2-6" && v.ID != "start_end_to_video" && p["generateAudio"] == true {
				b["sound"] = "on"
			}
		}
	case strings.HasPrefix(m.ID, "wanxiang"):
		if m.ID == "wanxiang-i2v-flash" {
			b["images"] = images
			break
		}
		size := str(obj(c.Tables["Tr"][str(p, "resolution")]), str(p, "aspectRatio"))
		if m.ID == "wanxiang-v2-6" {
			b["shot_type"] = "single"
			if p["generateMultiClip"] == true {
				b["shot_type"] = "multi"
			}
			wanAudio(b, media["audio"], p["generateAudio"])
			switch v.ID {
			case "image_to_video":
				b["model"] = "wanxiang-2.6-i2v"
				b["images"] = images
			case "reference_to_video", "reference_video_to_video":
				b["model"] = "wanxiang-2.6-r2v"
				b["reference_urls"] = append(append([]string{}, videos...), images...)
				b["_prompt"] = wanCharacterPrompt(prompt, len(videos))
				b["size"] = size
				delete(b, "resolution")
			default:
				b["model"] = "wanxiang-2.6-t2v"
				b["size"] = size
				delete(b, "resolution")
			}
		} else {
			stem := "plus"
			if m.ID == "wanxiang-preview" {
				stem = "preview"
				wanAudio(b, media["audio"], p["generateAudio"])
				b["prompt_extend"] = true
				b["watermark"] = false
			}
			if len(images) > 0 {
				b["model"] = "wanxiang-i2v-" + stem
				b["images"] = images
			} else {
				b["model"] = "wanxiang-t2v-" + stem
				b["size"] = size
				delete(b, "resolution")
			}
		}
		if b["size"] == "" {
			return nil, errors.New("万象尺寸查表失败")
		}
	default:
		return nil, errors.New("视频模型转换尚未封装")
	}
	return b, nil
}

var seedPrompt = regexp.MustCompile(`\{\{(Image|Video|Audio) (\d+)\}\}`)
var humanPrompt = regexp.MustCompile(`\{\{(Image|Video|Audio|Text|Element) (\d+)\}\}`)
var h3AtMention = regexp.MustCompile(`@(https?://|(?:Image|Video|Audio|Text)\s*\d+)`)

// Matches the published H3 web adapter (D$). Local ComfyUI H3 uses <Picture N>
// instead; never apply the local model's reference syntax to this cloud API.
func h3WebPrompt(prompt string) string {
	return h3AtMention.ReplaceAllString(humanPrompt.ReplaceAllString(prompt, "$1 $2"), "$1")
}

var klingPrompt = regexp.MustCompile(`\{\{(Image|Video|Element) (\d+)\}\}`)

func supportsSeedanceWebSearch(id string) bool {
	switch id {
	case "seedance-2.0", "seedance-2.5", "seedance-2.5-4k", "seedance-2.5-draft":
		return true
	}
	return false
}

func wan3Prompt(prompt string) string {
	return seedPrompt.ReplaceAllStringFunc(prompt, func(value string) string {
		match := seedPrompt.FindStringSubmatch(value)
		return map[string]string{"Image": "图", "Video": "视频", "Audio": "音频"}[match[1]] + match[2]
	})
}

func wanCharacterPrompt(prompt string, videos int) string {
	return seedPrompt.ReplaceAllStringFunc(prompt, func(value string) string {
		match := seedPrompt.FindStringSubmatch(value)
		if match[1] == "Audio" {
			return humanPrompt.ReplaceAllString(value, "$1 $2")
		}
		var ordinal int
		_, _ = fmt.Sscan(match[2], &ordinal)
		if match[1] == "Image" {
			ordinal += videos
		}
		return fmt.Sprintf("character%d", ordinal)
	})
}

func wanAudio(body map[string]any, audio []string, generate any) {
	if len(audio) > 0 {
		body["audio_url"] = audio[0]
	} else {
		body["audio"] = generate
	}
}

func roleURLs(urls []string, role string) []any {
	out := []any{}
	for _, u := range urls {
		out = append(out, map[string]any{"url": u, "role": role, "is_verified": false})
	}
	return out
}
func dimension(size, ratio string) (string, error) {
	edge := map[string]float64{"1K": 1024, "2K": 2048, "3K": 3072, "4K": 4096}[size]
	var a, b float64
	if edge == 0 {
		return "", errors.New("未知像素档")
	}
	if _, e := fmt.Sscanf(ratio, "%f:%f", &a, &b); e != nil || a <= 0 || b <= 0 {
		return "", errors.New("比例无效")
	}
	w, h := edge, edge
	if a > b {
		h = math.Round(edge * b / a)
	} else {
		w = math.Round(edge * a / b)
	}
	return fmt.Sprintf("%.0fx%.0f", w, h), nil
}
func verifiedModel(id string) bool {
	switch id {
	case "gpt-image-2.6-flare", "gpt-image-2.6-sunburst", "doubao-seedream-5.5",
		"hunyuan-image-3.5-preview", "fal-recraft-v4-vector", "omnihuman-1.5",
		"seedance-2.5-4k", "seedance-2.5-flash", "kling-4.0-preview":
		return true
	}
	return slices.Contains(strings.Fields("happyhorse-1.0 happyhorse-1.1 happyhorse-1.0-edit wan3.0-video gpt-image-2.5-flare gpt-image-2.5-sunburst gpt-image-2 nano-banana-2.1 nano-banana-flash nano-banana-flash-lite tamar-google-gemini-pro nano-banana nano-banana-2 reve-2.1 fal-recraft-v4 doubao-seedream-5.0-lite doubao-seedream-5.0-pro grok-2-image grok-imagine-image grok-imagine-image-quality doubao-seedream-4.0 doubao-seedream-4.5 midjourney-v7 midjourney-v8.2 midjourney-v8.1 midjourney-niji7 gpt-image-1 flux-2-pro flux-kontext-max flux-kontext-pro world-edit seedance-2.0 seedance-2.0-mini seedance-2.0-fast seedance-2.5 seedance-2.5-draft MiniMax-H3 MiniMax-H3-Max kling-video-o3 flux-3 depth-anything-video gemini-omni-flash gemini-omni-1.1-flash kling-v3-pro kling-o3-video-edit kling-video-o3-edit kling-video-o1-edit kling-video-o1 veo3.1-lite veo3.1-lite-extend veo3.1-fast veo3.1 seedance-1.5-pro wanxiang-v2-6 kling-v2-6 MiniMax-Hailuo-2.3 doubao-seedance-pro doubao-seedance-lite midjourney-video minimax MiniMax-Hailuo-2.3-Fast viduq1 viduq2 viduq2-pro viduq2-turbo viduq3-pro kling kling-v2-5-turbo-pro kling-motion-control wanxiang-plus wanxiang-i2v-flash wanxiang-preview pixverse-v5.5 pixverse-v5 veo3 veo3-fast veo2 sora-2-standard sora-2-pro grok-imagine-video-1.5 grok-imagine-video"), id)
}
