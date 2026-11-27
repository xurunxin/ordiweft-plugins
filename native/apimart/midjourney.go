// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"strconv"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func prepareMidjourneyFollowUp(spec *modelSpec, body map[string]any, source aigc.SourceTask) error {
	operation, _ := body["operation"].(string)
	switch operation {
	case "upscale", "variation", "reroll":
	default:
		return errors.New("invalid Midjourney continuation operation")
	}
	delete(body, "operation")
	body["task_id"] = source.Ticket
	if operation == "reroll" {
		delete(body, "index")
	}
	spec.Endpoint = "/v1/midjourney/generations/" + operation
	return nil
}

func decodeMidjourney(spec modelSpec, raw json.RawMessage) ([]aigc.Media, error) {
	var result struct {
		GridURL   string        `json:"grid_image_url"`
		ImageURLs []string      `json:"image_urls"`
		VideoURLs []string      `json:"video_urls"`
		Images    []remoteMedia `json:"images"`
		Videos    []remoteMedia `json:"videos"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result.Images) > 0 || len(result.Videos) > 0 {
		return decodeMedia(spec.Model.Capability, raw)
	}
	urls, kind := result.ImageURLs, "image"
	if spec.Model.Capability == "video.generate" {
		urls, kind = result.VideoURLs, "video"
	} else if result.GridURL != "" && spec.Model.ID != "midjourney-inpaint" {
		urls = []string{result.GridURL}
	}
	if len(urls) == 0 {
		return decodeMedia(spec.Model.Capability, raw)
	}
	outputs := make([]aigc.Media, 0, len(urls))
	for _, address := range urls {
		if !httpsURL(address) {
			return nil, errors.New("APIMart returned an invalid Midjourney media URL")
		}
		outputs = append(outputs, aigc.Media{Kind: kind, URL: address})
	}
	return outputs, nil
}

type midjourneyButton struct {
	CustomID string `json:"customId"`
	SnakeID  string `json:"custom_id"`
}

var midjourneyHDButtons = map[string][]string{
	"subtle":    {"upsample_v6_2x_subtle", "upsample_v7_2x_subtle"},
	"creative":  {"upsample_v6_2x_creative", "upsample_v7_2x_creative"},
	"legacy_2x": {"upsample_v5_2x"}, "legacy_4x": {"upsample_v5_4x"},
}

func (p *plugin) resolveMidjourneyUpscale(ctx context.Context, key string, body map[string]any) error {
	style, _ := body["upscale_style"].(string)
	delete(body, "upscale_style")
	if style == "" || style == "normal" {
		return nil
	}
	options := midjourneyHDButtons[style]
	index, ok := body["index"].(float64)
	id, _ := body["task_id"].(string)
	if !ok || len(options) == 0 || !taskIDPattern.MatchString(id) {
		return errors.New("invalid Midjourney HD selection")
	}
	var source remoteTask
	if err := p.query(ctx, key, "/v1/tasks/", id, &source); err != nil {
		return err
	}
	if status := strings.ToLower(source.Status); status != "success" && status != "completed" {
		return errors.New("Midjourney source is not complete")
	}
	var nested struct {
		Buttons []midjourneyButton `json:"buttons"`
	}
	if err := json.Unmarshal(source.Result, &nested); err != nil {
		return err
	}
	position := strconv.Itoa(int(index))
	for _, button := range append(source.Buttons, nested.Buttons...) {
		custom := button.CustomID
		if custom == "" {
			custom = button.SnakeID
		}
		parts := strings.Split(custom, "::")
		if len(parts) != 5 || parts[0] != "MJ" || parts[1] != "JOB" || parts[3] != position || parts[4] == "" || len(custom) > 256 {
			continue
		}
		for _, action := range options {
			if parts[2] == action {
				body["custom_id"] = custom
				delete(body, "index")
				return nil
			}
		}
	}
	return errors.New("该原任务未提供所选位置的高清放大按钮")
}

func validateMidjourneyMask(files []attachment) error {
	if len(files) != 1 || files[0].ref.Role != "mask" || len(files[0].input.Data) > 12<<20 {
		return errors.New("Midjourney requires one PNG mask up to 12 MiB")
	}
	data := files[0].input.Data
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > 32000000 {
		return errors.New("invalid Midjourney PNG mask")
	}
	mask, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid Midjourney PNG mask")
	}
	bounds := mask.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := mask.At(x, y).RGBA(); a == 0 {
				return nil
			}
		}
	}
	return errors.New("请在遮罩上标出至少一处重绘区域")
}

func (p *plugin) openMidjourneyModal(ctx context.Context, key, source string, speed any, checkpoint func(string)) (string, error) {
	body, _ := json.Marshal(map[string]any{"task_id": source, "speed": speed})
	reply, err := p.request(ctx, http.MethodPost, "/v1/midjourney/generations/inpaint", key, "application/json", body)
	if err != nil {
		return "", err
	}
	tasks, err := submission(reply)
	if err != nil || len(tasks) != 1 {
		return "", errors.New("APIMart did not return a single Midjourney modal task")
	}
	id := tasks[0]
	checkpoint(id)
	if _, err = p.waitForTask(ctx, key, "/v1/tasks/", id, true); err != nil {
		return "", err
	}
	return id, nil
}
