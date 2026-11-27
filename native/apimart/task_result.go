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
	media, err := decodeMedia(spec.Model.Capability, raw)
	if err != nil {
		return nil, err
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
