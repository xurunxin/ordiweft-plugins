// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestDocumentedModelParameters(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	image := aigc.Input{Kind: "image", URL: "https://example.com/reference.png"}
	const uuid = "e5c1d19e-c022-427b-92b5-d529510249e5"
	for _, tc := range []struct {
		model, capability, prompt string
		params                    map[string]any
		inputs                    []aigc.Input
		want                      map[string]any
		omit                      []string
		bound, rejected           bool
	}{
		{model: "gemini-nano-banana-2.1-ext", capability: "image.generate", prompt: "A boat", params: map[string]any{"official_fallback": true, "nsfw_check": true},
			want: map[string]any{"official_fallback": true, "nsfw_check": true, "n": 1}},
		{model: "gpt-image-2.6-flare", capability: "image.generate", prompt: "A vector icon", params: map[string]any{"output_format": "svg", "complexity": "high",
			"svg": map[string]any{"model": "gpt-5.4-mini", "reasoning": map[string]any{"effort": "low"}}},
			want: map[string]any{"output_format": "svg", "complexity": "high", "svg": map[string]any{"model": "gpt-5.4-mini", "reasoning": map[string]any{"effort": "low"}}}},
		{model: "gpt-image-2.6-ext", capability: "image.generate", prompt: "A vector icon", inputs: []aigc.Input{image},
			params: map[string]any{"output_format": "svg"}, rejected: true},
		{model: "qwen-image-3.0", capability: "image.generate", prompt: "A focused shot", inputs: []aigc.Input{image},
			params: map[string]any{"prompt_extend": true, "prompt_extend_mode": "agent"}, rejected: true},
		{model: "wan2.7-image-pro", capability: "image.generate", prompt: "Add a red lamp", inputs: []aigc.Input{image, image},
			params: map[string]any{"bbox_list": []any{map[string]any{}, map[string]any{"boxes": []any{map[string]any{"x1": 3, "y1": 5, "x2": 16, "y2": 29}}}}},
			want:   map[string]any{"bbox_list": []any{[]any{}, []any{[]any{3, 5, 16, 29}}}}},
		{model: "wan2.7-image", capability: "image.generate", prompt: "A scene", params: map[string]any{"n": 5}, rejected: true},
		{model: "seedream-5-5", capability: "image.generate", inputs: []aigc.Input{image},
			params: map[string]any{"layer_decomposition": true, "text_recognition": true, "layer_decomposition_options": map[string]any{"max_images": 75}},
			want:   map[string]any{"layer_decomposition": true, "text_recognition": true, "n": 1, "layer_decomposition_options": map[string]any{"max_images": 75}},
			omit:   []string{"prompt", "background"}},
		{model: "seedance-2.5", capability: "video.generate", prompt: "A boat", params: map[string]any{"duration": -1, "web_search": true, "output_format": "mov"},
			want: map[string]any{"tools": []any{map[string]any{"type": "web_search"}}, "duration": -1, "output_format": "mov"}, omit: []string{"web_search"}},
		{model: "seedance-2.5-final-1080p", capability: "video.generate", bound: true, params: map[string]any{"resolution": "4k", "return_last_frame": true, "output_format": "mov"},
			want: map[string]any{"draft_task_id": "task_original", "resolution": "4k", "return_last_frame": true, "output_format": "mov"}, omit: []string{"prompt", "duration", "seed", "tools"}},
		{model: "apimart-zoe-final", capability: "video.generate", bound: true,
			want: map[string]any{"draft_task_id": "task_original", "resolution": "1080p"}, omit: []string{"prompt", "duration", "draft", "output_format", "return_last_frame"}},
		{model: "flux-3-video-final", capability: "video.generate", bound: true, params: map[string]any{"resolution": "fhd"},
			want: map[string]any{"draft_from_task_id": "task_original", "resolution": "fhd"}, omit: []string{"draft", "prompt", "nsfw_check"}},
		{model: "wan3.0-video-document", capability: "video.generate", params: map[string]any{"file_url": "https://example.com/script.pdf", "duration": -1},
			want: map[string]any{"generation_type": "reference", "file_url": "https://example.com/script.pdf", "duration": -1}, omit: []string{"prompt", "link_url"}},
		{model: "wan3.0-video-document", capability: "video.generate", params: map[string]any{"file_url": "https://example.com/script.pdf", "link_url": "https://example.com/article"}, rejected: true},
		{model: "kling-v3", capability: "video.generate", params: map[string]any{"multi_shot": true, "duration": 5, "multi_prompt": []any{
			map[string]any{"prompt": "Wide", "duration": "2"}, map[string]any{"prompt": "Close", "duration": "3"},
		}}, want: map[string]any{"shot_type": "customize", "multi_prompt": []any{
			map[string]any{"index": 1, "prompt": "Wide", "duration": 2}, map[string]any{"index": 2, "prompt": "Close", "duration": 3},
		}}},
		{model: "kling-v3", capability: "video.generate", params: map[string]any{"multi_shot": true, "duration": 5, "multi_prompt": []any{
			map[string]any{"prompt": "Wide", "duration": "3"},
		}}, rejected: true},
		{model: "pixverse-v6", capability: "video.generate", prompt: "Transitions", params: map[string]any{"generate_multi_clip_switch": true},
			want: map[string]any{"generate_multi_clip_switch": true, "motion_mode": "normal"}},
		{model: "suno-inspiration", capability: "music.generate", prompt: "Lullaby", inputs: []aigc.Input{{Kind: "audio", URL: "https://example.com/guide.mp3"}},
			params: map[string]any{"version": "custom", "custom_model_id": uuid, "auto_lyrics": true, "weirdness": 0.4},
			want:   map[string]any{"custom_model_id": uuid, "auto_lyrics": true, "weirdness": 0.4}, omit: []string{"version", "weirdness_constraint"}},
		{model: "suno-upload-cover", capability: "music.generate", prompt: "Old lyrics", inputs: []aigc.Input{{Kind: "audio", URL: "https://example.com/guide.mp3", Role: "source_audio"}},
			params: map[string]any{"custom": "inspiration", "gpt_description": "Slow jazz", "duration_s": 30},
			want:   map[string]any{"custom": false, "gpt_description": "Slow jazz"}, omit: []string{"prompt", "duration_s"}},
	} {
		t.Run(tc.model+"/"+fmt.Sprint(tc.rejected), func(t *testing.T) {
			request := generation(t, tc.model, tc.capability, tc.prompt, tc.params, tc.inputs)
			if tc.bound {
				request.SourceTask = &aigc.SourceTask{Ticket: "task_original"}
			}
			_, body, _, err := p.prepare(request)
			if tc.rejected {
				if err == nil {
					t.Fatal("invalid option combination was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(body)
			var wire map[string]any
			if err := json.Unmarshal(actual, &wire); err != nil {
				t.Fatal(err)
			}
			for key, want := range tc.want {
				actual, err := json.Marshal(wire[key])
				if err != nil {
					t.Fatal(err)
				}
				expected, _ := json.Marshal(want)
				if string(actual) != string(expected) {
					t.Errorf("%s = %s; want %s", key, actual, expected)
				}
			}
			for _, key := range tc.omit {
				if _, ok := wire[key]; ok {
					t.Errorf("unexpected wire parameter %s", key)
				}
			}
		})
	}
}

func TestDocumentedSupplementalFrames(t *testing.T) {
	const video = "https://example.com/result.mp4"
	const frame = "https://example.com/last.png"
	for _, raw := range []string{
		"{\"videos\":[{\"url\":[\"" + video + "\"],\"last_frame_url\":\"" + frame + "\"}]}",
		"{\"videos\":[{\"url\":[\"" + video + "\"]}],\"last_frame_url\":\"" + frame + "\"}",
	} {
		spec := modelSpec{Model: aigc.Model{Capability: "video.generate"}}
		result, err := decodeTaskMedia(spec, map[string]any{"return_last_frame": true}, json.RawMessage(raw))
		want := []aigc.Media{{Kind: "video", URL: video}, {Kind: "image", URL: frame}}
		if err != nil || !reflect.DeepEqual(result, want) || !singleVideoOutput(result) {
			t.Fatalf("mixed output lost the last frame or video ancestry: %#v %v", result, err)
		}
	}
}

func TestWhisperUsesDocumentedMultipartOptions(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		for k, v := range map[string]string{"model": "whisper-1", "response_format": "verbose_json", "language": "zh", "prompt": "专有名词", "temperature": "0.25"} {
			if got := r.FormValue(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
		fmt.Fprint(w, "{\"text\":\"test\",\"duration\":2,\"segments\":[{\"start\":0,\"end\":2,\"text\":\"test\"}]}")
	}))
	defer local.Close()
	p.base = local.URL
	request := generation(t, "whisper-1", "speech.transcribe", "", map[string]any{"language": "zh", "prompt": "专有名词", "temperature": 0.25},
		[]aigc.Input{{Kind: "audio", Data: []byte("RIFFzzzzWAVEfmt ")}})
	result, err := p.Execute(t.Context(), request, nil)
	if err != nil || result.Transcription == nil || len(result.Transcription.Segments) != 1 {
		t.Fatalf("transcription response was not normalized: %#v %v", result, err)
	}
}
