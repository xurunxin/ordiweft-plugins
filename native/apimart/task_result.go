// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func singleVideoOutput(media []aigc.Media) bool {
	if len(media) == 0 || media[0].Kind != "video" {
		return false
	}
	for _, output := range media[1:] {
		if output.Kind == "video" {
			return false
		}
	}
	return true
}

func decodeTaskMedia(spec modelSpec, body map[string]any, raw json.RawMessage) ([]aigc.Media, error) {
	if spec.Upstream == "MiniMax-H3-Context-IR" {
		var result struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, err
		}
		media := aigc.Media{Kind: "text", Data: []byte(result.Prompt)}
		if err := media.ValidatePromptText(); err != nil {
			return nil, err
		}
		return []aigc.Media{media}, nil
	}
	if spec.Upstream == "midjourney" {
		return decodeMidjourney(spec, raw)
	}
	if spec.Endpoint == "/v1/music/generations/stemsFlowMusic" {
		return decodeFlowStems(raw)
	}
	if spec.Upstream == "flowmusic" {
		return decodeFlowMusic(raw)
	}
	media, err := decodeMedia(spec.Model.Capability, raw)
	if err != nil {
		return nil, err
	}
	if spec.Upstream == "seedream-5-5" && body["layer_decomposition"] == true {
		annotateSeedreamLayers(media, raw)
	}
	if body["enable_gif"] == true {
		if len(media) == 0 {
			return decodeMedia("image.generate", raw)
		}
		for i := range media {
			media[i].Kind = "image"
		}
	}
	if body["return_last_frame"] != true || spec.Model.Capability != "video.generate" || len(media) == 0 {
		return media, nil
	}
	var result struct {
		LastFrameURL string `json:"last_frame_url"`
		Videos       []struct {
			LastFrameURL string `json:"last_frame_url"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	address := strings.TrimSpace(result.LastFrameURL)
	if address == "" && len(result.Videos) == 1 {
		address = strings.TrimSpace(result.Videos[0].LastFrameURL)
	}
	if address != "" {
		if !httpsURL(address) {
			return nil, errors.New("APIMart returned an invalid last-frame URL")
		}
		media = append(media, aigc.Media{Kind: "image", URL: address})
	}
	return media, nil
}

func annotateSeedreamLayers(media []aigc.Media, raw json.RawMessage) {
	var result struct {
		Images []struct {
			URLs   []string          `json:"url"`
			Layers []json.RawMessage `json:"layers"`
		} `json:"images"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return
	}
	layers := make(map[string]json.RawMessage)
	for _, row := range result.Images {
		if len(row.URLs) != len(row.Layers) {
			continue
		}
		for i, address := range row.URLs {
			if _, duplicate := layers[address]; duplicate {
				layers[address] = nil
				continue
			}
			layers[address] = row.Layers[i]
		}
	}
	for i := range media {
		candidate := media[i]
		candidate.ImageLayer = layers[candidate.URL]
		if candidate.ValidateImageLayer() == nil {
			media[i] = candidate
		}
	}
}
