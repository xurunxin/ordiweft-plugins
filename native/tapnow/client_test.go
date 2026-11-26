package tapnow_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnowtest"
)

func fixture(t *testing.T) (*tapnow.Client, *tapnowtest.Fake) {
	t.Helper()
	f := tapnowtest.New()
	s := httptest.NewServer(f)
	f.URL = s.URL
	t.Cleanup(s.Close)
	c := tapnow.New(s.URL+"/mcp", t.TempDir())
	c.PollImage = time.Millisecond
	c.PollVideo = time.Millisecond
	if e := c.Discover(context.Background()); e != nil {
		t.Fatal(e)
	}
	return c, f
}
func TestNativeReferenceSchemaAndOutputAssociation(t *testing.T) {
	c, f := fixture(t)
	f.Multi = true
	media, ticket, e := c.Produce(context.Background(), "multi-native-source", "fixture", tapnow.Request{Kind: "image", Tool: "create_hero_image", Model: "fast", Prompt: "synthetic"}, func(tapnow.Ticket) {})
	if e != nil || len(media) != 2 || media[0].NodeID != "fixture-a" || media[1].NodeID != "fixture-b" || ticket.NodeIDs[1] != "fixture-b" {
		t.Fatalf("unordered association %+v %+v %v", media, ticket, e)
	}
	r := tapnow.Request{Kind: "video", Tool: "create_hero_video", Model: "draft", Prompt: "move", VideoConsent: true, References: []tapnow.Reference{{Kind: "image", Role: "image_id", URL: "/files/local", Native: &tapnow.NativeImage{ProjectID: "fixture-project", NodeID: media[1].NodeID}}}}
	args, e := c.Arguments(context.Background(), r)
	if e != nil || args["image_id"] != "fixture-b" || args["project_id"] != "fixture-project" || f.Count("upload_image") != 0 {
		t.Fatalf("native arguments %+v %v", args, e)
	}
	t.Run("upload-only field", func(t *testing.T) {
		r.Kind = "image"
		r.Tool = "create_deck_visuals"
		r.Model = ""
		r.Prompt = ""
		r.Parameters = map[string]any{"outline": []any{"synthetic"}}
		r.References[0].Role = "style_ref_image_id"
		if _, _, e := c.Prepare(context.Background(), r); e == nil {
			t.Fatal("upload-only field accepted native node")
		}
	})
	if f.Count("upload_image") != 0 || f.Count("create_hero_video") != 0 || f.Count("create_deck_visuals") != 0 {
		t.Fatal("preflight rejection made calls")
	}
}
func TestDiscoveryOAuthAndSSE(t *testing.T) {
	c, f := fixture(t)
	f.SSE = true
	f.Pagination = true
	if e := c.Discover(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(c.Status().Services) != 4 {
		t.Fatalf("wrong services %+v", c.Status())
	}
	authorize, e := c.Start(context.Background(), f.URL+"/callback", "admin")
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(authorize)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("resource") == "" || q.Get("code_challenge") == "" {
		t.Fatal("PKCE/resource absent")
	}
	if e = c.Complete(context.Background(), q.Get("state"), "code", "member"); e == nil {
		t.Fatal("wrong owner accepted")
	}
	if e = c.Complete(context.Background(), q.Get("state"), "code", "admin"); e != nil {
		t.Fatal(e)
	}
	if e = c.Complete(context.Background(), q.Get("state"), "code", "admin"); e == nil {
		t.Fatal("state replay accepted")
	}
	if e = c.Discover(context.Background()); e != nil {
		t.Fatal(e)
	}
	if f.Authorization[len(f.Authorization)-1] != "Bearer fixture-access" {
		t.Fatal("independent OAuth token not used by transport")
	}
	b, _ := json.Marshal(c.Status())
	if strings.Contains(string(b), "fixture-access") || strings.Contains(string(b), "refresh_token") {
		t.Fatal("credential leaked")
	}
	if e = c.Disconnect(); e != nil {
		t.Fatal(e)
	}
}
func TestActualSchemaRejectsUnsupportedPayload(t *testing.T) {
	c, _ := fixture(t)
	base := tapnow.Request{Kind: "video", Tool: "create_hero_video", Model: "draft", Prompt: "move", VideoConsent: true, Parameters: map[string]any{"duration": float64(5)}}
	if _, _, e := c.Prepare(context.Background(), base); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		name   string
		mutate func(*tapnow.Request)
	}{
		{"website model", func(r *tapnow.Request) { r.Model = "Seedance2.5" }},
		{"duration enum", func(r *tapnow.Request) { r.Parameters = map[string]any{"duration": float64(6)} }},
		{"extra field", func(r *tapnow.Request) { r.Parameters = map[string]any{"generate_audio": true} }},
		{"video input", func(r *tapnow.Request) {
			r.References = []tapnow.Reference{{Kind: "video", Role: "video_url", URL: "https://example.com/video.mp4"}}
		}},
		{"private input", func(r *tapnow.Request) {
			r.References = []tapnow.Reference{{Kind: "image", Role: "image_id", URL: "/files/private"}}
		}},
		{"consent", func(r *tapnow.Request) { r.VideoConsent = false }},
		{"model in parameters", func(r *tapnow.Request) { r.Parameters = map[string]any{"model": "quality"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := base
			test.mutate(&r)
			if _, _, e := c.Prepare(context.Background(), r); e == nil {
				t.Fatal("unsupported paid payload accepted")
			}
		})
	}
}
func TestProductionTicketUploadAndFutureReferences(t *testing.T) {
	c, f := fixture(t)
	f.Extended = true
	if e := c.Discover(context.Background()); e != nil {
		t.Fatal(e)
	}
	r := tapnow.Request{Kind: "video", Tool: "create_hero_video", Model: "draft", Prompt: "move", VideoConsent: true, References: []tapnow.Reference{{Kind: "video", Role: "video_urls", URL: f.URL + "/media/video.mp4", Order: 9}, {Kind: "video", Role: "video_urls", URL: f.URL + "/media/video.mp4?first", Order: 1}, {Kind: "image", Role: "image_id", URL: f.URL + "/media/image.png", Order: 0}}}
	checkpoints := 0
	media, ticket, e := c.Produce(context.Background(), "fixture-future-job", "alice", r, func(checkpoint tapnow.Ticket) {
		checkpoints++
		if checkpoint.ProjectID == "" || len(checkpoint.NodeIDs) == 0 {
			t.Fatalf("bad checkpoint")
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	if checkpoints != 1 || len(media) != 1 || media[0].Kind != "video" {
		t.Fatal("result mapping")
	}
	args := f.Argument("create_hero_video")
	if args["image_id"] != "fixture-image" || args["project_id"] != "fixture-project" {
		t.Fatal("upload IDs not used")
	}
	videos := args["video_urls"].([]any)
	if videos[0] != f.URL+"/media/video.mp4?first" {
		t.Fatal("video order lost")
	}
	poll := f.Argument("get_production_result")
	if poll["project_id"] != ticket.ProjectID {
		t.Fatal("poll project lost")
	}
	if _, _, e = c.Produce(context.Background(), "fixture-future-job", "alice", r, func(tapnow.Ticket) {}); e == nil || f.Count("create_hero_video") != 1 {
		t.Fatal("duplicate billed")
	}
}
func TestProductionTerminalAndAmbiguousNoRetry(t *testing.T) {
	for _, mode := range []string{"failed", "not_found", "ambiguous", "isError", "pending"} {
		t.Run(mode, func(t *testing.T) {
			c, f := fixture(t)
			f.Mode = mode
			c.Timeout = 200 * time.Millisecond
			r := tapnow.Request{Kind: "image", Tool: "create_hero_image", Model: "fast", Prompt: "synthetic"}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			_, _, e := c.Produce(ctx, "fixture-terminal-job", "alice", r, func(tapnow.Ticket) {})
			if e == nil {
				t.Fatal("missing error")
			}
			if f.Count("create_hero_image") != 1 {
				t.Fatal("submit count")
			}
			if (mode == "failed" || mode == "not_found") && f.Count("get_production_result") != 1 {
				t.Fatal("terminal continued polling")
			}
			if strings.Contains(e.Error(), "private-provider-secret") {
				t.Fatal("raw provider error leaked")
			}
			receipt, ok := c.Receipt("fixture-terminal-job")
			if !ok || receipt.Owner != "alice" {
				t.Fatal("receipt lost")
			}
		})
	}
}

func TestRemotePollingOutlivesPreparationLimit(t *testing.T) {
	c, f := fixture(t)
	c.Timeout = time.Second
	media, ticket, err := c.Produce(context.Background(), "slow-original-ticket", "alice", tapnow.Request{Kind: "image", Tool: "create_hero_image", Model: "fast", Prompt: "synthetic"}, func(tapnow.Ticket) { time.Sleep(c.Timeout + 10*time.Millisecond) })
	if err != nil || len(media) != 1 || len(ticket.NodeIDs) == 0 || f.Count("create_hero_image") != 1 || f.Count("get_production_result") < 2 {
		t.Fatal("remote polling inherited preparation limit or resubmitted", err, ticket)
	}
}
