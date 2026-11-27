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
