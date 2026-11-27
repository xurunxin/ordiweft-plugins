// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func applyGenerationModes(catalog []modelSpec) {
	type familyKey struct{ capability, upstream string }
	groups := map[familyKey][]int{}
	for i, spec := range catalog {
		if !spec.Model.Hidden && spec.Model.TaskContinuation == nil {
			key := familyKey{spec.Model.Capability, spec.Upstream}
			groups[key] = append(groups[key], i)
		}
	}
	for key, group := range groups {
		if len(group) < 2 {
			continue
		}
		familyLabel := key.upstream
		for _, i := range group {
			if model := catalog[i].Model; model.ID == key.upstream {
				familyLabel, _, _ = strings.Cut(model.Label, " · ")
				break
			}
		}
		for _, i := range group {
			spec := &catalog[i]
			mode := generationMode(*spec)
			mode.Family, mode.FamilyLabel = key.upstream, familyLabel
			spec.Model.GenerationMode = &mode
			if spec.Model.ID == "kling-v4-reference" {
				spec.Model.MaxInputs = min(spec.Model.MaxInputs, 15)
			}
			if spec.Model.ID == "suno" {
				custom := spec.Model.Fields["custom"]
				custom.Label = "创作方式"
				spec.Model.Fields["custom"] = custom
			}
		}
	}
}

func generationMode(spec modelSpec) aigc.ModelGenerationMode {
	mode := aigc.ModelGenerationMode{Intent: "text"}
	ranges := map[string]*aigc.InputRange{"image": &mode.Images, "video": &mode.Videos, "audio": &mode.Audios}
	var first, last bool
	for _, ref := range spec.References {
		limit := ranges[ref.Kind]
		limit.Min += ref.Min
		limit.Max += ref.Max
		first = first || ref.Role == "first_frame"
		last = last || ref.Role == "last_frame"
	}
	_, mode.Label, _ = strings.Cut(spec.Model.Label, " · ")
	switch {
	case strings.Contains(mode.Label, "续写"):
		mode.Intent = "extend"
	case strings.Contains(mode.Label, "视频编辑"):
		mode.Intent = "edit"
	case first && last:
		mode.Intent = "first_last_frame"
	case first:
		mode.Intent = "first_frame"
	case len(spec.References) > 0:
		mode.Intent = "reference"
	}
	if mode.Label == "" {
		switch mode.Intent {
		case "first_last_frame":
			mode.Label = "文字 / 首尾帧"
		case "first_frame":
			mode.Label = "文字 / 首帧"
		case "reference":
			mode.Label = "文字 / 素材参考"
		default:
			mode.Label = "文生视频"
		}
		if spec.Model.Capability == "music.generate" {
			mode.Label = "生成音乐"
		}
	}
	// Drafts and alternate music actions require intent beyond the media type.
	draft, _ := spec.Fixed["draft"].(bool)
	mode.ManualOnly = draft || spec.Upstream == "flowmusic" && spec.Model.ID != spec.Upstream
	switch spec.Model.ID {
	case "suno-sounds", "suno-upload-cover", "suno-upload-extend":
		mode.ManualOnly = true
	case "magi-2-reference":
		mode.MinInputs = 1
	}
	return mode
}
