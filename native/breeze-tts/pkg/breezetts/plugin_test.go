package breezetts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func requestFor(p *Plugin, model, prompt string, params map[string]any) aigc.Request {
	capability := "speech.generate"
	if model == "breeze-voice-design" || model == "breeze-voice-clone" {
		capability = "voice.design"
	}
	b, _ := json.Marshal(aigc.Generation{Provider: p.Manifest().ID, Capability: capability, Model: model, Prompt: prompt, Parameters: params})
	return aigc.Request{JobID: "host-test-1", Payload: b, Secrets: map[string]string{"API_KEY": "private-test-key"}}
}

func TestManifestAndModeValidation(t *testing.T) {
	for _, online := range []bool{false, true} {
		p := &Plugin{Online: online}
		if err := p.Manifest().Validate(); err != nil {
			t.Fatal(err)
		}
	}
	p := &Plugin{}
	for _, tc := range []struct {
		mode, instruction, transcript string
		reference, ok                 bool
	}{
		{"design", "温柔清晰的女声", "", false, true}, {"design", "", "", false, false},
		{"clone", "", "参考文字", true, true}, {"clone", "", "", true, false}, {"clone", "", "参考文字", false, false},
		{"direction", "轻声说话", "参考文字", true, true}, {"direction", "", "参考文字", true, false},
	} {
		t.Run(tc.mode+tc.instruction+tc.transcript+fmt.Sprint(tc.reference), func(t *testing.T) {
			r := requestFor(p, "breeze-tts-2-local", "你好", map[string]any{"mode": tc.mode, "instruction": tc.instruction, "ref_text": tc.transcript})
			if tc.reference {
				r.Inputs = []aigc.Input{{Kind: "audio", Role: "speaker"}}
			}
			if err := p.Validate(context.Background(), r); (err == nil) != tc.ok {
				t.Fatalf("validation=%v want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestLocalStreamingAndInactiveReference(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var posts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"status":"ok","sample_rate":24000}`)
		case "/v1/model":
			fmt.Fprintf(w, `{"model_digest":%q}`, digest)
		case "/v1/audio/speech":
			posts.Add(1)
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if r.FormValue("text") != "[笑]你好" || r.FormValue("instruction") != "温柔声音" || r.FormValue("cfg_scale") != "4" {
				t.Error("lost text/instruction/CFG")
			}
			if len(r.MultipartForm.File) > 0 || r.FormValue("ref_text") != "" {
				t.Error("inactive reference leaked")
			}
			w.Header().Set("Content-Type", "audio/pcm")
			w.Header().Set("X-Sample-Rate", "24000")
			w.Header().Set("X-Sample-Format", "s16le")
			_, _ = w.Write(make([]byte, 48000))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer s.Close()
	p := &Plugin{URL: s.URL, ModelDigest: digest}
	r := requestFor(p, "breeze-tts-2-local", "[笑]你好", map[string]any{"instruction": "温柔声音", "ref_text": "stale"})
	// References hidden by the selected mode remain in saved graphs.
	r.Inputs = []aigc.Input{{Kind: "audio", Role: "speaker", Data: []byte("stale")}}
	var receipts []aigc.Result
	result, err := p.Execute(context.Background(), r, func(v aigc.Result) { receipts = append(receipts, v) })
	if err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 || len(receipts) != 2 || len(result.Media) != 1 {
		t.Fatal("submission/checkpoint mismatch")
	}
	if err = validateWAV(result.Media[0].Data, false); err != nil {
		t.Fatal(err)
	}
	p.ModelDigest = strings.Repeat("b", 64)
	if _, err = p.Execute(context.Background(), r, nil); err == nil || posts.Load() != 1 {
		t.Fatal("mismatched model submitted")
	}
}

func TestReferenceCloneUsesUnguidedTemplate(t *testing.T) {
	digest := strings.Repeat("a", 64)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"status":"ok","sample_rate":24000}`)
		case "/v1/model":
			fmt.Fprintf(w, `{"model_digest":%q}`, digest)
		case "/v1/audio/speech":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("cfg_scale") != "1" || r.FormValue("instruction") != "" || r.FormValue("ref_text") != "参考逐字稿" || len(r.MultipartForm.File["ref_audio"]) != 1 {
				t.Error("invalid reference-only template inputs")
			}
			w.Header().Set("Content-Type", "audio/pcm")
			w.Header().Set("X-Sample-Rate", "24000")
			w.Header().Set("X-Sample-Format", "s16le")
			_, _ = w.Write(make([]byte, 48000))
		}
	}))
	defer s.Close()
	p := &Plugin{URL: s.URL, ModelDigest: digest}
	r := requestFor(p, "breeze-tts-2-local", "你好", map[string]any{"mode": "clone", "cfg_scale": 4, "instruction": "stale direction", "ref_text": "参考逐字稿"})
	r.Inputs = []aigc.Input{{Kind: "audio", Role: "speaker", Data: pcmWAV(make([]byte, 48000))}}
	result, err := p.Execute(context.Background(), r, nil)
	var metadata map[string]any
	_ = json.Unmarshal(result.Metadata, &metadata)
	if err != nil || metadata["cfgScale"] != float64(1) {
		t.Fatalf("result=%v error=%v", result.Metadata, err)
	}
}

func TestOnlineAsyncReceiptAndNoRetry(t *testing.T) {
	var posts, gets atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "private-test-key" {
			t.Error("missing key")
		}
		switch r.URL.Path {
		case "/v1/text-to-speech/voc_test":
			posts.Add(1)
			var body struct {
				Text         string `json:"text"`
				Instructions string `json:"instructions"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Text != "你好" {
				t.Error("Chinese text changed in the HTTP body")
			}
			if r.URL.Query().Get("delivery") != "async" || r.URL.Query().Get("output_format") != "wav_24000" {
				t.Error("wrong delivery")
			}
			w.WriteHeader(202)
			fmt.Fprint(w, `{"generation_job_id":"job_test","history_item_id":"hist_test","status":"queued"}`)
		case "/v1/generation-jobs/job_test":
			state := "admitted"
			if n := gets.Add(1); n == 2 {
				state = "streaming"
			} else if n > 2 {
				state = "ready"
			}
			fmt.Fprintf(w, `{"generation_job_id":"job_test","history_item_id":"hist_test","status":%q,"state":%q}`, state, state)
		case "/v1/generation-jobs/job_test/audio":
			_, _ = w.Write(pcmWAV(make([]byte, 48000)))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer s.Close()
	p := &Plugin{Online: true, URL: s.URL, PollInterval: time.Millisecond}
	r := requestFor(p, "breeze-tts-2", "你好", map[string]any{"voice_id": "voc_test"})
	var receipts []string
	result, err := p.Execute(context.Background(), r, func(v aigc.Result) { receipts = append(receipts, string(v.Metadata)) })
	if err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 || gets.Load() != 3 || len(result.Media) != 1 || len(receipts) != 5 || !strings.Contains(receipts[1], "job_test") || !strings.Contains(receipts[3], "streaming") {
		t.Fatal("async flow mismatch")
	}
	for _, receipt := range receipts {
		if strings.Contains(receipt, "private-test-key") {
			t.Fatal("secret in receipt")
		}
	}
}

func TestOnlineFailedSubmissionKeepsRecoveryHeader(t *testing.T) {
	var posts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("history-item-id", "hist_recover")
		w.WriteHeader(503)
		fmt.Fprint(w, `{"detail":"private-test-key"}`)
	}))
	defer s.Close()
	p := &Plugin{Online: true, URL: s.URL}
	r := requestFor(p, "breeze-tts-2", "你好", map[string]any{"voice_id": "voc_test"})
	var receipt string
	_, err := p.Execute(context.Background(), r, func(v aigc.Result) { receipt = string(v.Metadata) })
	if err == nil || posts.Load() != 1 || !strings.Contains(receipt, "hist_recover") || strings.Contains(err.Error(), "private-test-key") {
		t.Fatal("failed submission lost receipt, retried, or exposed body")
	}
}

func TestOnlineDesignSavesReusableVoice(t *testing.T) {
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/voice-previews/design":
			var body struct {
				Description string `json:"voice_description"`
				Text        string `json:"text"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Description != "温柔清晰的声音" || body.Text != "欢迎来到织序，让创意拥有声音。" {
				t.Error("Chinese preview parameters changed")
			}
			fmt.Fprintf(w, `{"previews":[{"generated_voice_id":"preview_test","audio_base_64":%q}]}`, base64.StdEncoding.EncodeToString(pcmWAV(make([]byte, 48000))))
		case "/v1/voice-previews/preview_test/save":
			var body struct {
				Name string `json:"voice_name"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name != "新音色" {
				t.Error("Chinese saved name changed")
			}
			fmt.Fprint(w, `{"voice_id":"voc_new","name":"新音色"}`)
		default:
			t.Error("unexpected request")
		}
	}))
	defer s.Close()
	p := &Plugin{Online: true, URL: s.URL}
	r := requestFor(p, "breeze-voice-design", "温柔清晰的声音", map[string]any{"voice_name": "新音色"})
	var receipt string
	result, err := p.Execute(context.Background(), r, func(v aigc.Result) { receipt = string(v.Metadata) })
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || result.Media[0].Voice.ID != "voc_new" || !strings.Contains(receipt, "preview_test") {
		t.Fatal("voice/save receipt mismatch")
	}
	if !strings.Contains(string(result.Metadata), `"billable_units":28`) {
		t.Fatal("voice design did not account for the preview script exactly once")
	}
}

