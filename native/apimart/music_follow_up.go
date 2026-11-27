// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"strings"
)

var sunoFollowUpPaths = map[string]string{
	"extend": "extend", "cover": "coverSong", "replace": "replaceMusic", "stem": "stems", "stems": "stemsAll",
}
var flowFollowUpPaths = map[string]string{"extend": "extendFlowMusic", "cover": "coverFlowMusic", "replace": "replaceFlowMusic", "stems": "stemsFlowMusic"}

func prepareMusicFollowUp(spec *modelSpec, body map[string]any, source *aigc.SourceTask) error {
	operation, _ := body["operation"].(string)
	delete(body, "operation")
	path := ""
	if spec.Upstream == "suno" {
		path = sunoFollowUpPaths[operation]
		if source != nil {
			body["task_id"], body["audio_index"] = source.Ticket, source.ItemIndex+1
		}
		if operation == "cover" || operation == "extend" {
			custom := body["custom"] == "custom"
			body["custom"] = custom
			if !custom && operation == "cover" {
				description, _ := body["gpt_description"].(string)
				if strings.TrimSpace(description) == "" {
					return errors.New("Suno cover requires an inspiration description")
				}
			}
		}
	} else {
		path = flowFollowUpPaths[operation]
		if source != nil {
			body["clip_id"] = source.Ticket
		}
		if seed, ok := body["replacement_seed"]; ok {
			body["seed"] = seed
			delete(body, "replacement_seed")
		}
		instruction, _ := body["instruction"].(string)
		if operation != "stems" && strings.TrimSpace(instruction) == "" {
			return errors.New("Flow Music requires an editing instruction")
		}
	}
	if path == "" {
		return errors.New("invalid APIMart music operation")
	}
	if operation == "replace" {
		start, startOK := body["start_s"].(float64)
		end, endOK := body["end_s"].(float64)
		if !startOK || !endOK || end <= start {
			return errors.New("music replacement end must follow its start")
		}
	}
	spec.Endpoint = "/v1/music/generations/" + path
	return nil
}

type musicTrack struct {
	AudioURL string `json:"audio_url"`
	WAVURL   string `json:"wav_url"`
	ClipID   string `json:"clip_id"`
}

func (t musicTrack) address(upstream string) string {
	if upstream == "flowmusic" && t.WAVURL != "" {
		return t.WAVURL
	}
	return t.AudioURL
}

func musicTracks(raw json.RawMessage) ([]musicTrack, error) {
	var result struct {
		Tracks []musicTrack `json:"music"`
	}
	err := json.Unmarshal(raw, &result)
	return result.Tracks, err
}

func decodeFlowMusic(raw json.RawMessage) ([]aigc.Media, error) {
	tracks, err := musicTracks(raw)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return decodeMedia("music.generate", raw)
	}
	var result []aigc.Media
	seen := map[string]bool{}
	for _, track := range tracks {
		address := track.address("flowmusic")
		if address == "" || seen[address] {
			continue
		}
		if !httpsURL(address) {
			return nil, errors.New("APIMart returned an invalid audio URL")
		}
		seen[address] = true
		result = append(result, aigc.Media{Kind: "audio", URL: address})
	}
	return result, nil
}

func taskOrigins(spec modelSpec, task remoteTask, outputs []aigc.Media, start int) []aigc.TaskOrigin {
	if spec.Model.Capability == "video.generate" {
		if !singleVideoOutput(outputs) || start >= 16 {
			return nil
		}
		return []aigc.TaskOrigin{{SourceTask: aigc.SourceTask{Ticket: task.ID, OutputIndex: start}, CreatedAt: task.Created * 1000}}
	}
	if spec.Model.Capability == "image.generate" && len(outputs) == 1 && start < 16 {
		return []aigc.TaskOrigin{{SourceTask: aigc.SourceTask{Ticket: task.ID, OutputIndex: start}, CreatedAt: task.Created * 1000}}
	}
	tracks, err := musicTracks(task.Result)
	if spec.Model.Capability != "music.generate" || err != nil {
		return nil
	}
	var origins []aigc.TaskOrigin
	for index, output := range outputs {
		candidate := aigc.SourceTask{OutputIndex: start + index}
		for i, track := range tracks {
			if track.address(spec.Upstream) != output.URL {
				continue
			}
			if candidate.Ticket != "" {
				candidate.Ticket = ""
				break
			}
			candidate.Ticket, candidate.ItemIndex = task.ID, i
			if spec.Upstream == "flowmusic" {
				if !uuidPattern.MatchString(track.ClipID) {
					candidate.Ticket = ""
					break
				}
				candidate.Ticket = track.ClipID
			}
		}
		if candidate.Valid() {
			origins = append(origins, aigc.TaskOrigin{SourceTask: candidate, CreatedAt: task.Created * 1000})
		}
	}
	return origins
}

func decodeFlowStems(raw json.RawMessage) ([]aigc.Media, error) {
	var result struct {
		Tracks []struct {
			URL     string `json:"url"`
			FileURL string `json:"file_url"`
			Mime    string `json:"mime_type"`
		} `json:"music"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Tracks) != 1 {
		return nil, errors.New("invalid Flow Music stem archive")
	}
	track := result.Tracks[0]
	address := track.FileURL
	if address == "" {
		address = track.URL
	}
	if track.Mime != "application/zip" || !httpsURL(address) {
		return nil, errors.New("invalid Flow Music stem archive URL")
	}
	return []aigc.Media{{Kind: "audio-archive", URL: address}}, nil
}
