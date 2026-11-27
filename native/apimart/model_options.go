// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func normalizeModelOptions(spec modelSpec, body map[string]any, files []attachment) error {
	var images, videos int
	var hasMask, hasMarkup bool
	for _, file := range files {
		switch file.ref.Role {
		case "mask":
			hasMask = true
		case "markup":
			hasMarkup = true
		default:
			switch file.ref.Kind {
			case "image":
				images++
			case "video":
				videos++
			}
		}
	}

	switch spec.Upstream {
	case "gpt-image-2.6-ext", "gpt-image-2.6-flare", "gpt-image-2.6-sunburst":
		return normalizeGPTImageOptions(spec.Upstream, body, images, hasMask, hasMarkup)
	case "qwen-image-3.0", "qwen-image-3.0-pro":
		if body["prompt_extend_mode"] == "agent" && images > 0 {
			return errors.New("Qwen Image 智能改写仅支持文生图")
		}
	case "wan2.7-image", "wan2.7-image-pro":
		return normalizeWanImageOptions(spec.Upstream, body, images)
	case "seedream-5-5":
		if body["layer_decomposition"] == true {
			if images != 1 {
				return errors.New("Seedream 图层拆分需要恰好一张参考图")
			}
			if size, _ := body["size"].(string); size != "" && size != "auto" && size != "1K" && size != "1.5K" && size != "2K" {
				return errors.New("Seedream 图层拆分尺寸只能选择 1K、1.5K、2K 或 auto")
			}
		} else if body["prompt"] == nil {
			return errors.New("Seedream 图片生成需要提示词")
		}
		if body["background"] == "transparent" && body["output_format"] != "png" {
			return errors.New("Seedream 透明背景需要 PNG 格式")
		}
	case "grok-imagine-image", "grok-imagine-image-quality":
		body["aspect_ratio"] = body["size"]
		delete(body, "size")
	case "midjourney":
		return normalizeMidjourneyOptions(body, images)
	case "seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini", "seedance-2.5", "seedance-2-5-flash":
		if body["web_search"] == true {
			body["tools"] = []map[string]string{{"type": "web_search"}}
		}
		delete(body, "web_search")
	case "wan3.0-video", "wan3.0-video-prime":
		return normalizeWanVideoOptions(spec.Model.ID, body, len(files))
	case "apimart-zoe":
		if spec.Model.TaskContinuation == nil && len(files) == 0 && body["prompt"] == nil {
			return errors.New("Zoe 文生视频需要提示词")
		}
	case "kling-v3", "kling-v3-omni":
		return normalizeKlingOptions(spec.Upstream, body, images)
	case "kling-v4", "kling-v4-flash":
		if strings.HasSuffix(spec.Model.ID, "-reference") && videos == 0 && body["audio_setting"] != nil {
			return errors.New("可灵 V4 原声设置需要连接一段参考视频")
		}
	case "pixverse-v6":
		if images == 2 && body["generate_multi_clip_switch"] == true {
			return errors.New("PixVerse 连续多片段仅支持文生视频和单图生视频")
		}
	}
	return nil
}

func normalizeGPTImageOptions(upstream string, body map[string]any, images int, mask, markup bool) error {
	format := body["output_format"]
	if mask && markup || markup && images == 0 {
		return errors.New("GPT Image 标注层需要底图，且不能与蒙版同时使用")
	}
	if format != "gif" && format != "svg" {
		return nil
	}
	if upstream == "gpt-image-2.6-ext" && images > 0 {
		return errors.New("GPT Image 2.6 Ext 的 GIF 和 SVG 仅支持文生图")
	}
	if mask || markup {
		return errors.New("GPT Image 的 GIF 和 SVG 不接受蒙版或标注层")
	}
	if format == "svg" {
		if body["n"] != float64(1) {
			return errors.New("GPT Image SVG 每次只能输出一张")
		}
		if svg, ok := body["svg"].(map[string]any); ok {
			if name, _ := svg["model"].(string); strings.TrimSpace(name) == "" {
				return errors.New("请选择 SVG 编写模型")
			}
		}
		return nil
	}
	if upstream == "gpt-image-2.6-ext" {
		if body["size"] != nil && body["size"] != "auto" {
			return errors.New("GPT Image 2.6 Ext GIF 的尺寸必须为 auto")
		}
	} else if size, _ := body["size"].(string); strings.Contains(size, ":") {
		return errors.New("GPT Image GIF 尺寸使用像素值或 auto")
	}
	return nil
}