func TestOnlineCloneUsesOnlyOriginalRequestMetering(t *testing.T) {
	for _, tc := range []struct {
		name string
		log  string
		want bool
	}{
		{"matching receipt", `{"request_id":"clone-request","endpoint":"/v1/voice-previews/clone","method":"POST","status":200,"billable_units":23}`, true},
		{"unrelated receipt", `{"request_id":"foreign-request","endpoint":"/v1/voice-previews/clone","method":"POST","status":200,"billable_units":23}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sample := pcmWAV(make([]byte, 4*48000))
			var writes, meteringReads atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					writes.Add(1)
				}
				switch r.URL.Path {
				case "/v1/voice-previews/clone":
					w.Header().Set("X-Request-ID", "clone-request")
					fmt.Fprint(w, `{"generated_voice_id":"preview_test","requires_verification":false}`)
				case "/v1/voice-previews/preview_test/stream":
					_, _ = w.Write(sample)
				case "/v1/voice-previews/preview_test/save":
					w.Header().Set("X-Request-ID", "save-request")
					fmt.Fprint(w, `{"voice_id":"voc_new","name":"新音色"}`)
				case "/v1/usage":
					meteringReads.Add(1)
					if r.Method != "GET" {
						t.Error("metering must be read-only")
					}
					fmt.Fprintf(w, `{"items":[%s]}`, tc.log)
				default:
					t.Errorf("unexpected %s", r.URL.Path)
				}
			}))
			defer s.Close()
			p := &Plugin{Online: true, URL: s.URL}
			r := requestFor(p, "breeze-voice-clone", "这是音色描述", map[string]any{"voice_name": "新音色"})
			r.Inputs = []aigc.Input{{Kind: "audio", Data: sample}}
			result, err := p.Execute(context.Background(), r, nil)
			if err != nil {
				t.Fatal(err)
			}
			if writes.Load() != 2 || meteringReads.Load() != 1 || result.Media[0].Voice.ID != "voc_new" {
				t.Fatal("original preview/save flow changed")
			}
			if got := strings.Contains(string(result.Metadata), `"billable_units":23`); got != tc.want {
				t.Fatalf("incorrect original metering: %s", result.Metadata)
			}
		})
	}
}

func TestInvalidUTF8StopsBeforeSubmission(t *testing.T) {
	p := &Plugin{Online: true}
	r := requestFor(p, "breeze-tts-2", "你好", map[string]any{"voice_id": "voc_test"})
	r.Payload = bytes.ReplaceAll(r.Payload, []byte("你好"), []byte{0xc4, 0xe3, 0xba, 0xc3})
	if err := p.Validate(context.Background(), r); err == nil {
		t.Fatal("GB2312 JSON accepted")
	}
	r = requestFor(p, "breeze-tts-2", "\uFFFD你好", map[string]any{"voice_id": "voc_test"})
	if err := p.Validate(context.Background(), r); err == nil {
		t.Fatal("replacement characters accepted")
	}
}

func TestVoiceCatalogPaginationSamplesAndWire(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.Header.Get("xi-api-key") != "private-test-key" {
			t.Error("catalog must be an authenticated read")
		}
		switch r.URL.Query().Get("voice_type") + "/" + r.URL.Query().Get("page") {
		case "default/1":
			fmt.Fprint(w, `{"voices":[{"voice_id":"voc_zh","name":"幽兰影","language_code":"zh","preview_url":"https://r.breeze.blue/example.mp3"}],"page":1,"total":2,"has_more":true,"next_page_token":"snapshot"}`)
		case "default/2":
			if r.URL.Query().Get("next_page_token") != "snapshot" {
				t.Error("snapshot token lost")
			}
			fmt.Fprint(w, `{"voices":[{"voice_id":"voc_en","name":"English","language_code":"en","preview_url":"https://example.invalid/untrusted.mp3"}],"page":2,"total":2,"has_more":false,"next_page_token":"snapshot"}`)
		case "personal/1":
			fmt.Fprint(w, `{"voices":[{"voice_id":"voc_mine","name":"我的音色","language_code":"zh","origin":"cloned"}],"page":1,"total":1,"has_more":false}`)
		default:
			t.Error("unexpected page")
		}
	}))
	defer s.Close()
	p := &Plugin{Online: true, URL: s.URL}
	input, _ := json.Marshal(aigc.WireCall{APIVersion: 2, Method: "voices", Request: aigc.Request{Secrets: map[string]string{"API_KEY": "private-test-key"}}})
	var output bytes.Buffer
	if err := aigc.Serve(context.Background(), bytes.NewReader(input), &output, p); err != nil {
		t.Fatal(err)
	}
	var reply aigc.WireReply
	if json.Unmarshal(output.Bytes(), &reply) != nil || reply.Error != "" {
		t.Fatal(output.String())
	}
	var catalog struct {
		Items []aigc.VoiceOption `json:"items"`
	}
	if json.Unmarshal(reply.Result.Metadata, &catalog) != nil {
		t.Fatal("invalid metadata")
	}
	if requests.Load() != 2 || len(catalog.Items) != 2 || catalog.Items[0].Label != "幽兰影" || catalog.Items[0].PreviewURL == "" || catalog.Items[1].Category != "复刻" {
		t.Fatal("catalog pagination, UTF-8, category or preview validation failed")
	}
	page, err := p.SearchVoices(context.Background(), map[string]string{"API_KEY": "private-test-key"}, aigc.VoiceQuery{Page: 2, PageSize: 20, PageToken: "snapshot"})
	if err != nil || page.Page != 2 || len(page.Items) != 1 || page.Items[0].PreviewURL != "" {
		t.Fatal("paged search or preview validation failed", err)
	}
}

func TestReferenceAndResponseWAVValidation(t *testing.T) {
	valid := pcmWAV(make([]byte, 48000))
	for _, tc := range []struct {
		data []byte
		ok   bool
	}{
		{valid, true}, {[]byte("RIFF"), false}, {valid[:len(valid)-2], false}, {pcmWAV([]byte{0, 0}), false},
	} {
		if err := validateWAV(tc.data, true); (err == nil) != tc.ok {
			t.Fatalf("WAV validation %v want %v", err, tc.ok)
		}
	}
}

func TestRedirectNeverResubmitsOrForwardsKey(t *testing.T) {
	var destination atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { destination.Add(1) }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 307) }))
	defer s.Close()
	p := &Plugin{Online: true, URL: s.URL}
	r := requestFor(p, "breeze-tts-2", "你好", map[string]any{"voice_id": "voc_test"})
	if _, err := p.Execute(context.Background(), r, nil); err == nil || destination.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
