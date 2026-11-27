// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestAPIMartMultimodalVideoModes(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	image := aigc.Input{Kind: "image", URL: "https://example.com/subject.png"}
	video := aigc.Input{Kind: "video", URL: "https://example.com/motion.mp4"}
	audio := aigc.Input{Kind: "audio", URL: "https://example.com/sound.wav"}
	mixed := []aigc.Input{image, video, audio}
	audioOnly := []aigc.Input{audio}

	for _, tc := range []struct {
		name, model, prompt, projected, taskType, wantError string
		params                                              map[string]any
		inputs                                              []aigc.Input
	}{
		{name: "H3 mixed at provider limit", model: "MiniMax-H3", inputs: mixed,
			prompt:    strings.Repeat("声", 6991) + "{{Image 1}}{{Video 1}}{{Audio 1}}",
			projected: strings.Repeat("声", 6991) + "图片1视频1音频1"},
		{name: "H3 too long", model: "MiniMax-H3", inputs: mixed, prompt: strings.Repeat("声", 7001), wantError: "prompt too long"},
		{name: "H3 requires visual alongside audio", model: "MiniMax-H3", inputs: audioOnly, wantError: "reference audio requires"},
		{name: "H3 Max 1080P receives mixed references", model: "MiniMax-H3-Max", inputs: mixed,
			params: map[string]any{"resolution": "1080P", "watermark": true}},
		{name: "H3 supports last frame alone", model: "MiniMax-H3-frames", inputs: []aigc.Input{{Kind: "image", Role: "last_frame", URL: image.URL}}},
		{name: "H3 frame mode requires at least one frame", model: "MiniMax-H3-frames", wantError: "requires at least"},
		{name: "Seedance 2.0 audio needs visual", model: "seedance-2.0", inputs: audioOnly, wantError: "reference audio requires"},
		{name: "Seedance 2.0 standard upper limit", model: "seedance-2.0", inputs: mixed, prompt: strings.Repeat("声", 4001), wantError: "prompt too long"},
		{name: "Seedance 2.0 Mini longer prompt", model: "seedance-2.0-mini", inputs: mixed, prompt: strings.Repeat("声", 7001)},
		{name: "Seedance 2.5 three types", model: "seedance-2.5", inputs: mixed,
			prompt:    strings.Repeat("舟", 9000) + "{{Image 1}}{{Video 1}}{{Audio 1}}",
			projected: strings.Repeat("舟", 9000) + "@图片1@视频1@音频1", taskType: "reference"},
		{name: "Seedance 2.5 Flash pure audio", model: "seedance-2-5-flash", inputs: audioOnly, taskType: "reference"},
		{name: "Seedance 2.5 draft pure audio", model: "seedance-2.5-draft", inputs: audioOnly, taskType: "reference"},
		{name: "Seedance 2.5 single video reference", model: "seedance-2.5", inputs: []aigc.Input{video}, taskType: "reference"},
		{name: "Seedance 2.5 single video edit", model: "seedance-2.5-edit", inputs: []aigc.Input{video}, taskType: "edit"},
		{name: "Seedance 2.5 text only", model: "seedance-2.5"},
		{name: "Wan 3.0 image-only reference", model: "wan3.0-video", inputs: []aigc.Input{image},
			prompt: "{{Image 1}}", projected: "图1"},
		{name: "Kling V4 mixed reference", model: "kling-v4-reference", inputs: mixed,
			prompt: "{{Image 1}}{{Video 1}}{{Audio 1}}", projected: "@Image1@Video1@Audio1", taskType: "reference"},
		{name: "APIMart Zoe requires visual alongside audio", model: "apimart-zoe", inputs: audioOnly, wantError: "reference audio requires"},
		{name: "MAGI-2 pure audio", model: "magi-2-reference", inputs: audioOnly, taskType: "reference"},
		{name: "MAGI-2 above provider limit", model: "magi-2-reference", inputs: audioOnly, prompt: strings.Repeat("声", 15001), wantError: "prompt too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := tc.prompt
			if prompt == "" {
				prompt = "A paper boat in the rain"
			}
			_, body, files, err := p.prepare(generation(t, tc.model, "video.generate", prompt, tc.params, tc.inputs))
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("wanted %q, got %v", tc.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != len(tc.inputs) {
				t.Fatalf("received %d reference attachments, want %d", len(files), len(tc.inputs))
			}
			want := tc.projected
			if want == "" {
				want = prompt
			}
			if body["prompt"] != want {
				t.Fatal("provider received a different prompt")
			}
			if mode, _ := body["omni_reference_task_type"].(string); mode != tc.taskType {
				t.Fatalf("provider mode %q, want %q", mode, tc.taskType)
			}
			if tc.model == "wan3.0-video" && body["generation_type"] != "reference" {
				t.Fatal("Wan reference images must stay in the reference family")
			}
			if tc.model == "MiniMax-H3-Max" && body["watermark"] != nil {
				t.Fatal("H3 Max 1080P must omit watermark")
			}
			if tc.model == "MiniMax-H3-frames" && files[0].ref.Field != "last_frame_image" {
				t.Fatal("explicit last frame was assigned to a different field")
			}
		})
	}
}

func TestDocumentedVideoModeClassifications(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"MiniMax-H3", "MiniMax-H3-Max"} {
		main := p.models[model].Model.GenerationMode
		frame := p.models[model+"-frames"].Model.GenerationMode
		if main == nil || main.TextLabel != "文生视频" || main.Label != "多模态参考生视频" || main.Intent != "reference" ||
			frame == nil || frame.Label != "图生视频" || frame.MinInputs != 1 || frame.Images.Max != 2 {
			t.Fatalf("%s does not expose the three documented APIMart modes", model)
		}
	}
	for _, model := range []string{"seedance-2.5", "seedance-2-5-flash", "kling-v4", "kling-v4-flash", "apimart-zoe"} {
		edit := p.models[model+"-edit"].Model.GenerationMode
		if edit == nil || edit.Intent != "edit" || !edit.ManualOnly {
			t.Fatalf("%s routes an ordinary video attachment to edit", model)
		}
	}
	for _, id := range []string{"viduq4-preview", "wan3.0-video-edit", "wan3.0-video-prime-edit"} {
		if model := p.models[id].Model; model.ID != id || !model.Hidden || model.GenerationMode != nil {
			t.Fatalf("expected %s to remain addressable only as a legacy model", id)
		}
	}
}
