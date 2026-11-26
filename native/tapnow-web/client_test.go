package tapnowweb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnowtest"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func defaults(v Mode) map[string]any {
	p := map[string]any{}
	for k, f := range v.Fields {
		if f.Default != nil {
			p[k] = f.Default
		} else if x := v.Defaults[k]; x != nil {
			p[k] = x
		}
	}
	return p
}

func TestPollMultipleTasksKeepsEveryOutputInTicketOrder(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	polls := 0
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/conversation/v1/generations/tasks" {
			return transport.RoundTrip(r)
		}
		polls++
		status := "completed"
		if polls == 1 {
			status = "running"
		}
		body, _ := json.Marshal(map[string]any{"code": 0, "data": []any{
			map[string]any{"id": "task-b", "status": status, "outputs": map[string]any{"image_urls": []string{c.origin + "/second"}}},
			map[string]any{"id": "task-a", "status": "completed", "generated_images": []any{map[string]any{"image_url": c.origin + "/first"}, map[string]any{"image_url": c.origin + "/first-alt"}}, "outputs": map[string]any{"image_urls": []string{c.origin + "/first"}}},
		}})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})
	media, err := c.Poll(context.Background(), Ticket{TaskIDs: []string{"task-a", "task-b"}}, "image")
	if err != nil || polls != 2 || paid.Load() != 0 || len(media) != 3 || media[0].NodeID != "task-a" || media[0].URL != c.origin+"/first" || media[1].NodeID != "task-a" || media[1].URL != c.origin+"/first-alt" || media[2].NodeID != "task-b" || media[2].URL != c.origin+"/second" {
		t.Fatal("multiple task outputs lost or reordered", media, polls, err)
	}
}
func TestEveryVisibleCatalogModePrepares(t *testing.T) {
	c := Baseline()
	if len(c.Models) == 0 {
		t.Fatal("empty catalog")
	}
	ids := map[string]bool{}
	hidden := 0
	for _, m := range c.Models {
		if m.Hidden {
			hidden++
		}
	}
	if hidden == 0 {
		t.Fatal("published hidden entries not respected")
	}
	for _, m := range c.Models {
		if ids[m.ID] {
			t.Fatal("duplicate ID", m.ID)
		}
		ids[m.ID] = true
		for _, v := range m.Modes {
			t.Run(m.ID+"/"+v.ID, func(t *testing.T) {
				refs := []tapnow.Reference{}
				for kind, n := range map[string]int{"image": int(v.Images.Min), "video": int(v.Videos.Min), "audio": int(v.Audios.Min)} {
					for i := 0; i < n; i++ {
						refs = append(refs, tapnow.Reference{Kind: kind, Role: kind, URL: "/files/asset" + kind, Order: len(refs)})
					}
				}
				if v.ID == "reference_to_video" && len(refs) == 0 {
					refs = append(refs, tapnow.Reference{Kind: "image", Role: "image", URL: "/files/assetimage", Order: 0})
				}
				r := Request{Model: m.ID, Mode: v.ID, Kind: m.Kind, Prompt: "Test prompt", Parameters: defaults(v), References: refs, VideoConsent: true, UploadConsent: true, MaxCost: 10}
				body, _, e := c.Prepare(r)
				if m.Hidden {
					if e == nil {
						t.Fatal("hidden mode executed")
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				if body["provider"] == nil || body["model"] == nil || body["model"] == "" || body["prompt"] == nil {
					t.Fatal("missing wire identity")
				}
			})
		}
	}
}
func TestRoleTransformsAndLimits(t *testing.T) {
	c := Baseline()
	m, v, e := c.Find("seedance-2.5", "video", "video_edit")
	if e != nil {
		t.Fatal(e)
	}
	r := Request{Model: m.ID, Mode: v.ID, Kind: m.Kind, Prompt: "Use {{Image 1}} with {{Video 1}}", Parameters: defaults(v), References: []tapnow.Reference{{Kind: "image", Role: "image", URL: "/files/image", Order: 1}, {Kind: "video", Role: "video", URL: "/files/video", Order: 0}}, VideoConsent: true, UploadConsent: true, MaxCost: 30}
	b, _, e := c.Prepare(r)
	if e != nil {
		t.Fatal(e)
	}
	if b["model"] != "agent-video-generation-02" || b["ratio"] != "adaptive" || b["duration"] != -1 || b["prompt"] != "Use Image1 with Video1" {
		t.Fatal(b)
	}
	if obj(arr(b["videos"])[0])["role"] != "reference_video" {
		t.Fatal(b)
	}
	r.References = append(r.References, tapnow.Reference{Kind: "video", Role: "video", URL: "/files/video2", Order: 2})
	if _, _, e = c.Prepare(r); e == nil {
		t.Fatal("extra edit source silently accepted")
	}
	r.References = r.References[:2]
	r.UploadConsent = false
	if _, _, e = c.Prepare(r); e == nil {
		t.Fatal("private upload lacks consent")
	}
	r.UploadConsent = true
	r.Parameters["aspectRatio"] = "16:9"
	if _, _, e = c.Prepare(r); e == nil {
		t.Fatal("forbidden field accepted")
	}
}
func TestGPTPixelAndBackground(t *testing.T) {
	c := Baseline()
	_, v, _ := c.Find("gpt-image-2.5-flare", "image", "text_to_image")
	r := Request{Model: "gpt-image-2.5-flare", Mode: "text_to_image", Kind: "image", Prompt: "test", Parameters: defaults(v), MaxCost: 1}
	r.Parameters["background"] = "transparent"
	b, _, e := c.Prepare(r)
	if e != nil || b["size"] != "1024x1024" || b["output_format"] != "png" {
		t.Fatal(b, e)
	}
	r.Parameters["aspectRatio"] = "auto"
	r.Parameters["imageSize"] = "auto"
	r.Parameters["targetWidth"] = float64(3840)
	r.Parameters["targetHeight"] = float64(2160)
	if _, _, e = c.Prepare(r); e != nil {
		t.Fatal(e)
	}
	r.Parameters["targetHeight"] = float64(3840)
	if _, _, e = c.Prepare(r); e == nil {
		t.Fatal("pixel cap bypass")
	}
}

func fixture(t *testing.T, quote string, malformed bool) (*Client, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var generations, creates atomic.Int32
	var submitted map[string]any
	var fixtureURL string
	lastKind := "image"
	remoteNode := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing bearer")
		}
		w.Header().Set("Content-Type", "application/json")
		data := any(map[string]any{})
		switch r.URL.Path {
		case "/api/public/v1/users/me":
			data = map[string]string{"user_id": "fixture-user", "org_id": "fixture-org"}
		case "/api/conversation/v1/file/upload-url":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			ext := map[string]string{"image/png": ".png", "video/mp4": ".mp4", "audio/wav": ".wav"}[str(body, "mime_type")]
			if ext == "" || number(body["file_size"]) <= 24 {
				t.Error("upload sniffed metadata", body)
			}
			if body["file_name"] != "canvas-reference"+ext {
				t.Error("upload filename must carry the sniffed extension", body["file_name"])
			}
			data = map[string]string{"file_id": "fixture-file-" + strings.TrimPrefix(ext, ".")}
		case "/api/billing/v2/estimate":
			data = map[string]string{"tapiesCost": quote}
		case "/api/canvas/v1/asset-reviews":
			if r.Method != http.MethodGet {
				t.Error("generation may only read existing asset reviews")
			}
		case "/api/billing/v2/wallet/balance":
			data = map[string]string{"total": "10000", "available": "10000", "onHold": "0"}
		case "/api/canvas/v1/canvases":
			creates.Add(1)
			data = map[string]string{"id": "fixture-canvas"}
		case "/api/canvas/v1/canvases/fixture-canvas/nodes:batchActions":
			var batch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&batch)
			action := obj(arr(batch["actions"])[0])
			if action["action"] == "create" {
				remoteNode = obj(arr(action["creates"])[0])
			} else if action["action"] == "update" {
				remoteNode = obj(arr(action["updates"])[0])
			}
		case "/api/canvas/v1/canvases/fixture-canvas":
			data = map[string]any{"nodes": []any{remoteNode}}
		case "/api/canvas/v1/canvases/fixture-canvas/connections:batchActions":
			data = map[string]any{}
		case "/api/conversation/v1/generations/image", "/api/conversation/v1/generations/video":
			lastKind = "image"
			if strings.HasSuffix(r.URL.Path, "/video") {
				lastKind = "video"
			}
			generations.Add(1)
			_ = json.NewDecoder(r.Body).Decode(&submitted)
			if obj(submitted["context"])["user_id"] != "fixture-user" || obj(submitted["metadata"])["canvas_id"] != "fixture-canvas" {
				t.Error("missing remote association", submitted)
			}
			if len(urlList(submitted["images"])) > 0 && strings.HasPrefix(urlList(submitted["images"])[0], "/files/") {
				t.Error("private URL forwarded")
			}
			if malformed {
				fmt.Fprint(w, `{"code":0,"data":{"result":{}}}`)
				return
			}
			data = map[string]any{"result": map[string]any{"ids": []string{"fixture-task"}}}
		case "/api/conversation/v1/generations/tasks":
			if r.URL.Query().Get("ids") != "fixture-task" {
				t.Error("task IDs")
			}
			outputKey := "image_urls"
			if lastKind == "video" {
				outputKey = "video_urls"
			}
			data = []any{map[string]any{"id": "fixture-task", "status": "completed", "outputs": map[string]any{outputKey: []string{fixtureURL + "/output"}}}}
		default:
			t.Error("unexpected path", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	t.Cleanup(srv.Close)
	fixtureURL = srv.URL
	c := New("", "fixture-token", "", t.TempDir(), "")
	c.origin = srv.URL
	c.download = tapnow.New(srv.URL, "")
	c.http = srv.Client()
	c.PollInterval = time.Millisecond
	c.Timeout = 15 * time.Second
	return c, &generations, &creates
}
func TestProduceOnceDurableAndPrivateUpload(t *testing.T) {
	c, g, creates := fixture(t, "2.5", false)
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "image_to_image")
	r := Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), References: []tapnow.Reference{{Kind: "image", Role: "image", URL: "/files/private", Order: 0}}, UploadConsent: true, MaxCost: 3}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureImage(), Mime: "image/png"}, nil
	}
	checkpoint := false
	media, ticket, e := c.Produce(context.Background(), "fixture-job-001", "alice", "fixture-local", r, resolve, func(Ticket) { checkpoint = true })
	if e != nil || !checkpoint || len(media) != 1 || len(ticket.TaskIDs) != 1 || g.Load() != 1 {
		t.Fatal(media, ticket, e)
	}
	if creates.Load() != 1 {
		t.Fatal("no remote canvas")
	}
	receipt, ok := c.Receipt("fixture-job-001")
	if !ok || receipt.Owner != "alice" || receipt.Phase != "done" || receipt.Cost != 2.5 {
		t.Fatal(receipt, ok)
	}
	if _, _, e = c.Produce(context.Background(), "fixture-job-001", "alice", "fixture-local", r, resolve, nil); e == nil || g.Load() != 1 {
		t.Fatal("duplicate paid submission")
	}
	fresh := New("", "fixture-token", "", c.dir, "")
	if receipt, ok = fresh.Receipt("fixture-job-001"); !ok || len(receipt.Ticket.TaskIDs) != 1 {
		t.Fatal("restart lost receipt")
	}
	b, _ := os.ReadFile(filepath.Join(c.dir, "job-fixture-job-001.json"))
	if strings.Contains(string(b), "fixture-token") {
		t.Fatal("credential persisted")
	}
}

