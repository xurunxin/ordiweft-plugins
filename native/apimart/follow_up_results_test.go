// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestContextIRReturnsPromptAndTokenReceipt(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	original := "A paper boat"
	enhanced := "镜头一\n固定机位，纸船慢慢推近。"
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/videos/generations":
			calls++
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "MiniMax-H3-Context-IR" || body["prompt"] != original || body["aspect_ratio"] != "16:9" || body["resolution"] != nil {
				t.Error(body)
			}
			fmt.Fprint(w, `{"code":200,"data":[{"task_id":"task_ir"}]}`)
		case "GET /v1/tasks/task_ir":
			fmt.Fprintf(w, `{"code":200,"data":{"id":"task_ir","status":"completed","usage":{"input_tokens":500,"output_tokens":300},"result":{"prompt":%q}}}`, enhanced)
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p.base = server.URL
	request := generation(t, "MiniMax-H3-Context-IR", "video.generate", original, nil, nil)
	result, err := p.Execute(t.Context(), request, nil)
	if err != nil || calls != 1 || len(result.Media) != 1 || result.Media[0].Kind != "text" || string(result.Media[0].Data) != enhanced {
		t.Fatalf("%+v %v", result, err)
	}
	if got := string(result.Metadata); !strings.Contains(got, `"completion_tokens":300`) || !strings.Contains(got, `"prompt_tokens":500`) {
		t.Fatal(got)
	}
	if err := p.Validate(t.Context(), generation(t, "MiniMax-H3-Context-IR", "video.generate", original, map[string]any{"aspect_ratio": "adaptive"}, nil)); err == nil {
		t.Fatal("accepted adaptive text generation")
	}
}

func TestSeedreamRichTextFollowsItsLayerURL(t *testing.T) {
	raw := json.RawMessage(`{"images":[{"url":["https://example.com/base.png","https://example.com/title.png"],"layers":[{"z_index":0},{"z_index":3,"text_recognition":{"text":"sale","lines":[{"runs":[{"text":"sale","style":{"bold":true}}]}]}}]}]}`)
	spec := modelSpec{Upstream: "seedream-5-5", Model: aigc.Model{Capability: "image.generate"}}
	media, err := decodeTaskMedia(spec, map[string]any{"layer_decomposition": true}, raw)
	if err != nil || len(media) != 2 || strings.Contains(string(media[0].ImageLayer), "sale") || !strings.Contains(string(media[1].ImageLayer), `"bold":true`) {
		t.Fatalf("%+v %v", media, err)
	}

	malformed := json.RawMessage(`{"images":[{"url":["https://example.com/base.png","https://example.com/title.png"],"layers":[{"z_index":3}]}]}`)
	media, err = decodeTaskMedia(spec, map[string]any{"layer_decomposition": true}, malformed)
	if err != nil || len(media) != 2 || len(media[0].ImageLayer) != 0 || len(media[1].ImageLayer) != 0 {
		t.Fatalf("%+v %v", media, err)
	}
}

