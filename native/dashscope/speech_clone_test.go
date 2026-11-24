package dashscope

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestSpeechAutoCloneOverridesManualAndSurvivesRestart(t *testing.T) {
	clones, synthesis := 0, 0
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"output":{"audio":{"url":"https://example.com/result.wav"}}}`
		respond := func() (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}
		if r.Method == "GET" && r.URL.Path == "/api/v1/uploads" {
			body = `{"data":{"upload_host":"https://fixture.oss-cn-beijing.aliyuncs.com","upload_dir":"private/fixture","oss_access_key_id":"temporary-oss-key","policy":"policy","signature":"signature"}}`
			return respond()
		}
		if r.URL.Host == "fixture.oss-cn-beijing.aliyuncs.com" {
			return respond()
		}
		var b struct {
			Model string
			Input map[string]any
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if strings.HasSuffix(r.URL.Path, customizationPath) {
			if b.Input["action"] == "query_voice" {
				body = `{"output":{"status":"OK","target_model":"qwen-audio-3.1-tts-flash"}}`
				return respond()
			}
			clones++
			if b.Model != "voice-enrollment" || b.Input["target_model"] != "qwen-audio-3.1-tts-flash" || b.Input["url"] == nil {
				t.Error("clone contract", b)
			}
			body = `{"output":{"voice_id":"qwen-cloned-test"}}`
			return respond()
		}
		synthesis++
		if b.Model != "qwen-audio-3.1-tts-flash" || b.Input["voice"] != "qwen-cloned-test" {
			t.Error("input did not override manual", b)
		}
		return respond()
	})
	dir := t.TempDir()
	for range 2 {
		p := New()
		p.Client.Transport = transport
		p.VoiceCacheDir = dir
		r := fixture(t, p, "qwen-audio-3.1-tts-flash", map[string]any{"voice": "MANUAL-MUST-NOT-BE-SENT"}, 0)
		r.Inputs = []aigc.Input{{Kind: "audio", Role: "speaker", Data: sampleWAV()}}
		out, err := p.Execute(context.Background(), r, nil)
		if err != nil || out.Media[0].Voice == nil || out.Media[0].Voice.ID != "qwen-cloned-test" {
			t.Fatal(out, err)
		}
	}
	if clones != 1 || synthesis != 2 {
		t.Fatal("duplicate voice creation", clones, synthesis)
	}
}

func TestSpeechDesignUsesBoundRouteAndRejectsMixedSources(t *testing.T) {
	p := New()
	r := fixture(t, p, "qwen-audio-3.1-tts-flash", nil, 0)
	r.Voice = &aigc.Voice{Provider: "dashscope", ID: "design-test", Kind: "design", CompatibleModels: []string{"qwen-audio-3.1-tts-flash"}}
	if err := p.Validate(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	r.Inputs = []aigc.Input{{Kind: "audio", Data: sampleWAV()}}
	if p.Validate(context.Background(), r) == nil {
		t.Fatal("mixed voice inputs")
	}
	r.Inputs = nil
	r.Voice.CompatibleModels = []string{"qwen-audio-3.0-tts-plus"}
	if p.Validate(context.Background(), r) == nil {
		t.Fatal("cross-family voice")
	}
}

func TestQwen3CustomPathsStopBeforeAnyProviderCallAndPreserveCache(t *testing.T) {
	p := New()
	p.VoiceCacheDir = t.TempDir()
	path := filepath.Join(p.VoiceCacheDir, "existing.json")
	existing := []byte(`{"id":"existing-qwen3-voice","compatibleModels":["qwen3-tts-vc-2026-01-22"]}`)
	if err := os.WriteFile(path, existing, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	p.Client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		t.Error("disabled path contacted provider")
		return nil, nil
	})
	for _, model := range []string{"qwen-voice-design", "qwen-voice-enrollment", "qwen3-tts-vd-2026-01-26", "qwen3-tts-vc-2026-01-22"} {
		if _, err := p.Execute(context.Background(), fixture(t, p, model, nil, 0), nil); err == nil || !strings.Contains(err.Error(), "已禁用") {
			t.Fatal(model, err)
		}
	}
	for _, kind := range []string{"audio", "voice", "manual"} {
		r := fixture(t, p, "qwen3-tts-flash", nil, 0)
		switch kind {
		case "audio":
			r.Inputs = []aigc.Input{{Kind: "audio", Data: sampleWAV()}}
		case "voice":
			r.Voice = &aigc.Voice{Provider: "dashscope", ID: "existing-qwen3-voice", Kind: "clone", CompatibleModels: []string{"qwen3-tts-vc-2026-01-22"}}
		case "manual":
			r = fixture(t, p, "qwen3-tts-flash", map[string]any{"voice": "existing-qwen3-voice"}, 0)
		}
		if _, err := p.Execute(context.Background(), r, nil); err == nil {
			t.Fatal("custom input accepted", kind)
		}
		if kind == "audio" {
			if _, err := p.cloneForSpeech(context.Background(), "", aigc.Generation{Model: "qwen3-tts-flash"}, r, nil); err == nil {
				t.Fatal("internal clone bypass")
			}
		}
	}
	files, _ := os.ReadDir(p.VoiceCacheDir)
	after, _ := os.ReadFile(path)
	if calls != 0 || len(files) != 1 || string(after) != string(existing) {
		t.Fatal("provider called or cache changed", calls)
	}
}

func TestSpeechCatalogDefaultsAndLegacyDefinitions(t *testing.T) {
	m := catalog()
	for _, n := range m.Nodes {
		if n.ID == "speech" && n.Model != "qwen-audio-3.1-tts-flash" {
			t.Fatal("unsafe default", n)
		}
	}
	for _, model := range m.Models {
		if model.ID == "qwen3-tts-flash" && (len(model.InputKinds) != 0 || len(model.VoiceModels) != 0 || model.MaxInputs != 0) {
			t.Fatal("Qwen3 custom input advertised")
		}
		if (strings.HasPrefix(model.ID, "qwen-voice-") || strings.HasPrefix(model.ID, "qwen3-tts-v")) && !model.Hidden {
			t.Fatal("legacy entry visible", model.ID)
		}
	}
}
