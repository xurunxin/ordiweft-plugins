// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestDocumentedWireFieldsForRemainingModes(t *testing.T) {
	image := aigc.Input{Kind: "image", URL: "https://example.com/actor.png"}
	video := aigc.Input{Kind: "video", URL: "https://example.com/motion.mp4"}
	audio := aigc.Input{Kind: "audio", URL: "https://example.com/beat.wav"}
	actor := []any{map[string]any{"tag": "@Image1", "type": "image", "image_urls": []any{image.URL}}}
	motion := []any{map[string]any{"tag": "@Video1", "type": "reference", "video_url": video.URL}}
	klingVideo := []any{map[string]any{"refer_type": "feature", "video_url": video.URL, "keep_original_sound": "no"}}
	for _, tc := range []struct {
		name, model, prompt string
		params              map[string]any
		inputs              []aigc.Input
		want                map[string]any
		skip                []string
		ticket              bool
	}{
		{name: "Veo text omits routing field", model: "veo3.1-fast", params: map[string]any{"generation_type": "text-to-video"}, skip: []string{"generation_type", "image_urls"}},
		{name: "Veo frame uses documented routing value", model: "veo3.1-fast", params: map[string]any{"generation_type": "image-to-video"},
			inputs: []aigc.Input{image}, want: map[string]any{"generation_type": "frame", "image_urls": []any{image.URL}}},
		{name: "Veo historic missing routing field infers shape", model: "veo3.1-fast", inputs: []aigc.Input{image, image}, skip: []string{"generation_type"}},
		{name: "SkyReels uses typed reference objects", model: "skyreels-v4-std-reference", prompt: "Use {{Image 1}} as the actor",
			inputs: []aigc.Input{image, video}, want: map[string]any{"ref_images": actor, "ref_videos": motion, "prompt": "Use @Image1 as the actor @Video1"}, skip: []string{"image_urls", "video_urls"}},
		{name: "SkyReels voice belongs to the subject", model: "skyreels-v4-fast-voice", prompt: "Use {{Audio 1}}",
			inputs: []aigc.Input{image, audio}, want: map[string]any{"ref_images": []any{
				map[string]any{"tag": "@Image1", "type": "image", "image_urls": []any{image.URL}, "audio_url": audio.URL},
			}}, skip: []string{"audio_url"}},
		{name: "Kling distinguishes feature reference", model: "kling-v3-omni-feature", prompt: "Follow {{Video 1}}",
			inputs: []aigc.Input{video}, want: map[string]any{"video_list": klingVideo, "prompt": "Follow <<<video_1>>>"}, skip: []string{"video_urls"}},
		{name: "Wan text with soundtrack", model: "wan2.7", inputs: []aigc.Input{audio},
			want: map[string]any{"audio_url": audio.URL}, skip: []string{"image_urls", "video_urls"}},
		{name: "PixVerse single image", model: "pixverse-v6-i2v", inputs: []aigc.Input{image},
			want: map[string]any{"image_urls": []any{image.URL}}, skip: []string{"first_frame_image", "last_frame_image"}},
		{name: "PixVerse legacy first frame remains usable", model: "pixverse-v6", inputs: []aigc.Input{image},
			want: map[string]any{"image_urls": []any{image.URL}}, skip: []string{"first_frame_image"}},
		{name: "PixVerse bound task accepts new prompt", model: "pixverse-v6-extend", prompt: "The paper boat approaches the shore", ticket: true,
			want: map[string]any{"extend_from_task_id": "task_original", "prompt": "The paper boat approaches the shore"}, skip: []string{"video_urls", "image_urls"}},
		{name: "Gemini 1.1 uses the metadata task", model: "gemini-omni-1.1-flash-task-extend", prompt: "Add snowfall", ticket: true,
			want: map[string]any{"metadata": map[string]any{"task": "extend"}, "extend_from_task_id": "task_original"},
			skip: []string{"omni_reference_task_type", "duration"}},
		{name: "Gemini Ext continuation sends declared operation", model: "gemini-omni-1.2-flash-ext-task-extend", prompt: "Add snowfall", ticket: true,
			want: map[string]any{"extend_from_task_id": "task_original", "omni_reference_task_type": "extend", "prompt": "Add snowfall"}, skip: []string{"video_urls"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := loadPlugin()
			if err != nil {
				t.Fatal(err)
			}
			var submitted atomic.Int32
			local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST /v1/videos/generations":
					submitted.Add(1)
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					for key, want := range tc.want {
						if !reflect.DeepEqual(body[key], want) {
							t.Errorf("%s = %#v, want %#v", key, body[key], want)
						}
					}
					for _, key := range tc.skip {
						if _, ok := body[key]; ok {
							t.Errorf("unexpected %s", key)
						}
					}
					io.WriteString(w, `{"code":200,"data":[{"task_id":"task_fixture","status":"submitted"}]}`)
				case "GET /v1/tasks/task_fixture":
					io.WriteString(w, `{"code":200,"data":{"id":"task_fixture","status":"completed","result":{"videos":[{"url":["https://example.com/result.mp4"]}]}}}`)
				default:
					t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer local.Close()
			p.base = local.URL
			prompt := tc.prompt
			if prompt == "" {
				prompt = "A short scene"
			}
			request := generation(t, tc.model, "video.generate", prompt, tc.params, tc.inputs)
			if tc.ticket {
				request.SourceTask = &aigc.SourceTask{Ticket: "task_original"}
			}
			if _, err := p.Execute(t.Context(), request, nil); err != nil {
				t.Fatal(err)
			}
			if submitted.Load() != 1 {
				t.Fatalf("submitted %d requests, want one", submitted.Load())
			}
		})
	}
}