func normalizeWanImageOptions(upstream string, body map[string]any, imageCount int) error {
	sequential := body["enable_sequential"] == true
	count, _ := body["n"].(float64)
	if !sequential && count > 4 {
		return errors.New("Wan 2.7 非组图模式最多生成 4 张")
	}
	allow4K := upstream == "wan2.7-image-pro" && imageCount == 0 && !sequential
	size, _ := body["size"].(string)
	if !allow4K && (strings.EqualFold(size, "4K") || body["resolution"] == "4K") {
		return errors.New("Wan 2.7 的 4K 仅支持 Pro 文生单图模式")
	}
	if imageCount > 0 {
		delete(body, "thinking_mode")
	}
	if raw, ok := body["bbox_list"].([]any); ok {
		if len(raw) == 0 {
			delete(body, "bbox_list")
		} else if err := encodeWanBoxes(body, raw, imageCount); err != nil {
			return err
		}
	}
	if palette, ok := body["color_palette"].([]any); ok {
		if len(palette) == 0 {
			delete(body, "color_palette")
		} else if !validWanPalette(palette) {
			return errors.New("Wan 2.7 配色表需要 3 至 10 个六位十六进制色值，占比合计 100%")
		}
	}
	return nil
}

func encodeWanBoxes(body map[string]any, rows []any, imageCount int) error {
	if len(rows) != imageCount {
		return errors.New("Wan 2.7 框选区域需要按顺序与参考图一一对应")
	}
	list := make([][][4]float64, len(rows))
	for i, row := range rows {
		boxes, _ := row.(map[string]any)["boxes"].([]any)
		if len(boxes) > 2 {
			return errors.New("Wan 2.7 每张参考图最多两个框选区域")
		}
		list[i] = make([][4]float64, 0, len(boxes))
		for _, item := range boxes {
			box := item.(map[string]any)
			coords := [4]float64{box["x1"].(float64), box["y1"].(float64), box["x2"].(float64), box["y2"].(float64)}
			if coords[0] >= coords[2] || coords[1] >= coords[3] {
				return errors.New("Wan 2.7 框选右下坐标必须大于左上坐标")
			}
			list[i] = append(list[i], coords)
		}
	}
	body["bbox_list"] = list
	return nil
}

func validWanPalette(items []any) bool {
	if len(items) < 3 || len(items) > 10 {
		return false
	}
	sum := 0.0
	for _, item := range items {
		color := item.(map[string]any)
		hex, ratio := color["hex"].(string), color["ratio"].(string)
		_, hexErr := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 24)
		pct, pctErr := strconv.ParseFloat(strings.TrimSuffix(ratio, "%"), 64)
		if len(hex) != 7 || hex[0] != '#' || hexErr != nil || pctErr != nil ||
			!strings.HasSuffix(ratio, "%") || !(pct >= 0 && pct <= 100) {
			return false
		}
		sum += pct
	}
	return math.Abs(sum-100) < 0.000001
}

func normalizeWanVideoOptions(id string, body map[string]any, fileCount int) error {
	document, _ := body["file_url"].(string)
	page, _ := body["link_url"].(string)
	if strings.HasSuffix(id, "-document") && (document == "") == (page == "") {
		return errors.New("Wan 3.0 需要填写一个参考文档或网页地址")
	}
	if document != "" || page != "" {
		if body["prompt_extend"] == false || (document != "" && !publicHTTPURL(document)) || (page != "" && !publicHTTPURL(page)) {
			return errors.New("Wan 3.0 文档或网页参考需要公网地址并开启提示词改写")
		}
		body["generation_type"] = "reference"
	}
	for _, key := range []string{"file_url", "link_url"} {
		if body[key] == "" {
			delete(body, key)
		}
	}
	if !strings.HasSuffix(id, "-edit") && fileCount == 0 && body["prompt"] == nil && document == "" && page == "" {
		return errors.New("Wan 3.0 需要提示词或参考素材")
	}
	return nil
}

