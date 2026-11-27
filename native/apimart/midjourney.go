// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"

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
	} else if result.GridURL != "" {
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