// documentContractCase verifies the emitted wire fields, including compatibility
// values retained in older saved canvases.
func TestDocumentedModelContractRequests(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	const image = "https://example.com/reference.png"
	const video = "https://example.com/source.mp4"
	const audio = "https://example.com/voice.wav"
	for _, tc := range []struct {
		model, capability, endpoint, responseVersion string
		parameters                                   map[string]any
		inputs                                       []aigc.Input
		want                                         map[string]any
		omit                                         []string
		source                                       bool
	}{
		{model: "gemini-3.1-flash-lite-image-ext", capability: "image.generate", want: map[string]any{"resolution": "1K", "n": 1}, omit: []string{"official_fallback", "google_search"}},
		{model: "grok-imagine-image-2.0", capability: "image.generate", responseVersion: "2026-07-27",
			parameters: map[string]any{"quality": "low"}, inputs: []aigc.Input{{Kind: "image", URL: image}},
			want: map[string]any{"image_urls": []string{image}}, omit: []string{"quality"}},
		{model: "veo3.1-fast-official", capability: "video.generate", parameters: map[string]any{"sample_count": 2, "person_generation": "disallow", "resize_mode": "crop"},
			want: map[string]any{"sample_count": 2, "person_generation": "disallow"}, omit: []string{"resize_mode"}},
		{model: "veo3.1-quality-official-frames", capability: "video.generate", parameters: map[string]any{"resize_mode": "crop"},
			inputs: []aigc.Input{{Kind: "image", Role: "last_frame", URL: image}}, want: map[string]any{"last_frame_image": image, "resize_mode": "crop"}},
		{model: "veo3.1-fast-remix", capability: "video.generate", endpoint: "/v1/videos/task_original/remix", source: true,
			want: map[string]any{"model": "veo3.1-fast", "prompt": "Scene description"}, omit: []string{"source_task_id", "duration"}},
		{model: "happyhorse-1.1-frames", capability: "video.generate", inputs: []aigc.Input{{Kind: "image", Role: "first_frame", URL: image}},
			want: map[string]any{"first_frame_image": image, "resolution": "1080P"}, omit: []string{"size", "image_urls"}},
		{model: "wan2.7-r2v", capability: "video.generate",
			inputs: []aigc.Input{{Kind: "image", Role: "reference_image_1", URL: image}, {Kind: "audio", Role: "reference_voice_1", URL: audio}, {Kind: "image", Role: "first_frame", URL: image}},
			want:   map[string]any{"image_with_roles": []any{map[string]any{"url": image, "role": "reference_image", "reference_voice": audio}, map[string]any{"url": image, "role": "first_frame"}}},
			omit:   []string{"reference_voice", "size", "audio_urls"}},
		{model: "wan2.7-videoedit", capability: "video.generate", inputs: []aigc.Input{{Kind: "video", Role: "source_video", URL: video}},
			parameters: map[string]any{"audio_setting": "keep"}, want: map[string]any{"metadata": map[string]string{"audio_setting": "origin"}}, omit: []string{"audio_setting"}},
		{model: "wan2.6-i2v", capability: "video.generate", inputs: []aigc.Input{{Kind: "image", URL: image}},
			parameters: map[string]any{"duration": 9, "audio": false}, want: map[string]any{"duration": 9, "resolution": "1080p"}, omit: []string{"audio"}},
		{model: "wan2.6-i2v-flash", capability: "video.generate", inputs: []aigc.Input{{Kind: "image", URL: image}},
			parameters: map[string]any{"audio": false}, want: map[string]any{"audio": false, "resolution": "1080p"}},
		{model: "kling-v3-motion-control", capability: "video.generate", inputs: []aigc.Input{{Kind: "image", Role: "first_frame", URL: image}, {Kind: "video", Role: "source_video", URL: video}},
			parameters: map[string]any{"watermark_info": map[string]any{"enabled": true}}, want: map[string]any{"watermark_info": map[string]bool{"enabled": true}}},
		{model: "grok-imagine-video-edit", capability: "video.generate", inputs: []aigc.Input{{Kind: "video", Role: "source_video", URL: video}},
			parameters: map[string]any{"resolution": "480p"}, want: map[string]any{"video": map[string]string{"url": video}}, omit: []string{"duration", "resolution", "image_urls"}},
		{model: "skyreels-v4-std", capability: "video.generate", inputs: []aigc.Input{{Kind: "image", Role: "mid_frame_1", URL: image}},
			parameters: map[string]any{"mid_frame_times": map[string]any{"mid_frame_1": 2}},
			want:       map[string]any{"mid_frame_images": []any{map[string]any{"tag": "@Image1", "image_url": image, "time_stamp": 2}}}, omit: []string{"mid_frame_times"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var submitted atomic.Int32
			endpoint := tc.endpoint
			if endpoint == "" {
				endpoint = "/v1/videos/generations"
				if tc.capability == "image.generate" {
					endpoint = "/v1/images/generations"
				}
			}
			local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST " + endpoint:
					submitted.Add(1)
					if got := r.Header.Get("X-APIMart-Response-Version"); got != tc.responseVersion {
						t.Errorf("response version = %q want %q", got, tc.responseVersion)
					}
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					for k, v := range tc.want {
						want, _ := json.Marshal(v)
						if string(body[k]) != string(want) {
							t.Errorf("%s = %s; want %s", k, body[k], want)
						}
					}
					for _, k := range tc.omit {
						if _, ok := body[k]; ok {
							t.Errorf("unexpected request field %s", k)
						}
					}
					if tc.responseVersion != "" {
						w.WriteHeader(http.StatusAccepted)
						io.WriteString(w, "{\"code\":202,\"data\":{\"id\":\"task_fixture\",\"object\":\"generation.task\",\"status\":\"pending\",\"poll_url\":\"https://example.com/untrusted\"}}")
					} else {
						io.WriteString(w, "{\"code\":200,\"data\":[{\"task_id\":\"task_fixture\"}]}")
					}
				case "GET /v1/tasks/task_fixture":
					io.WriteString(w, "{\"code\":200,\"data\":{\"id\":\"task_fixture\",\"status\":\"completed\",\"result\":{\"images\":[{\"url\":\"https://example.com/output.png\"}],\"videos\":[{\"url\":\"https://example.com/output.mp4\"}]}}}")
				default:
					t.Errorf("unexpected route: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer local.Close()
			p.base = local.URL
			req := generation(t, tc.model, tc.capability, "Scene description", tc.parameters, tc.inputs)
			if tc.source {
				req.SourceTask = &aigc.SourceTask{Ticket: "task_original"}
			}
			if _, err := p.Execute(t.Context(), req, nil); err != nil {
				t.Fatal(err)
			}
			if submitted.Load() != 1 {
				t.Fatalf("submitted %d generation requests", submitted.Load())
			}
		})
	}
}

func TestModelContractRejectsUnsupportedReferences(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		model  string
		params map[string]any
		inputs []aigc.Input
	}{
		{model: "wan2.7-r2v", inputs: []aigc.Input{{Kind: "image", Role: "reference_image_2"}, {Kind: "audio", Role: "reference_voice_1"}}},
		{model: "wan2.7-r2v", params: map[string]any{"duration": 11}, inputs: []aigc.Input{{Kind: "video"}}},
		{model: "skyreels-v4-std", params: map[string]any{"duration": 5, "mid_frame_times": map[string]any{"mid_frame_1": 5}}, inputs: []aigc.Input{{Kind: "image", Role: "mid_frame_1"}}},
		{model: "grok-imagine-video-1.5-edit", inputs: []aigc.Input{{Kind: "video", Role: "source_video"}}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			request := generation(t, tc.model, "video.generate", "Test", tc.params, tc.inputs)
			if _, _, _, err := p.prepare(request); err == nil {
				t.Fatal("invalid model options were accepted")
			}
		})
	}
}