func normalizeKlingOptions(upstream string, body map[string]any, images int) error {
	if body["multi_shot"] == true && body["shot_type"] == "customize" {
		shots, _ := body["multi_prompt"].([]any)
		if len(shots) < 1 || len(shots) > 6 {
			return errors.New("可灵自定义分镜需要 1 至 6 个镜头")
		}
		var sum int
		for i, item := range shots {
			shot := item.(map[string]any)
			seconds, err := strconv.Atoi(shot["duration"].(string))
			if err != nil || seconds < 1 || seconds > 15 || strings.TrimSpace(shot["prompt"].(string)) == "" {
				return fmt.Errorf("可灵分镜 %d 需要提示词及有效的整数秒时长", i+1)
			}
			shot["index"], shot["duration"] = i+1, seconds
			sum += seconds
		}
		if float64(sum) != body["duration"] {
			return errors.New("可灵分镜时长之和必须等于视频时长")
		}
	}
	elements, _ := body["element_list"].([]any)
	if len(elements) == 0 {
		delete(body, "element_list")
		return nil
	}
	if len(elements) > 3 || upstream == "kling-v3" && images == 0 {
		return errors.New("可灵最多支持 3 个主体，可灵 V3 还需连接一张图片")
	}
	for _, item := range elements {
		element := item.(map[string]any)
		urls := element["element_input_urls"].([]any)
		if len(urls) < 2 || len(urls) > 4 || strings.TrimSpace(element["name"].(string)) == "" || strings.TrimSpace(element["description"].(string)) == "" {
			return errors.New("可灵主体需要名称、描述和 2 至 4 张参考图")
		}
		for _, address := range urls {
			if !publicHTTPURL(address.(string)) {
				return errors.New("可灵主体图需要使用公网图片地址")
			}
		}
	}
	return nil
}

func normalizeMidjourneyOptions(body map[string]any, images int) error {
	if body["repeat"] == float64(1) {
		delete(body, "repeat")
	}
	for _, key := range []string{"cref", "sref", "dref"} {
		if address, _ := body[key].(string); address != "" && !publicHTTPURL(address) {
			return fmt.Errorf("Midjourney %s 需要公网图片网址", key)
		}
	}
	if body["iw"] != nil && images == 0 {
		return errors.New("Midjourney 垫图权重需要连接垫图")
	}
	if body["version"] == "8.2" && body["style"] == "raw" {
		return errors.New("Midjourney V8.2 请使用原始风格开关")
	}
	if body["hd"] == true && body["version"] != "8.1" && body["version"] != "8.2" {
		return errors.New("Midjourney 高清模式仅支持 V8.1 和 V8.2")
	}
	return nil
}

func publicHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.User != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate()
	}
	return strings.Contains(host, ".") && !strings.HasSuffix(host, ".localhost") &&
		!strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal")
}

func prepareSuno(id string, body map[string]any, prompt string) (string, error) {
	switch id {
	case "suno":
		custom := body["custom"] == "custom"
		body["custom"] = custom
		instrumental, _ := body["instrumental"].(bool)
		if !custom && (prompt == "" || len([]rune(prompt)) > 3000) || custom && !instrumental && prompt == "" {
			return "", errors.New("Suno 灵感模式需要 3000 字内的描述，自定义人声模式需要歌词")
		}
	case "suno-upload-cover":
		custom := body["custom"] == "custom"
		body["custom"] = custom
		if !custom {
			if description, _ := body["gpt_description"].(string); strings.TrimSpace(description) == "" {
				return "", errors.New("Suno 灵感翻唱需要灵感描述")
			}
			prompt = ""
		}
	}
	if body["version"] == "custom" {
		modelID, _ := body["custom_model_id"].(string)
		if !uuidPattern.MatchString(modelID) {
			return "", errors.New("Suno 自定义模型需要完整的 UUID")
		}
		if persona, _ := body["persona_id"].(string); persona != "" {
			return "", errors.New("Suno 自定义模型与 Persona 不能同时使用")
		}
		delete(body, "version")
	}
	return prompt, nil
}
