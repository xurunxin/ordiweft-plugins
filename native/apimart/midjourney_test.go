// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestMidjourneyHDUsesOriginalButtonAndRecordsSource(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	var posted int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/tasks/task_grid":
			w.Write([]byte(`{"code":200,"data":{"id":"task_grid","status":"SUCCESS","buttons":[{"customId":"MJ::JOB::upsample_v7_2x_subtle::1::wrong"},{"customId":"MJ::JOB::upsample_v7_2x_subtle::3::original"}]}}`))
		case "POST /v1/midjourney/generations/upscale":
			posted++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["task_id"] != "task_grid" || body["custom_id"] != "MJ::JOB::upsample_v7_2x_subtle::3::original" || body["index"] != nil || body["upscale_style"] != nil {
				t.Error(body)
			}
			w.Write([]byte(`{"code":200,"data":[{"status":"submitted","task_id":"task_single"}]}`))
		case "GET /v1/tasks/task_single":
			body, _ := json.Marshal(map[string]any{"code": 200, "data": map[string]any{"id": "task_single", "status": "SUCCESS", "created": time.Now().Unix(), "image_urls": []string{"https://example.com/single.png"}}})
			w.Write(body)
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p.base = server.URL
	request := generation(t, "midjourney-follow-up", "image.generate", "", map[string]any{"operation": "upscale", "upscale_style": "subtle", "index": 3}, nil)
	request.SourceTask = &aigc.SourceTask{Ticket: "task_grid"}
	out, err := p.Execute(t.Context(), request, nil)
	if err != nil || posted != 1 || len(out.TaskOrigins) != 1 || out.TaskOrigins[0].Ticket != "task_single" {
		t.Fatalf("%+v %v, submits=%d", out, err, posted)
	}
	request = generation(t, "midjourney-follow-up", "image.generate", "", map[string]any{"operation": "upscale", "upscale_style": "creative", "index": 3}, nil)
	request.SourceTask = &aigc.SourceTask{Ticket: "task_grid"}
	if _, err = p.Execute(t.Context(), request, nil); err == nil || posted != 1 {
		t.Fatal("unsupported HD operation was submitted", err, posted)
	}
}

func TestMidjourneyMaskCompletesOriginalModalOnce(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	pixels.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	var opened, submitted int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/midjourney/generations/inpaint", "POST /v1/midjourney/generations/modal":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if strings.HasSuffix(r.URL.Path, "/inpaint") {
				opened++
				if body["task_id"] != "task_original_single" || body["mask_url"] != nil || body["prompt"] != nil {
					t.Error(body)
				}
			} else {
				submitted++
				url, _ := body["mask_url"].(string)
				if body["task_id"] != "task_modal" || body["prompt"] != "Color red" || body["model"] != nil || !strings.HasPrefix(url, "data:image/png;base64,") {
					t.Error(body)
				}
			}
			w.Write([]byte(`{"code":200,"data":[{"status":"submitted","task_id":"task_modal"}]}`))
		case "GET /v1/tasks/task_modal":
			if submitted == 0 {
				w.Write([]byte(`{"code":200,"data":{"id":"task_modal","status":"MODAL"}}`))
				return
			}
			w.Write([]byte(`{"code":200,"data":{"id":"task_modal","status":"SUCCESS","grid_image_url":"https://example.com/grid.png","image_urls":["https://example.com/one.png","https://example.com/two.png"]}}`))
		default:
			t.Error(r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p.base = server.URL
	request := generation(t, "midjourney-inpaint", "image.generate", "Color red", nil, []aigc.Input{{Kind: "image", Role: "mask", Data: encoded.Bytes()}})
	request.SourceTask = &aigc.SourceTask{Ticket: "task_original_single"}
	var checkpoints int
	out, err := p.Execute(t.Context(), request, func(out aigc.Result) {
		if !strings.Contains(string(out.Metadata), "task_modal") {
			t.Error(string(out.Metadata))
		}
		checkpoints++
	})
	if err != nil || opened != 1 || submitted != 1 || checkpoints != 2 || len(out.Media) != 2 {
		t.Fatalf("%+v %v, %d/%d/%d", out, err, opened, submitted, checkpoints)
	}
}