func TestMusicContinuationUsesIndexInProviderResult(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	task := remoteTask{ID: "task_original", Created: time.Now().Unix(), Result: json.RawMessage(`{"music":[{},{"audio_url":"https://example.com/second.mp3"}]}`)}
	media, err := decodeTaskMedia(p.models["suno"], nil, task.Result)
	if err != nil {
		t.Fatal(err)
	}
	origins := taskOrigins(p.models["suno"], task, media, 4)
	if len(origins) != 1 || origins[0].OutputIndex != 4 || origins[0].ItemIndex != 1 {
		t.Fatal(origins)
	}
	request := generation(t, "suno-follow-up", "music.generate", "", map[string]any{"operation": "stem"}, nil)
	request.SourceTask = &origins[0].SourceTask
	spec, body, _, err := p.prepare(request)
	want := map[string]any{"model": "suno", "task_id": "task_original", "audio_index": 2, "stem_type": "lead_vocal", "audio_format": "mp3"}
	if err != nil || spec.Endpoint != "/v1/music/generations/stems" || !reflect.DeepEqual(body, want) {
		t.Fatalf("%s %+v %v", spec.Endpoint, body, err)
	}
	const clip = "e5c1d19e-c022-427b-92b5-d529510249e5"
	task.Result = json.RawMessage(fmt.Sprintf(`{"music":[{"clip_id":%q,"audio_url":"https://example.com/song.m4a","wav_url":"https://example.com/song.wav"}]}`, clip))
	media, err = decodeTaskMedia(p.models["flowmusic"], nil, task.Result)
	if err != nil {
		t.Fatal(err)
	}
	origins = taskOrigins(p.models["flowmusic"], task, media, 0)
	if len(origins) != 1 || origins[0].Ticket != clip || media[0].URL != "https://example.com/song.wav" {
		t.Fatal(origins, media)
	}
	request = generation(t, "flowmusic-follow-up", "music.generate", "", map[string]any{"operation": "replace", "instruction": "piano", "start_s": 2, "end_s": 3, "replacement_seed": 12}, nil)
	request.SourceTask = &origins[0].SourceTask
	spec, body, _, err = p.prepare(request)
	want = map[string]any{"model": "flowmusic", "clip_id": clip, "start_s": float64(2), "end_s": float64(3), "instruction": "piano", "seed": float64(12)}
	if err != nil || spec.Endpoint != "/v1/music/generations/replaceFlowMusic" || !reflect.DeepEqual(body, want) {
		t.Fatalf("%s %+v %v", spec.Endpoint, body, err)
	}
}

func TestFlowMusicStemsUsesBoundClipAndArchive(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	request := generation(t, "flowmusic-follow-up", "music.generate", "", map[string]any{"operation": "stems"}, nil)
	if err := p.Validate(t.Context(), request); err != nil {
		t.Fatal("preflight should not require the private source ticket", err)
	}
	if _, err := p.Execute(t.Context(), request, nil); err == nil {
		t.Fatal("executed source-bound operation without ticket")
	}
	request.SourceTask = &aigc.SourceTask{Ticket: "e5c1d19e-c022-427b-92b5-d529510249e5"}
	spec, body, _, err := p.prepare(request)
	if err != nil || spec.Endpoint != "/v1/music/generations/stemsFlowMusic" || len(body) != 2 || body["clip_id"] != request.SourceTask.Ticket {
		t.Fatalf("%+v %v", body, err)
	}
	raw := json.RawMessage(`{"music":[{"file_url":"https://example.com/stems.zip","mime_type":"application/zip"}]}`)
	outputs, err := decodeTaskMedia(spec, body, raw)
	if err != nil || len(outputs) != 1 || outputs[0].Kind != "audio-archive" || outputs[0].URL != "https://example.com/stems.zip" {
		t.Fatalf("%+v %v", outputs, err)
	}
}

