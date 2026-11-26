package tapnowweb

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var kling4Mention = regexp.MustCompile(`@(image|video|element)_\d+`)

func transformKling4(body map[string]any, mode string, parameters map[string]any, prompt string, media map[string][]string) error {
	contents := []any{}
	inputIDs := map[string]bool{}
	add := func(kind string, index int, role, url string) {
		id := fmt.Sprintf("%s_%d", kind, index+1)
		contents = append(contents, map[string]any{"type": role, "url": url, "id": id})
		inputIDs["@"+id] = true
	}
	switch mode {
	case "text_to_video":
		body["kling_operation"] = "text-to-video"
		body["aspect_ratio"] = parameters["aspectRatio"]
	case "image_to_video", "start_end_to_video":
		body["kling_operation"] = "image-to-video"
		images := media["image"]
		for i, url := range images {
			role := "middle_frame"
			if i == 0 {
				role = "first_frame"
			} else if i == len(images)-1 {
				role = "last_frame"
			}
			add("image", i, role, url)
		}
	case "reference_to_video", "reference_video_to_video", "video_edit":
		body["kling_operation"] = "omni-video"
		body["aspect_ratio"] = parameters["aspectRatio"]
		for i, url := range media["image"] {
			add("image", i, "refer_image", url)
		}
		for i, url := range media["video"] {
			role := "feature_video"
			if mode == "video_edit" && i == 0 {
				role = "base_video"
			}
			add("video", i, role, url)
		}
		if audio := media["audio"]; len(audio) > 0 {
			references := make([]any, 0, len(audio))
			for i, url := range audio {
				references = append(references, map[string]any{"url": url, "id": fmt.Sprintf("audio_%d", i+1)})
			}
			body["kling_audio_refs"] = references
		}
	default:
		return errors.New("Kling 4.0 生成方式无效")
	}
	prompt = klingPrompt.ReplaceAllStringFunc(prompt, func(token string) string {
		parts := klingPrompt.FindStringSubmatch(token)
		return "@" + strings.ToLower(parts[1]) + "_" + parts[2]
	})
	for _, mention := range kling4Mention.FindAllString(prompt, -1) {
		if !inputIDs[mention] {
			return errors.New("Kling 4.0 提示词引用未连接的素材")
		}
	}
	body["kling_contents"] = contents
	body["_prompt"] = humanPrompt.ReplaceAllString(prompt, "$1 $2")
	return nil
}
