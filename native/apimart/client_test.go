// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func generation(t *testing.T, model, capability, prompt string, parameters map[string]any, inputs []aigc.Input) aigc.Request {
	t.Helper()
	payload, err := json.Marshal(aigc.Generation{Provider: provider, Capability: capability, Model: model, Prompt: prompt, Parameters: parameters})
	if err != nil {
		t.Fatal(err)
	}
	return aigc.Request{Payload: payload, Inputs: inputs, Secrets: map[string]string{secret: "fixture-only"}}
}

func TestTaskContinuationUsesVerifiedUpstreamTicket(t *testing.T) {
	for _, fixture := range []struct {
		source, target, originalBody, followupBody string
		parameters                                 map[string]any
	}{
		{
			source: "MiniMax-H3", target: "MiniMax-H3-Regeneration",
			originalBody: `"resolution":"768P"`, parameters: map[string]any{"resolution": "768P", "duration": 5},
			followupBody: `{"model":"MiniMax-H3-Regeneration","source_task_id":"task_original"}`,
		},
		{
			source: "seedance-2.5-draft", target: "seedance-2.5-final-1080p",
			originalBody: `"draft":true`, parameters: map[string]any{"duration": 5},
			followupBody: `{"model":"seedance-2.5","draft_task_id":"task_original","resolution":"1080p","watermark":false}`,
		},
	} {
		t.Run(fixture.source, func(t *testing.T) {
			p, err := loadPlugin()
			if err != nil {
				t.Fatal(err)
			}
			created := time.Now().Add(-time.Hour).Unix()
			var posts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST /v1/videos/generations":
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					id := "task_original"
					if posts.Add(1) == 1 {
						if !strings.Contains(string(body), fixture.originalBody) {
							t.Errorf("original video settings: %s", body)
						}
					} else {
						id = "task_continued"
						var got, want map[string]any
						if json.Unmarshal(body, &got) != nil || json.Unmarshal([]byte(fixture.followupBody), &want) != nil || !reflect.DeepEqual(got, want) {
							t.Errorf("followup submitted %s, expected %s", body, fixture.followupBody)
						}
					}
					fmt.Fprintf(w, `{"code":200,"data":[{"task_id":%q,"status":"submitted"}]}`, id)
				case "GET /v1/tasks/task_original", "GET /v1/tasks/task_continued":
					id := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
					fmt.Fprintf(w, `{"code":200,"data":{"id":%q,"status":"completed","created":%d,"cost":0.3104,"usage":{"completion_tokens":38800},"result":{"videos":[{"url":["https://example.com/video.mp4"]}]}}}`, id, created)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer upstream.Close()
			p.base = upstream.URL
			source := generation(t, fixture.source, "video.generate", "Original", fixture.parameters, nil)
			original, err := p.Execute(t.Context(), source, nil)
			if err != nil || len(original.TaskOrigins) != 1 || original.TaskOrigins[0].Ticket != "task_original" ||
				original.TaskOrigins[0].CreatedAt != created*1000 || original.TaskOrigins[0].OutputIndex != 0 {
				t.Fatalf("original task evidence: %v %v", original, err)
			}
			followup := generation(t, fixture.target, "video.generate", "", nil, nil)
			if _, err = p.Execute(t.Context(), followup, nil); err == nil || posts.Load() != 1 {
				t.Fatal("unbound followup reached the provider")
			}
			followup.SourceTask = &original.TaskOrigins[0].SourceTask
			continued, err := p.Execute(t.Context(), followup, nil)
			if err != nil || posts.Load() != 2 || len(continued.Media) != 1 || len(continued.TaskOrigins) != 0 {
				t.Fatalf("followup result: %v %v", continued, err)
			}
			if !strings.Contains(string(continued.Metadata), `"completion_tokens":38800`) ||
				!strings.Contains(string(continued.Metadata), `"usd":0.3104`) {
				t.Fatal("missing settlement figures", string(continued.Metadata))
			}
		})
	}
}

func TestMagiReferenceSubmission(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Bool
	var submitted atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-only" {
			t.Error("fixture authorization missing")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/videos/generations":
			submitted.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			urls, _ := body["image_urls"].([]any)
			videos, _ := body["video_urls"].([]any)
			audios, _ := body["audio_urls"].([]any)
			if body["model"] != "magi-2" || body["duration"] != float64(4) ||
				body["resolution"] != "1080p" || body["omni_reference_task_type"] != "reference" ||
				len(urls) != 1 || urls[0] != "https://example.com/boat.png" ||
				len(videos) != 1 || videos[0] != "https://example.com/motion.mp4" ||
				len(audios) != 1 || audios[0] != "https://example.com/ambience.wav" {
				t.Errorf("invalid MAGI-2 reference projection: %v", body)
			}
			io.WriteString(w, `{"code":200,"data":[{"task_id":"task_magi2_fixture","status":"submitted"}]}`)
		case "GET /v1/tasks/task_magi2_fixture":
			if !accepted.Load() {
				t.Error("original task was queried before the checkpoint")
			}
			io.WriteString(w, `{"code":200,"data":{"id":"task_magi2_fixture","status":"completed","cost":0.32,"result":{"videos":[{"url":["https://example.com/boat.mp4"]}]}}}`)
		default:
			t.Errorf("unexpected fixture call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer upstream.Close()
	p.base = upstream.URL
	r := generation(t, "magi-2-reference", "video.generate", "A paper boat", map[string]any{"duration": 4},
		[]aigc.Input{
			{Kind: "image", URL: "https://example.com/boat.png"},
			{Kind: "video", URL: "https://example.com/motion.mp4"},
			{Kind: "audio", URL: "https://example.com/ambience.wav"},
		})
	result, err := p.Execute(t.Context(), r, func(checkpoint aigc.Result) {
		if !strings.Contains(string(checkpoint.Metadata), "task_magi2_fixture") {
			t.Error("checkpoint missing task ID")
		}
		accepted.Store(true)
	})
	if err != nil || submitted.Load() != 1 || len(result.Media) != 1 || result.Media[0].URL != "https://example.com/boat.mp4" {
		t.Fatal("original MAGI-2 task did not yield its video", err, result)
	}
}

func TestSunoRejectedSubmissionIsNeverReissued(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/music/generations" {
			t.Errorf("unexpected follow-up %s %s", r.Method, r.URL.Path)
		}
		posts.Add(1)
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["custom"] != false || body["version"] != "v6" || body["style"] != nil {
			t.Errorf("Suno inspiration sent inactive or invalid parameters: %v", body)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	p.base = upstream.URL
	r := generation(t, "suno", "music.generate", "Strings", map[string]any{"style": "old custom draft"}, nil)
	_, err = p.Execute(context.Background(), r, func(aigc.Result) { t.Error("rejection produced a checkpoint") })
	if err == nil || posts.Load() != 1 {
		t.Fatal("failed submission must end after one POST", posts.Load(), err)
	}
}