func fixtureImage() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	return b.Bytes()
}
func fixtureVideo(seconds uint32) []byte {
	b := append([]byte(nil), tapnowtest.Video...)
	var change func([]byte)
	change = func(data []byte) {
		for off := 0; off+8 < len(data); {
			n := int(binary.BigEndian.Uint32(data[off:]))
			if n < 8 || off+n > len(data) {
				return
			}
			body := data[off+8 : off+n]
			switch string(data[off+4 : off+8]) {
			case "moov":
				change(body)
			case "mvhd":
				if len(body) >= 20 && body[0] == 0 {
					binary.BigEndian.PutUint32(body[12:], 1000)
					binary.BigEndian.PutUint32(body[16:], seconds*1000)
				}
			}
			off += n
		}
	}
	change(b)
	return b
}
func TestEstimateAndUnknownNeverGenerateAgain(t *testing.T) {
	for _, tc := range []struct {
		quote      string
		bad        bool
		generation int32
	}{{"3.5", false, 0}, {"2garbage", false, 0}, {"NaN", false, 0}, {"2", true, 1}} {
		t.Run(tc.quote, func(t *testing.T) {
			c, g, _ := fixture(t, tc.quote, tc.bad)
			_, v, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
			r := Request{Model: "nano-banana-flash", Mode: v.ID, Kind: "image", Prompt: "test", Parameters: defaults(v), MaxCost: 3}
			if _, _, e := c.Produce(context.Background(), "fixture-unknown", "alice", "fixture-local", r, nil, nil); e == nil {
				t.Fatal("should reject")
			}
			if g.Load() != tc.generation {
				t.Fatal("paid submission count", g.Load())
			}
			if _, _, e := c.Produce(context.Background(), "fixture-unknown", "alice", "fixture-local", r, nil, nil); e == nil || g.Load() != tc.generation {
				t.Fatal("retry not blocked")
			}
		})
	}
}
func TestRequestAuthRedactionAndOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, origin, wantOrigin, wantToken string
	}{
		{"default", "", "https://app.tapnow.media", "secret"},
		{"media", "https://app.tapnow.media", "https://app.tapnow.media", "secret"},
		{"legacy", "https://app.tapnow.ai", "https://app.tapnow.ai", "secret"},
		{"untrusted", "https://evil.example", "https://app.tapnow.media", ""},
		{"lookalike", "https://app.tapnow.media.evil.example", "https://app.tapnow.media", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.origin, "secret", "", t.TempDir(), "")
			if c.origin != tc.wantOrigin || c.token != tc.wantToken || c.Status().Connected {
				t.Fatal("unexpected origin or credential state")
			}
		})
	}
	c, g, _ := fixture(t, "2", false)
	if c.Status().Connected {
		t.Fatal("configured falsely connected")
	}
	if e := c.Probe(context.Background()); e != nil || !c.Status().Connected {
		t.Fatal(e)
	}
	b, _ := json.Marshal(c.Status())
	if strings.Contains(string(b), "fixture-token") || strings.Contains(string(b), "fixture-user") {
		t.Fatal("status credential/identity leak")
	}
	if g.Load() != 0 {
		t.Fatal("probe generated")
	}
}
func TestRefreshFailurePreservesCatalog(t *testing.T) {
	c := New("", "", "", t.TempDir(), "missing-script.mjs")
	old := c.Status().Catalog.Asset
	if c.Refresh(context.Background()) == nil || c.Status().Catalog.Asset != old {
		t.Fatal("bad refresh replaced catalog")
	}
}
