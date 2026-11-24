package dashscope

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func sampleWAV() []byte {
	b := make([]byte, 44+24000*2*5)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 24000)
	binary.LittleEndian.PutUint32(b[28:], 48000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(b)-44))
	return b
}
func TestDesignedVoiceIDAndPreviewCanFollowIndependentPaths(t *testing.T) {
	for _, model := range []string{"voice-enrollment"} {
		t.Run(model, func(t *testing.T) {
			p := New()
			calls := 0
			preview := sampleWAV()
			target := "qwen-audio-3.1-tts-flash"
			field := "voice_id"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var b struct {
					Model      string         `json:"model"`
					Input      map[string]any `json:"input"`
					Parameters map[string]any `json:"parameters"`
				}
				_ = json.NewDecoder(r.Body).Decode(&b)
				if b.Input["action"] == "query_voice" {
					if b.Input["voice_id"] != "designed-fixture-voice" {
						t.Error("unexpected queried voice")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"status": "OK", "target_model": target}})
					return
				}
				calls++
				switch calls {
				case 1:
					if r.URL.Path != customizationPath || b.Model != model || b.Input["target_model"] != target || b.Input["voice_prompt"] != "处理图1" || b.Parameters["response_format"] != "wav" {
						t.Error("design mapping", b)
					}
					if model == "voice-enrollment" && b.Input["action"] != "create_voice" || model == "qwen-voice-design" && b.Input["action"] != "create" {
						t.Error("wrong action")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{field: "designed-fixture-voice", "target_model": target, "preview_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(preview), "response_format": "wav"}}, "usage": map[string]any{"count": 1}})
				case 2:
					if b.Model != target || b.Input["voice"] != "designed-fixture-voice" {
						t.Error("voice binding ignored", b)
					}
					if target == "qwen-audio-3.1-tts-flash" && (r.URL.Path != "/services/audio/tts/SpeechSynthesizer" || b.Input["language_type"] != nil || b.Input["format"] != "wav") {
						t.Error("wrong audio API", b)
					}
					_, _ = w.Write([]byte(`{"output":{"audio":{"url":"https://example.com/tts.wav"}}}`))
				default:
					t.Error("unexpected additional request")
				}
			}))
			defer srv.Close()
			local(p, srv)
			checkpoints := 0
			design, err := p.Execute(context.Background(), fixture(t, p, model, nil, 0), func(r aigc.Result) {
				checkpoints++
				if !bytes.Contains(r.Metadata, []byte("designed-fixture-voice")) {
					t.Error("voice checkpoint missing")
				}
			})
			if err != nil || len(design.Media) != 1 || !bytes.Equal(design.Media[0].Data, preview) || design.Media[0].Voice == nil || checkpoints != 1 {
				t.Fatal("preview or identity missing", err)
			}
			voice := design.Media[0].Voice
			if len(voice.CompatibleModels) != 1 || voice.CompatibleModels[0] != target {
				t.Fatal("voice model binding")
			}
			tts := fixture(t, p, target, nil, 0)
			tts.Voice = voice
			if _, err = p.Execute(context.Background(), tts, nil); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatal("unexpected paid submission", calls)
			}
			// ID reuse and sample reuse have independent compatibility checks.
			tts.Voice = &aigc.Voice{Provider: "another-provider", ID: voice.ID, CompatibleModels: []string{target}}
			if p.Validate(context.Background(), tts) == nil {
				t.Fatal("foreign voice ID accepted")
			}
			tts.Voice = &aigc.Voice{Provider: "dashscope", ID: "legacy-clone", Kind: "clone", CompatibleModels: []string{"qwen3-tts-vc-2026-01-22"}}
			if p.Validate(context.Background(), tts) == nil {
				t.Fatal("wrong target accepted")
			}
		})
	}
}
func TestVoiceValidationAndCreationFailures(t *testing.T) {
	if !voiceID.MatchString("qwen-audio-3.1-tts-flash-vd-canvas-fixture") {
		t.Fatal("real model-derived voice IDs contain periods")
	}
	p := New()
	for _, tc := range []struct {
		model  string
		params map[string]any
		count  int
	}{
		{"voice-enrollment", map[string]any{"preview_text": "太短"}, 0},
		{"voice-enrollment", map[string]any{"prefix": "bad-name"}, 0},
		{"qwen-voice-design", map[string]any{"preferred_name": "非法前缀"}, 0},
		{"qwen-voice-enrollment", nil, 0},
		{"qwen3-tts-vd-2026-01-26", nil, 0},
	} {
		if p.Validate(context.Background(), fixture(t, p, tc.model, tc.params, tc.count)) == nil {
			t.Fatal("invalid voice request accepted", tc)
		}
	}
	calls, checkpoints := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"output":{"voice_id":"retained-voice","preview_audio":{"data":"invalid-base64"}}}`))
	}))
	defer srv.Close()
	local(p, srv)
	_, err := p.Execute(context.Background(), fixture(t, p, "voice-enrollment", nil, 0), func(r aigc.Result) {
		checkpoints++
		if !bytes.Contains(r.Metadata, []byte("retained-voice")) {
			t.Error("lost created ID")
		}
	})
	if err == nil || calls != 1 || checkpoints != 1 {
		t.Fatal("creation retried or identity lost", err, calls, checkpoints)
	}
}

func TestPCMPreviewRequiresReportedRate(t *testing.T) {
	pcm := make([]byte, 48000)
	b, err := pcmPreviewWAV(pcm, 24000)
	if err != nil || string(b[:4]) != "RIFF" || binary.LittleEndian.Uint32(b[24:]) != 24000 || len(b) != 48044 {
		t.Fatal("invalid WAV", err)
	}
	for _, rate := range []int{0, 7999, 48001} {
		if _, err := pcmPreviewWAV(pcm, rate); err == nil {
			t.Fatal("guessed rate", rate)
		}
	}
}
