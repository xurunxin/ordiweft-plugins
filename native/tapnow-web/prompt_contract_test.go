package tapnowweb

import (
	"encoding/json"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestH3PublishedPromptContract(t *testing.T) {
	if got := h3WebPrompt("{{Image 2}} {{Video 1}} {{Audio 1}} @Image3 @https://example.org/a"); got != "Image 2 Video 1 Audio 1 Image3 https://example.org/a" {
		t.Fatal(got)
	}
	c := Baseline()
	_, mode, e := c.Find("MiniMax-H3", "video", "reference_to_video")
	if e != nil {
		t.Fatal(e)
	}
	r := Request{Kind: "video", Model: "MiniMax-H3", Mode: "reference_to_video", Prompt: "{{Image 1}} with {{Video 1}} and {{Audio 1}}", Parameters: defaults(mode), VideoConsent: true, UploadConsent: true, References: []tapnow.Reference{{Kind: "image", Role: "image", URL: "/files/image", Order: 0}, {Kind: "video", Role: "video", URL: "/files/video", Order: 1}, {Kind: "audio", Role: "audio", URL: "/files/audio", Order: 2}}}
	b, _, e := c.Prepare(r)
	if e != nil {
		t.Fatal(e)
	}
	if b["prompt"] != "Image 1 with Video 1 and Audio 1" || b["reference_image_urls"] == nil || b["reference_video_urls"] == nil || b["reference_audio_urls"] == nil || b["first_frame_image_url"] != nil {
		t.Fatal(b)
	}
	r.Model = "seedance-2.5"
	_, seed, e := c.Find(r.Model, "video", r.Mode)
	if e != nil {
		t.Fatal(e)
	}
	r.Parameters = defaults(seed)
	b, _, e = c.Prepare(r)
	if e != nil {
		t.Fatal(e)
	}
	if b["prompt"] != "Image1 with Video1 and Audio1" || !strings.HasPrefix(b["model"].(string), "agent-video-generation-") || b["reference_image_urls"] != nil {
		t.Fatal(b)
	}
	images := b["images"].([]any)
	if images[0].(map[string]any)["role"] != "reference_image" {
		t.Fatal(b)
	}
}

func TestH3MaxOnlySupportsTextAndFirstFrame(t *testing.T) {
	c := Baseline()
	ref := tapnow.Reference{Kind: "image", URL: "/files/first"}
	r := Request{Model: "MiniMax-H3-Max", Kind: "video", Mode: "text_to_video", Prompt: "fixture", VideoConsent: true, UploadConsent: true}
	body, _, err := c.Prepare(r)
	if err != nil || body["resolution"] != "768P" || body["model"] != "MiniMax-H3-Max" {
		t.Fatal(body, err)
	}
	r.Mode, r.References = "image_to_video", []tapnow.Reference{ref}
	r.Parameters = map[string]any{"resolution": "480P", "duration": float64(5)}
	body, _, err = c.Prepare(r)
	if err != nil || body["resolution"] != "480P" || body["first_frame_image_url"] != ref.URL || body["ratio"] != nil {
		t.Fatal(body, err)
	}
	for _, parameters := range []map[string]any{{"resolution": "2K"}, {"duration": float64(4)}} {
		r.Parameters = parameters
		if body, _, err := c.Prepare(r); err == nil {
			t.Fatal("unsupported Max parameters reached generation", body)
		}
	}
	for _, mode := range []string{"reference_to_video", "start_end_to_video"} {
		r.Mode, r.Parameters = mode, nil
		r.References = []tapnow.Reference{ref, {Kind: "image", URL: "/files/last", Order: 1}}
		if body, _, err := c.Prepare(r); err == nil {
			t.Fatal("unsupported Max mode reached generation", body)
		}
	}

	// Previously saved refresh results could retain all the regular H3 variants.
	regular, _, _ := c.Find("MiniMax-H3", "video", "text_to_video")
	maxModel, _, _ := c.Find("MiniMax-H3-Max", "video", "text_to_video")
	oldMax := merge(maxModel.Raw, map[string]any{"variants": regular.Raw["variants"]})
	raw, err := json.Marshal(rawCatalog{Source: c.Source, Asset: c.Asset, Models: []map[string]any{oldMax}})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = saved.Find(r.Model, r.Kind, "reference_to_video"); err == nil || !strings.Contains(err.Error(), "单首帧") {
		t.Fatal("saved directory revived an unsupported Max reference mode", err)
	}
}

func TestDoubaoSeedanceProKeepsFirstAndLastFrameOrder(t *testing.T) {
	r := Request{Model: "doubao-seedance-pro", Kind: "video", Mode: "start_end_to_video", Prompt: "fixture", VideoConsent: true, UploadConsent: true,
		Parameters: map[string]any{"aspectRatio": "9:16"}, References: []tapnow.Reference{
			{Kind: "image", URL: "/files/last", Order: 1}, {Kind: "image", URL: "/files/first"},
		}}
	body, _, err := Baseline().Prepare(r)
	if err != nil {
		t.Fatal(err)
	}
	images := urlList(body["images"])
	if body["provider"] != "bytedance" || body["model"] != "doubao-seedance-pro" || body["ratio"] != "9:16" ||
		len(images) != 2 || images[0] != "/files/first" || images[1] != "/files/last" {
		t.Fatal(body)
	}
}

func TestSeedanceRouteAndFrameParameters(t *testing.T) {
	c := Baseline()
	for _, tc := range []struct {
		model, wireModel, resolution string
		search, draft                bool
	}{
		{"seedance-2.0", "agent-video-generation-01", "720p", true, false},
		{"seedance-2.0-fast", "agent-video-generation-01-fast", "720p", false, false},
		{"seedance-2.0-mini", "agent-video-generation-01-mini", "480p", false, false},
		{"seedance-2.5", "agent-video-generation-02", "720p", true, false},
		{"seedance-2.5-4k", "agent-video-generation-02", "4k", true, false},
		{"seedance-2.5-draft", "agent-video-generation-02", "480p", true, true},
	} {
		t.Run(tc.model, func(t *testing.T) {
			r := Request{Model: tc.model, Kind: "video", Prompt: "fixture", VideoConsent: true, UploadConsent: true}
			for _, mode := range []string{"text_to_video", "image_to_video", "start_end_to_video"} {
				r.Mode, r.References, r.Parameters = mode, nil, nil
				switch mode {
				case "text_to_video":
					r.Parameters = map[string]any{"enableWebSearch": true}
				case "image_to_video":
					r.References = []tapnow.Reference{{Kind: "image", URL: "/files/first"}}
				case "start_end_to_video":
					r.References = []tapnow.Reference{{Kind: "image", URL: "/files/last", Order: 1}, {Kind: "image", URL: "/files/first"}}
				}
				body, _, err := c.Prepare(r)
				if err != nil || body["model"] != tc.wireModel || body["resolution"] != tc.resolution || (body["draft"] == true) != tc.draft {
					t.Fatal(mode, body, err)
				}
				if mode == "text_to_video" {
					if body["ratio"] == nil || (body["enable_web_search"] == true) != tc.search {
						t.Fatal(mode, body)
					}
				} else {
					frames := arr(body["images"])
					if body["ratio"] != nil || len(frames) != len(r.References) || obj(frames[0])["role"] != "first_frame" || obj(frames[0])["url"] != "/files/first" {
						t.Fatal("frame ratio must be derived upstream", mode, body)
					}
					if len(frames) == 2 && obj(frames[1])["role"] != "last_frame" {
						t.Fatal("missing end frame", body)
					}
				}
			}
		})
	}
}
