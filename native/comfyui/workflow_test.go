package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const fixtureGraph = `{"load":{"class_type":"LoadImage","inputs":{"image":"original.png"}},"save":{"class_type":"SaveImage","inputs":{"images":["load",0],"filename_prefix":"test"}}}`

func fixtureInfo() ObjectInfo {
	return ObjectInfo{
		"LoadImage": {Input: map[string]map[string][]any{"required": {"image": {[]any{"original.png"}, map[string]any{"image_upload": true}}}}, Output: []string{"IMAGE"}},
		"SaveImage": {Input: map[string]map[string][]any{"required": {"images": {"IMAGE"}, "filename_prefix": {"STRING"}}}, OutputNode: true},
	}
}
func TestCompilePreservesLinksAndCreatesTypedRoles(t *testing.T) {
	d, e := Compile("fixture", "http://localhost:8000", json.RawMessage(fixtureGraph), fixtureInfo())
	if e != nil {
		t.Fatal(e)
	}
	m := d.Manifest.Models[0]
	if len(m.InputRoles) != 1 || m.InputRoles["ref-1"].Kind != "image" || len(m.Fields) != 1 || m.Capability != "image.generate" || strings.Join(m.OutputKinds, ",") != "image" {
		t.Fatal("incorrect schema", m)
	}
	if _, _, ok := link(d.Graph["save"].Inputs["images"], d.Graph); !ok {
		t.Fatal("graph edge lost")
	}
	for _, raw := range []string{`{"nodes":[],"links":[]}`, strings.Replace(fixtureGraph, "original.png", `x","api_key":"private`, 1), strings.Replace(fixtureGraph, `["load",0]`, `["missing",0]`, 1)} {
		if _, e := Parse(json.RawMessage(raw)); e == nil {
			t.Fatal("invalid graph accepted")
		}
	}
}
func TestExecuteUploadsAndSubmitsOnceWithDurableTicket(t *testing.T) {
	var submits, uploads atomic.Int32
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 7, 11)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
			w.WriteHeader(404)
		case "/queue":
			_, _ = w.Write([]byte(`{"queue_running":[[1,"receipt"]],"queue_pending":[]}`))
		case "/upload/image":
			uploads.Add(1)
			if e := r.ParseMultipartForm(1 << 20); e != nil {
				t.Error(e)
			}
			f, _, e := r.FormFile("image")
			if e != nil {
				t.Error(e)
			} else {
				f.Close()
			}
			_, _ = w.Write([]byte(`{"name":"uploaded.png","subfolder":"","type":"input"}`))
		case "/prompt":
			submits.Add(1)
			var body struct {
				Prompt Graph `json:"prompt"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Prompt["load"].Inputs["image"] != "uploaded.png" || body.Prompt["save"].Inputs["filename_prefix"] != "changed" {
				t.Error("parameters or input not injected")
			}
			_, _ = w.Write([]byte(`{"prompt_id":"receipt","node_errors":{}}`))
		case "/history/receipt":
			_, _ = w.Write([]byte(`{"receipt":{"status":{"completed":true,"status_str":"success"},"outputs":{"save":{"images":[{"filename":"result.png","subfolder":"nested\\windows","type":"output"},{"filename":"result.png","subfolder":"nested\\windows","type":"output"}]}}}}`))
		case "/view":
			if r.URL.Query().Get("subfolder") != `nested\windows` {
				t.Error("native relative subfolder changed")
			}
			_, _ = w.Write(picture.Bytes())
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d, e := Compile("fixture", server.URL, json.RawMessage(fixtureGraph), fixtureInfo())
	if e != nil {
		t.Fatal(e)
	}
	p := New(d)
	p.PollInterval = time.Millisecond
	payload, _ := json.Marshal(aigc.Generation{Provider: d.Manifest.ID, Capability: "image.generate", Model: "workflow", Parameters: map[string]any{"p001": "changed"}})
	checkpoint := false
	out, e := p.Execute(context.Background(), aigc.Request{JobID: "job", Payload: payload, Inputs: []aigc.Input{{Kind: "image", Role: "ref-1", Data: picture.Bytes()}}}, func(r aigc.Result) { checkpoint = bytes.Contains(r.Metadata, []byte(`"promptId":"receipt"`)) })
	if e != nil {
		t.Fatal(e)
	}
	if submits.Load() != 1 || uploads.Load() != 1 || !checkpoint || len(out.Media) != 1 || out.Media[0].Kind != "image" {
		t.Fatal("submission/receipt/output invariant failed")
	}
	if d.Graph["load"].Inputs["image"] != "original.png" {
		t.Fatal("immutable definition mutated")
	}
	if out, e = p.Collect(context.Background(), "receipt"); e != nil || len(out.Media) != 1 || submits.Load() != 1 || uploads.Load() != 1 {
		t.Fatal("ticket collection resubmitted work or failed", e)
	}
}
func TestNoRetryOnLostSubmissionAndNoRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			calls.Add(1)
		}
		w.WriteHeader(503)
	}))
	defer server.Close()
	d, e := Compile("fixture", server.URL, json.RawMessage(fixtureGraph), fixtureInfo())
	if e != nil {
		t.Fatal(e)
	}
	p := New(d)
	b, _ := json.Marshal(aigc.Generation{Provider: d.Manifest.ID, Capability: "image.generate", Model: "workflow"})
	if _, e = p.Execute(context.Background(), aigc.Request{Payload: b}, nil); e == nil || calls.Load() != 1 {
		t.Fatal("submission retried or failure ignored")
	}
	for _, name := range []string{"../secret", "C:\\config", "/absolute"} {
		if safeFile(name, "") {
			t.Fatal("unsafe path accepted")
		}
	}
	for _, folder := range []string{`..\secret`, `nested\..\secret`, `/absolute`, `\absolute`, `\\server\share`, `C:\secret`, `nested/../secret`, `nested\../secret`} {
		if safeFile("image.png", folder) {
			t.Fatal("unsafe subfolder accepted", folder)
		}
	}
	if !safeFile("image.png", `nested\windows`) || !safeFile("image.png", "nested/linux") {
		t.Fatal("safe native subfolder rejected")
	}
	for _, base := range []string{"file:///tmp", "http://user:password@host", "http://host/?token=x"} {
		if _, e = Endpoint(base); e == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}
func TestProvidedWorkflowsCompile(t *testing.T) {
	dir, infoPath := os.Getenv("COMFYUI_SAMPLE_DIR"), os.Getenv("COMFYUI_OBJECT_INFO")
	if dir == "" || infoPath == "" {
		t.Skip("set COMFYUI_SAMPLE_DIR and COMFYUI_OBJECT_INFO for real workflow schema acceptance")
	}
	b, e := os.ReadFile(infoPath)
	if e != nil {
		t.Fatal(e)
	}
	var info ObjectInfo
	if e = json.Unmarshal(b, &info); e != nil {
		t.Fatal(e)
	}
	files, e := filepath.Glob(filepath.Join(dir, "*.json"))
	if e != nil || len(files) == 0 {
		t.Fatal("no sample JSON")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			b, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			d, e := Compile(strings.TrimSuffix(filepath.Base(file), ".json"), "http://100.64.80.98:8000", b, info)
			if e != nil {
				t.Fatal(e)
			}
			m := d.Manifest.Models[0]
			t.Logf("nodes=%d parameters=%d inputs=%d output=%v warnings=%d", len(d.Graph), len(m.Fields), len(m.InputRoles), m.OutputKinds, len(d.Warnings))
		})
	}
}
