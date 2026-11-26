package tapnowweb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestNewImageAndVideoModelBodies(t *testing.T) {
	c := Baseline()
	for _, test := range []struct {
		name, kind, mode string
		parameters       map[string]any
		refs             []tapnow.Reference
		want             map[string]any
	}{
		{"fal-recraft-v4-vector", "image", "text_to_image", map[string]any{"generateMode": "pro", "aspectRatio": "16:9"}, nil,
			map[string]any{"model": "fal-recraft-v4-pro-vector", "image_size": "1024x576"}},
		{"hunyuan-image-3.5-preview", "image", "image_to_image", map[string]any{"imageSize": "4K"}, []tapnow.Reference{{Kind: "image", URL: "/files/image"}},
			map[string]any{"provider": "tencent", "scene": "image-edit", "resolution": "4k", "images": []string{"/files/image"}}},
		{"seedance-2.5-4k", "video", "text_to_video", nil, nil,
			map[string]any{"model": "agent-video-generation-02", "resolution": "4k", "draft": nil}},
		{"wan3.0-video", "video", "text_to_video", nil, []tapnow.Reference{{Kind: "audio", URL: "/files/audio"}},
			map[string]any{"model": "wan3.0-video", "reference_audio_urls": []string{"/files/audio"}, "prompt": "Audio: 音频1"}},
		{"wanxiang-v2-6", "video", "text_to_video", nil, []tapnow.Reference{{Kind: "audio", URL: "/files/audio"}},
			map[string]any{"model": "wanxiang-2.6-t2v", "audio_url": "/files/audio", "audio": nil}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt := "fixture"
			if test.name == "wan3.0-video" {
				prompt = "Audio: {{Audio 1}}"
			}
			body, _, err := c.Prepare(Request{Model: test.name, Kind: test.kind, Mode: test.mode, Prompt: prompt, References: test.refs,
				Parameters: test.parameters, VideoConsent: true, UploadConsent: true})
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range test.want {
				if !reflect.DeepEqual(body[key], value) {
					t.Errorf("%s = %#v, want %#v", key, body[key], value)
				}
			}
		})
	}
}

func TestKling4KeyframesKeepOrderAndAllowAnEmptyPrompt(t *testing.T) {
	r := Request{Model: "kling-4.0-preview", Kind: "video", Mode: "start_end_to_video", VideoConsent: true, UploadConsent: true,
		Parameters: map[string]any{"duration": float64(30)}, References: []tapnow.Reference{
			{Kind: "image", URL: "/files/last", Order: 20},
			{Kind: "image", URL: "/files/first", Order: 1},
			{Kind: "image", URL: "/files/middle", Order: 10},
		}}
	c := Baseline()
	body, _, err := c.Prepare(r)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		map[string]any{"id": "image_1", "type": "first_frame", "url": "/files/first"},
		map[string]any{"id": "image_2", "type": "middle_frame", "url": "/files/middle"},
		map[string]any{"id": "image_3", "type": "last_frame", "url": "/files/last"},
	}
	if body["kling_operation"] != "image-to-video" || !reflect.DeepEqual(body["kling_contents"], want) {
		t.Fatal(body)
	}
	r.References = r.References[:1]
	if _, _, err := c.Prepare(r); err == nil {
		t.Fatal("a keyframe sequence needs at least two images")
	}
}

func TestVideoInputsReachQuoteAndSingleSubmission(t *testing.T) {
	for _, model := range []string{"omnihuman-1.5", "kling-4.0-preview", "MiniMax-H3"} {
		t.Run(model, func(t *testing.T) {
			c, count, _ := fixture(t, "2", false)
			r := Request{Model: model, Kind: "video", Mode: "digital_human", VideoConsent: true, UploadConsent: true, MaxCost: 3,
				References: []tapnow.Reference{
					{Kind: "image", URL: "/files/image", Order: 0},
					{Kind: "audio", URL: "/files/audio", Order: 1},
				}}
			if model != "omnihuman-1.5" {
				r.Mode, r.Prompt = "reference_to_video", "Use {{Image 1}} with {{Video 1}} and {{Audio 1}}"
				if model == "kling-4.0-preview" {
					r.Mode = "video_edit"
				}
				r.References = append(r.References, tapnow.Reference{Kind: "video", URL: "/files/video", Order: 2})
			}
			resolve := func(_ context.Context, url string) (Material, error) {
				switch url {
				case "/files/image":
					return Material{Bytes: fixtureImage()}, nil
				case "/files/audio":
					return Material{Bytes: toolWAV(), Duration: 600}, nil
				default:
					return Material{Bytes: fixtureVideo(4)}, nil
				}
			}
			transport := c.http.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			var quote, submitted map[string]any
			c.http.Transport = observeTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/billing/v2/estimate" || req.URL.Path == "/api/conversation/v1/generations/video" {
					payload, err := io.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					req.Body = io.NopCloser(bytes.NewReader(payload))
					body := map[string]any{}
					if err := json.Unmarshal(payload, &body); err != nil {
						return nil, err
					}
					if bytes.Contains(payload, []byte("/files/")) {
						t.Error("local asset path propagated to TapNow")
					}
					if req.URL.Path == "/api/billing/v2/estimate" {
						quote = obj(body["videoParams"])
					} else {
						submitted = body
					}
				}
				return transport.RoundTrip(req)
			})
			media, _, err := c.Produce(context.Background(), "model-inputs", "alice", "fixture-local", r, resolve, nil)
			if err != nil || count.Load() != 1 || len(media) != 1 || media[0].Kind != "video" {
				t.Fatal(media, count.Load(), err)
			}
			imageKey := "images"
			if model == "MiniMax-H3" {
				imageKey = "reference_images"
			}
			if len(urlList(quote[imageKey])) != 1 || len(urlList(quote["audios"])) != 1 {
				t.Fatal("quote lost references", quote)
			}
			switch model {
			case "omnihuman-1.5":
				if number(quote["duration"]) != 1 || submitted["duration"] != nil || submitted["prompt"] != nil ||
					!strings.HasSuffix(str(submitted, "audio_url"), "fixture-file-wav") {
					t.Fatal("OmniHuman voice or billing duration mismatch", submitted, quote)
				}
			case "kling-4.0-preview":
				contents := arr(submitted["kling_contents"])
				if submitted["prompt"] != "Use @image_1 with @video_1 and Audio 1" ||
					len(contents) != 2 || obj(contents[1])["type"] != "base_video" ||
					len(urlList(submitted["kling_audio_refs"])) != 1 || len(urlList(quote["videos"])) != 1 {
					t.Fatal("Kling edit inputs or roles mismatch", submitted, quote)
				}
			case "MiniMax-H3":
				if submitted["prompt"] != "Use Image 1 with Video 1 and Audio 1" || submitted["resolution"] != "2K" || quote["quality"] != "2K" ||
					len(urlList(submitted["reference_image_urls"])) != 1 || len(urlList(submitted["reference_video_urls"])) != 1 ||
					len(urlList(submitted["reference_audio_urls"])) != 1 || len(urlList(quote["videos"])) != 1 {
					t.Fatal("H3 multimodal inputs or quote mismatch", submitted, quote)
				}
			}
		})
	}
}