func TestMidjourneyGridAndFollowUp(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"grid_image_url":"https://example.com/grid.png","image_urls":["https://example.com/one.png","https://example.com/two.png"]}`)
	out, err := decodeTaskMedia(p.models["midjourney"], nil, raw)
	if err != nil || len(out) != 1 || out[0].URL != "https://example.com/grid.png" {
		t.Fatalf("%+v %v", out, err)
	}
	for _, operation := range []string{"upscale", "variation", "reroll"} {
		request := generation(t, "midjourney-follow-up", "image.generate", "", map[string]any{"operation": operation, "index": 3}, nil)
		request.SourceTask = &aigc.SourceTask{Ticket: "task_grid"}
		spec, body, _, err := p.prepare(request)
		if err != nil || spec.Endpoint != "/v1/midjourney/generations/"+operation || body["task_id"] != "task_grid" || body["model"] != nil || (body["index"] == nil) != (operation == "reroll") {
			t.Fatalf("%+v %v", body, err)
		}
	}
}

func TestGrokRegionEditUsesBoundSegmentationAndOnePaidSubmit(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	var segment, edit int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/images/generations":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			switch body["operation"] {
			case "segment":
				segment++
				if body["source_task_id"] != "task_image" || body["prompt"] != nil || body["include_mask_rle"] != false {
					t.Error(body)
				}
				fmt.Fprint(w, `{"code":200,"data":[{"task_id":"task_segment"}]}`)
			case "region_edit":
				edit++
				if body["image_id"] != "image_fixture" || body["prompt"] != "Paint it red" || body["source_task_id"] != nil || body["image_urls"] != nil || !reflect.DeepEqual(body["boxes"], []any{[]any{0.1, 0.2, 0.7, 0.8}}) {
					t.Error(body)
				}
				fmt.Fprint(w, `{"code":200,"data":[{"task_id":"task_edit"}]}`)
			default:
				t.Error(body)
				w.WriteHeader(400)
			}
		case "GET /v1/tasks/task_segment":
			fmt.Fprint(w, `{"code":200,"data":{"id":"task_segment","status":"completed","result":{"image_id":"image_fixture","cache_status":"hit"}}}`)
		case "GET /v1/tasks/task_edit":
			fmt.Fprint(w, `{"code":200,"data":{"id":"task_edit","status":"completed","result":{"images":[{"url":["https://example.com/edited.png"]}]}}}`)
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p.base = server.URL
	regions := []any{map[string]any{"x1": 0.1, "y1": 0.2, "x2": 0.7, "y2": 0.8}}
	request := generation(t, "grok-imagine-2.0-ext-region-edit", "image.generate", "Paint it red", map[string]any{"regions": regions}, nil)
	request.SourceTask = &aigc.SourceTask{Ticket: "task_image"}
	out, err := p.Execute(t.Context(), request, nil)
	if err != nil || len(out.Media) != 1 || out.Media[0].URL != "https://example.com/edited.png" || segment != 1 || edit != 1 {
		t.Fatalf("%+v %v, %d/%d", out, err, segment, edit)
	}
}

func TestGrokPolygonWithHoleAndSourceObjectIndices(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	point := func(x, y float64) map[string]any { return map[string]any{"x": x, "y": y} }
	outer := []any{point(.1, .1), point(.8, .1), point(.8, .8), point(.1, .8)}
	hole := []any{point(.3, .3), point(.4, .3), point(.35, .4)}
	regions := []any{map[string]any{"outer": outer, "holes": []any{map[string]any{"points": hole}}}}
	request := generation(t, "grok-imagine-2.0-ext-region-edit", "image.generate", "Change color", map[string]any{"selection_mode": "polygon", "polygons": regions}, nil)
	request.SourceTask = &aigc.SourceTask{Ticket: "task_original_single"}
	_, body, _, err := p.prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(body["selection_regions"])
	if body["selection_mode"] != nil || body["boxes"] != nil || !strings.Contains(string(payload), `"holes":[{"points":[0.3,0.3,0.4,0.3,0.35,0.4]}]`) {
		t.Fatal(body)
	}
	request = generation(t, "grok-imagine-2.0-ext-region-edit", "image.generate", "Change color", map[string]any{"selection_mode": "objects", "object_indices": []int{42}}, nil)
	request.SourceTask = &aigc.SourceTask{Ticket: "task_original_single"}
	var edits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["operation"] == "region_edit" {
				edits++
			}
			fmt.Fprint(w, `{"code":200,"data":[{"task_id":"task_segment"}]}`)
			return
		}
		fmt.Fprint(w, `{"code":200,"data":{"id":"task_segment","status":"completed","result":{"image_id":"img_original","objects":[{"index":7}]}}}`)
	}))
	defer server.Close()
	p.base = server.URL
	if _, err = p.Execute(t.Context(), request, nil); err == nil || edits != 0 {
		t.Fatal("unexpected edit of source with missing object", err, edits)
	}
}
