package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func fixture(t *testing.T, p *Plugin, model string, params map[string]any, count int) aigc.Request {
	t.Helper()
	var selected aigc.Model
	for _, m := range p.Manifest().Models {
		if m.ID == model {
			selected = m
		}
	}
	b, _ := json.Marshal(aigc.Generation{Provider: "dashscope", Capability: selected.Capability, Model: model, Prompt: "处理{{Image 1}}", Parameters: params})
	r := aigc.Request{Payload: b, Secrets: map[string]string{"API_KEY": "fixture-only-secret", "WORKSPACE_ID": "fixture-workspace"}}
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 300, 300))); err != nil {
		t.Fatal(err)
	}
	for range count {
		r.Inputs = append(r.Inputs, aigc.Input{Kind: "image", Data: img.Bytes()})
	}
	return r
}
func local(p *Plugin, srv *httptest.Server) {
	p.baseURL = srv.URL
	p.Client = srv.Client()
	p.pollInterval = time.Millisecond
}
func TestUnavailableModelDoesNotRetryOrExposeProviderBody(t *testing.T) {
	p := New()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"Model not exist. fixture-only-secret"}`))
	}))
	defer srv.Close()
	local(p, srv)
	_, err := p.call(context.Background(), "POST", srv.URL, multimodalPath, "fixture-only-secret", map[string]any{}, false)
	if err == nil || !strings.Contains(err.Error(), "目标模型或接口不可用") || strings.Contains(err.Error(), "fixture-only-secret") || calls != 1 {
		t.Fatalf("unsafe or retried failure: %v, calls=%d", err, calls)
	}
}
func TestCatalogAndSyncMappings(t *testing.T) {
	t.Setenv("DASHSCOPE_REGION", "cn-beijing")
	p := New()
	if err := p.Manifest().Validate(); err != nil {
		t.Fatal(err)
	}
	if len(p.Manifest().Models) != 17 || len(p.Manifest().Nodes) != 4 || p.Manifest().Scheduling.Cancelable {
		t.Fatal("invalid catalog")
	}
	for _, m := range p.Manifest().Models {
		if m.Capability == "video.generate" || m.Capability == "voice.design" || m.Capability == "speech.generate" && m.ID != "qwen3-tts-flash" {
			continue
		}
		t.Run(m.ID, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != multimodalPath || r.Header.Get("X-DashScope-Async") != "" || r.Header.Get("Authorization") != "Bearer fixture-only-secret" {
					t.Error("wrong protocol")
				}
				var body struct {
					Model      string         `json:"model"`
					Input      map[string]any `json:"input"`
					Parameters map[string]any `json:"parameters"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != m.ID {
					t.Error("wrong model")
				}
				if m.Capability == "speech.generate" {
					if body.Input["text"] != "处理图1" || body.Input["voice"] != "Cherry" || len(body.Parameters) != 0 {
						t.Error("wrong TTS input", body)
					}
					_, _ = w.Write([]byte(`{"request_id":"fixture-id","output":{"audio":{"url":"http://fixture.oss-cn-beijing.aliyuncs.com/result.wav"}}}`))
				} else {
					content := body.Input["messages"].([]any)[0].(map[string]any)["content"].([]any)
					if len(content) != 2 || !strings.HasPrefix(content[0].(map[string]any)["image"].(string), "data:image/png;base64,") || content[1].(map[string]any)["text"] != "处理图1" {
						t.Error("reference mapping")
					}
					if strings.HasPrefix(m.ID, "qwen") && body.Parameters["size"] != nil {
						t.Error("auto size must be omitted")
					}
					if strings.HasPrefix(m.ID, "wan") && body.Parameters["thinking_mode"] != nil {
						t.Error("inactive thinking sent")
					}
					_, _ = w.Write([]byte(`{"request_id":"fixture-id","usage":{"output_width":1370,"output_height":1148,"secret":"fixture-only-secret"},"output":{"choices":[{"message":{"content":[{"image":"https://example.com/result.png"},{"image":"https://example.com/result2.png"}]}}]}}`))
				}
			}))
			defer srv.Close()
			local(p, srv)
			count := 1
			if m.Capability == "speech.generate" {
				count = 0
			}
			r := fixture(t, p, m.ID, nil, count)
			if err := p.Validate(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("validation performed network IO")
			}
			out, err := p.Execute(context.Background(), r, nil)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(out.Media) < 1 || strings.Contains(string(out.Metadata), "fixture-only-secret") {
				t.Fatal("unsafe result")
			}
			if m.Capability == "speech.generate" && !strings.HasPrefix(out.Media[0].URL, "https:") {
				t.Fatal("OSS must use TLS")
			}
		})
	}
}
func TestVideoSingleSubmitCheckpointAndPolling(t *testing.T) {
	for _, model := range []string{"wan3.0-video", "wan3.0-video-prime"} {
		for _, mode := range []string{"reference", "first_frame", "first_last_frame"} {
			t.Run(model+"/"+mode, func(t *testing.T) {
				p := New()
				posts, gets, checkpoints := 0, 0, 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "POST" {
						posts++
						var b struct {
							Input struct {
								Media []struct {
									Type string `json:"type"`
								} `json:"media"`
							} `json:"input"`
							Parameters map[string]any `json:"parameters"`
						}
						_ = json.NewDecoder(r.Body).Decode(&b)
						if r.URL.Path != videoPath || r.Header.Get("X-DashScope-Async") != "enable" || b.Parameters["input_mode"] != nil {
							t.Error("invalid async request")
						}
						if mode == "reference" && len(b.Input.Media) != 0 {
							t.Error("implicit self reference")
						}
						if mode != "reference" && b.Input.Media[0].Type != "first_frame" {
							t.Error("first frame missing")
						}
						if mode == "first_last_frame" && b.Input.Media[1].Type != "last_frame" {
							t.Error("last frame missing")
						}
						_, _ = w.Write([]byte(`{"output":{"task_id":"ticket-1","task_status":"PENDING"}}`))
					} else {
						gets++
						if r.URL.Path != "/tasks/ticket-1" || checkpoints != 1 {
							t.Error("ticket was not retained before polling")
						}
						if gets == 1 {
							_, _ = w.Write([]byte(`{"output":{"task_status":"RUNNING"}}`))
						} else {
							_, _ = w.Write([]byte(`{"output":{"task_status":"SUCCEEDED","video_url":"https://example.com/result.mp4"}}`))
						}
					}
				}))
				defer srv.Close()
				local(p, srv)
				count := 0
				if mode == "first_frame" {
					count = 1
				}
				if mode == "first_last_frame" {
					count = 2
				}
				out, err := p.Execute(context.Background(), fixture(t, p, model, map[string]any{"input_mode": mode}, count), func(r aigc.Result) {
					checkpoints++
					if !strings.Contains(string(r.Metadata), "ticket-1") {
						t.Error("missing ticket")
					}
				})
				if err != nil || posts != 1 || gets != 2 || len(out.Media) != 1 {
					t.Fatal(err, posts, gets)
				}
			})
		}
	}
}
func TestCrossFieldValidationBeforeSubmission(t *testing.T) {
	p := New()
	for _, tc := range []struct {
		model  string
		params map[string]any
		count  int
	}{
		{"qwen-image-3.0", map[string]any{"prompt_extend_mode": "agent"}, 1},
		{"qwen-image-3.0", nil, 4}, {"qwen-image-2.1-pro", map[string]any{"negative_prompt": "unsupported"}, 0},
		{"wan2.7-image-pro", map[string]any{"size": "4K"}, 1},
		{"wan2.7-image-pro", map[string]any{"size": "4K", "enable_sequential": true}, 0},
		{"wan2.7-image", map[string]any{"n": 5}, 0}, {"wan2.7-image", nil, 10},
		{"wan3.0-video", map[string]any{"input_mode": "first_frame"}, 0},
		{"wan3.0-video", map[string]any{"input_mode": "first_last_frame"}, 1},
		{"wan3.0-video", map[string]any{"duration": 1}, 0},
	} {
		r := fixture(t, p, tc.model, tc.params, tc.count)
		if p.Validate(context.Background(), r) == nil {
			t.Fatalf("accepted invalid case %+v", tc)
		}
	}
	if err := p.Validate(context.Background(), fixture(t, p, "wan2.7-image", map[string]any{"n": 12, "enable_sequential": true}, 0)); err != nil {
		t.Fatal(err)
	}
}
func TestFailuresAndCancellationNeverResubmit(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			p := New()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "https://example.com")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"code":"failure","message":"fixture-only-secret"}`))
			}))
			defer srv.Close()
			p.baseURL = srv.URL // Keep production redirect policy.
			_, err := p.Execute(context.Background(), fixture(t, p, "qwen-image-3.0", nil, 0), nil)
			if err == nil || calls != 1 || strings.Contains(err.Error(), "fixture-only-secret") {
				t.Fatal("failure leaked or retried", err, calls)
			}
		})
	}
	for _, state := range []string{"FAILED", "CANCELED", "UNKNOWN", "malformed", "cancel-context"} {
		t.Run(state, func(t *testing.T) {
			p := New()
			posts, gets := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					_, _ = w.Write([]byte(`{"output":{"task_id":"ticket-1","task_status":"PENDING"}}`))
				} else {
					gets++
					_ = json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"task_status": state, "message": "fixture-only-secret"}})
				}
			}))
			defer srv.Close()
			local(p, srv)
			_, err := p.Execute(ctx, fixture(t, p, "wan3.0-video", nil, 0), func(aigc.Result) {
				if state == "cancel-context" {
					cancel()
				}
			})
			if err == nil || posts != 1 || gets > 1 || strings.Contains(err.Error(), "fixture-only-secret") {
				t.Fatal(err, posts, gets)
			}
		})
	}
}
func TestEndpointAndInputIsolation(t *testing.T) {
	p := New()
	p.region = "ap-southeast-1"
	r := fixture(t, p, "qwen-image-3.0", nil, 1)
	base, err := p.endpoint(r.Secrets)
	if err != nil || base != "https://fixture-workspace.ap-southeast-1.maas.aliyuncs.com/api/v1" {
		t.Fatal(base, err)
	}
	r.Secrets["WORKSPACE_ID"] = "evil.example/path"
	if _, err = p.endpoint(r.Secrets); err == nil {
		t.Fatal("workspace host injection")
	}
	if _, err = inputImage(aigc.Input{Kind: "image", URL: "https://example.com/private"}, "qwen-image-3.0"); err == nil {
		t.Fatal("unresolved URL accepted")
	}
	if _, err = inputImage(aigc.Input{Kind: "image", Data: []byte("not a real image")}, "qwen-image-3.0"); err == nil {
		t.Fatal("untrusted MIME")
	}
	if _, err = mediaURL("http://127.0.0.1/private"); err == nil {
		t.Fatal("insecure output")
	}
	if _, err = mediaURL("https://secret@example.com/private"); err == nil {
		t.Fatal("userinfo accepted")
	}
}
