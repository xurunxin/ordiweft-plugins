// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestMagiModesUploadAndSubmitOriginalOnce(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	const ticket = "7ee399ba-d249-439a-8971-15b468740a03"
	const storage = "fnf-api-input-prod-20250414194641741400000002.s3.amazonaws.com"
	png := []byte("\x89PNG\r\n\x1a\nfixture")
	mp4 := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	wav := []byte("RIFF\x00\x00\x00\x00WAVEfixture")
	for _, tc := range []struct {
		mode, prompt string
		inputs       []aigc.Input
		want         map[string]any
		poll         bool
	}{
		{mode: "text-to-video", prompt: "A yellow square", poll: true},
		{
			mode:   "image-to-video",
			inputs: []aigc.Input{{Kind: "image", Role: "end_image_url", Data: png}, {Kind: "image", Role: "image_url", Data: png}},
			want:   map[string]any{"image_url": "https://media.example.invalid/2", "end_image_url": "https://media.example.invalid/1"},
		},
		{
			mode:   "reference-to-video",
			inputs: []aigc.Input{{Kind: "image", Data: png}, {Kind: "video", Data: mp4}, {Kind: "audio", Data: wav}},
			want: map[string]any{
				"image_urls": []any{"https://media.example.invalid/1"},
				"video_urls": []any{"https://media.example.invalid/2"},
				"audio_urls": []any{"https://media.example.invalid/3"},
			},
		},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var grants, puts, submits, observations, checkpointCount int
			base := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = base })
			complete := fmt.Sprintf(`{"request_id":%q,"status":"completed","video":{"url":"https://media.example.invalid/output.mp4"}}`, ticket)
			http.DefaultTransport = transportFunc(func(req *http.Request) (*http.Response, error) {
				status, result := 200, "{}"
				if req.URL.Host == "api.higgsfield.ai" {
					if req.Header.Get("Authorization") != "Key test-key:test-secret" {
						t.Fatal("API request did not authenticate")
					}
				} else if req.Header.Get("Authorization") != "" {
					t.Fatal("API credential sent to storage")
				}
				switch {
				case req.Method == "POST" && req.URL.Path == "/files/generate-upload-url":
					grants++
					var grant map[string]string
					if err := json.NewDecoder(req.Body).Decode(&grant); err != nil {
						t.Fatal(err)
					}
					result = fmt.Sprintf(`{"public_url":"https://media.example.invalid/%d","upload_url":"https://%s/%d","upload_headers":{"Content-Type":%q,"x-amz-tagging":"retention=temporary"}}`, grants, storage, grants, grant["content_type"])
				case req.Method == "PUT" && req.URL.Host == storage:
					data, err := io.ReadAll(req.Body)
					if err != nil || puts >= len(tc.inputs) || !bytes.Equal(data, tc.inputs[puts].Data) ||
						req.Header.Get("X-Amz-Tagging") != "retention=temporary" {
						t.Fatal("storage PUT did not send the matching input bytes and headers", err)
					}
					puts++
				case req.Method == "POST" && req.URL.Path == "/"+magiPrefix+tc.mode:
					submits++
					if req.Header.Get("Idempotency-Key") == "" || req.GetBody != nil || puts != len(tc.inputs) {
						t.Fatal("submission before uploads or eligible for replay")
					}
					var got map[string]any
					if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
						t.Fatal(err)
					}
					want := map[string]any{"duration": float64(4), "resolution": "1080p", "aspect_ratio": "16:9", "generate_audio": false, "seed": float64(4294967295)}
					for k, v := range tc.want {
						want[k] = v
					}
					if tc.prompt != "" {
						want["prompt"] = tc.prompt
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("wrong request body: got %v, want %v", got, want)
					}
					result = complete
					if tc.poll {
						result = fmt.Sprintf(`{"request_id":%q,"status":"queued","status_url":"https://api.higgsfield.ai/requests/%s/status"}`, ticket, ticket)
					}
				case req.Method == "GET" && req.URL.Path == "/requests/"+ticket+"/status":
					observations++
					if checkpointCount != 1 {
						t.Fatal("original ticket was not saved before polling")
					}
					result = complete
				default:
					t.Fatalf("unexpected provider request %s %s", req.Method, req.URL)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(result)), Header: http.Header{}}, nil
			})
			request := magiRequest(t, tc.mode, tc.prompt, tc.inputs)
			if err := p.Validate(t.Context(), request); err != nil || grants+puts+submits+observations != 0 {
				t.Fatal("preflight should be offline", err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			result, err := p.Execute(ctx, request, func(result aigc.Result) {
				var metadata map[string]any
				if json.Unmarshal(result.Metadata, &metadata) != nil || metadata["requestId"] != ticket || len(result.Media) != 0 {
					t.Fatal("invalid receipt checkpoint")
				}
				checkpointCount++
			})
			wantPolls := 0
			if tc.poll {
				wantPolls = 1
			}
			if err != nil || submits != 1 || observations != wantPolls || checkpointCount != 1 || len(result.Media) != 1 ||
				result.Media[0].Kind != "video" || result.Media[0].URL != "https://media.example.invalid/output.mp4" {
				t.Fatalf("wrong original result: %+v, %v; submits=%d observations=%d", result, err, submits, observations)
			}
		})
	}
}

func TestMagiReferenceRequiresMediaAndRejectsUnavailableWithoutResubmitting(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	ref := magiRequest(t, "reference-to-video", "", nil)
	if err := p.Validate(t.Context(), ref); err == nil {
		t.Fatal("empty reference request accepted")
	}
	base := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = base })
	submits := 0
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		submits++
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"detail":"model_unavailable"}`)), Header: http.Header{}}, nil
	})
	_, err = p.Execute(t.Context(), magiRequest(t, "text-to-video", "A square", nil), func(aigc.Result) { t.Fatal("false receipt after rejected submission") })
	if err == nil || submits != 1 {
		t.Fatalf("rejected submission: %v, count %d", err, submits)
	}
}

func magiRequest(t *testing.T, mode, prompt string, refs []aigc.Input) aigc.Request {
	t.Helper()
	body, err := json.Marshal(aigc.Generation{Provider: providerID, Capability: "video.generate", Model: magiPrefix + mode, Prompt: prompt,
		Parameters: map[string]any{"duration": 4, "generate_audio": false, "seed": 4294967295}})
	if err != nil {
		t.Fatal(err)
	}
	return aigc.Request{JobID: "test-job", Payload: body, Inputs: refs, Secrets: map[string]string{credentialName: "test-key:test-secret"}}
}
