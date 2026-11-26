package tapnowweb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestReferenceGraphFailureDoesNotSubmit(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	transport := c.http.Transport
	connections := 0
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "connections:batchActions") {
			connections++
			return nil, errors.New("lost reference connection response")
		}
		return transport.RoundTrip(r)
	})
	_, mode, err := c.catalog.Find("seedance-2.5", "video", "reference_to_video")
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Model: "seedance-2.5", Mode: "reference_to_video", Kind: "video", Prompt: "reference", Parameters: defaults(mode), References: []tapnow.Reference{{Kind: "image", Role: "image", URL: "/files/reference"}}, VideoConsent: true, UploadConsent: true, MaxCost: 3}
	_, _, err = c.Produce(context.Background(), "graph-creation-original", "alice", "fixture-local", request, func(context.Context, string) (Material, error) { return Material{Bytes: fixtureImage()}, nil }, nil)
	if err == nil || connections != 1 || paid.Load() != 0 {
		t.Fatalf("unconfirmed graph was submitted or retried: error=%v connections=%d generations=%d", err, connections, paid.Load())
	}
}

func TestSeedanceReferenceMediaAndCanvasAgree(t *testing.T) {
	for _, modeID := range []string{"reference_to_video", "video_edit"} {
		t.Run(modeID, func(t *testing.T) {
			c, paid, _ := fixture(t, "2", false)
			var generation map[string]any
			var nodes, edges []any
			reviewReads := 0
			transport := http.DefaultTransport
			c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/canvas/v1/asset-reviews" {
					reviewReads++
					if r.Method != http.MethodGet || r.URL.Query().Get("file_ids") != "fixture-file-png" || len(r.URL.Query()["file_ids"]) != 1 {
						t.Error("review must read the original image only", r.Method, r.URL.RawQuery)
					}
					status := "ASSET_REVIEW_STATUS_PENDING"
					if modeID == "reference_to_video" {
						status = "ASSET_REVIEW_STATUS_APPROVED"
					}
					data, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"reviews": []map[string]string{
						{"file_id": "fixture-file-png", "status": status},
						{"file_id": "fixture-file-mp4", "status": "ASSET_REVIEW_STATUS_APPROVED"},
					}}})
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
				}
				if r.Method == "POST" && (strings.HasSuffix(r.URL.Path, "nodes:batchActions") || strings.HasSuffix(r.URL.Path, "connections:batchActions") || strings.HasSuffix(r.URL.Path, "/generations/video")) {
					b, _ := io.ReadAll(r.Body)
					r.Body = io.NopCloser(strings.NewReader(string(b)))
					var body map[string]any
					_ = json.Unmarshal(b, &body)
					if strings.HasSuffix(r.URL.Path, "/generations/video") {
						generation = body
					} else {
						action := obj(arr(body["actions"])[0])
						if action["action"] == "create" {
							if strings.HasSuffix(r.URL.Path, "nodes:batchActions") {
								nodes = arr(action["creates"])
							} else {
								edges = arr(action["creates"])
							}
						}
					}
				}
				return transport.RoundTrip(r)
			})
			_, mode, err := c.catalog.Find("seedance-2.5", "video", modeID)
			if err != nil {
				t.Fatal(err)
			}
			request := Request{Model: "seedance-2.5", Mode: modeID, Kind: "video", Prompt: "{{Video 1}} motion and {{Image 1}} appearance", Parameters: defaults(mode), References: []tapnow.Reference{{Kind: "video", Role: "video", URL: "/files/video", Order: 0}, {Kind: "image", Role: "image", URL: "/files/image", Order: 1}}, VideoConsent: true, UploadConsent: true, MaxCost: 3}
			resolver := func(_ context.Context, src string) (Material, error) {
				if src == "/files/video" {
					return Material{Bytes: fixtureVideo(5), Mime: "video/mp4"}, nil
				}
				return Material{Bytes: fixtureImage(), Mime: "image/png"}, nil
			}
			_, _, err = c.Produce(context.Background(), "reference-media-original", "alice", "fixture-local", request, resolver, nil)
			if err != nil || paid.Load() != 1 || reviewReads != 1 {
				t.Fatal(err, paid.Load(), reviewReads)
			}
			if len(arr(generation["videos"])) != 1 || len(arr(generation["images"])) != 1 || obj(arr(generation["videos"])[0])["role"] != "reference_video" || obj(arr(generation["images"])[0])["role"] != "reference_image" {
				t.Fatalf("lost media roles: %v", generation)
			}
			if generation["prompt"] != "Video1 motion and Image1 appearance" {
				t.Fatal(generation["prompt"])
			}
			if obj(arr(generation["images"])[0])["is_verified"] != (modeID == "reference_to_video") || obj(arr(generation["videos"])[0])["is_verified"] != false {
				t.Fatal("only a confirmed original image can carry approval", generation)
			}
			if str(obj(arr(generation["videos"])[0]), "url") == str(obj(arr(generation["images"])[0]), "url") {
				t.Fatal("distinct media bytes were bound to the same upload")
			}
			if modeID == "video_edit" && number(generation["duration"]) != -1 {
				t.Fatal("editing must follow source duration", generation)
			}
			if modeID == "reference_to_video" && number(generation["duration"]) == -1 {
				t.Fatal("reference video must not silently become editing", generation)
			}
			if len(nodes) != 3 || len(edges) != 2 {
				t.Fatal("reference previews missing", nodes, edges)
			}
			for i, kind := range []string{"video", "image"} {
				node := obj(nodes[i+1])
				edge := obj(edges[i])
				data := obj(node["data"])
				if data["src"] != obj(arr(generation[map[string]string{"video": "videos", "image": "images"}[kind]])[0])["url"] {
					t.Fatal("preview differs from generated input", node, generation)
				}
				if node["type"] != kind || data["type"] != "upload" || strings.HasPrefix(str(data, "src"), "/files/") || edge["source"] != node["id"] || edge["target"] != obj(nodes[0])["id"] || obj(edge["data"])["purpose"] != "generation-input" {
					t.Fatal(node, edge)
				}
			}
		})
	}
}
