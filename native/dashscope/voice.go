package dashscope

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/internal/voices"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const customizationPath = "/services/audio/tts/customization"

var voiceName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
var voicePrefix = regexp.MustCompile(`^[A-Za-z0-9]{1,10}$`)
var voiceID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,256}$`)

func addVoiceModels(m *aigc.Manifest) {
	m.Nodes = append(m.Nodes, aigc.Node{ID: "voice-design", Label: "百炼音色设计", Capability: "voice.design", Model: "voice-enrollment"})
	// The returned voice is bound to exactly one target; its sample is ordinary audio.
	for _, id := range []string{"qwen3-tts-vd-2026-01-26", "qwen3-tts-vc-2026-01-22", "qwen-audio-3.1-tts-flash", "qwen-audio-3.0-tts-plus"} {
		fields := map[string]aigc.Field{"voice": {Type: "string", Label: "音色 ID（或连接音色输出）", Control: "voice", MaxLength: 256}, "language_type": enum("语言", "Auto", "Auto", "Chinese", "English", "German", "Italian", "Portuguese", "Spanish", "Japanese", "Korean", "French", "Russian")}
		if strings.HasPrefix(id, "qwen-audio-") {
			delete(fields, "language_type")
			fields["instruction"] = aigc.Field{Type: "string", Label: "语气 / 方言指令", Control: "textarea", MaxLength: 500}
		}
		model := aigc.Model{ID: id, Label: id, Capability: "speech.generate", Hidden: strings.HasPrefix(id, "qwen3-tts-v"), InputKinds: []string{}, Prompt: &aigc.PromptSpec{Label: "合成文本", Required: true, MaxLength: 600}, Fields: fields}
		if strings.HasPrefix(id, "qwen-audio-") {
			model.Label = "Qwen-Audio-3.1-TTS-Flash"
			if id == "qwen-audio-3.0-tts-plus" {
				model.Label = "Qwen-Audio-3.0-TTS-Plus"
			}
			model.InputKinds = []string{"audio"}
			model.MaxInputs = 1
			model.InputRoles = map[string]aigc.InputRole{"speaker": {Kind: "audio", Label: "音色参考（自动复刻）", Description: "建议 10–20 秒清晰人声，至少 5 秒连续朗读；最多 60 秒、10MB。"}}
			f := fields["voice"]
			f.Default = "longanhuan_v3.1"
			if id == "qwen-audio-3.0-tts-plus" {
				f.Default = "longanhuan_v3.6"
			}
			fields["voice"] = f
		}
		m.Models = append(m.Models, model)
	}
	for _, id := range []string{"voice-enrollment", "voice-enrollment-clone", "qwen-voice-design", "qwen-voice-enrollment"} {
		fields := map[string]aigc.Field{}
		prompt := &aigc.PromptSpec{Label: "音色描述", Required: true, MaxLength: 2048}
		label := "Qwen 音色设计"
		kinds := []string{}
		maxInputs := 0
		if id == "voice-enrollment" || id == "voice-enrollment-clone" {
			label = "Qwen-Audio 音色设计"
			prompt.MaxLength = 500
			fields["target_model"] = enum("驱动 TTS 模型", "qwen-audio-3.1-tts-flash", "qwen-audio-3.1-tts-flash", "qwen-audio-3.0-tts-plus")
			fields["prefix"] = aigc.Field{Type: "string", Label: "音色名称前缀", Default: "canvas", Required: true, MaxLength: 10}
			fields["language"] = enum("试听语言", "zh", "zh", "en")
			fields["preview_text"] = aigc.Field{Type: "string", Label: "试听文本（15至200字符）", Control: "textarea", Required: true, Default: "你好，欢迎收听这段声音试听，希望你喜欢我的声音。", MaxLength: 200}
			if id == "voice-enrollment-clone" {
				label = "Qwen-Audio 音色复刻"
				prompt = &aigc.PromptSpec{Label: "备注（不参与复刻）", Required: false}
				delete(fields, "preview_text")
				delete(fields, "language")
				kinds, maxInputs = []string{"audio"}, 1
			}
		} else {
			fields["preferred_name"] = aigc.Field{Type: "string", Label: "音色名称前缀", Default: "canvas", Required: true, MaxLength: 16}
			if id == "qwen-voice-design" {
				fields["target_model"] = enum("驱动 TTS 模型", "qwen3-tts-vd-2026-01-26", "qwen3-tts-vd-2026-01-26")
				fields["language"] = enum("试听语言", "zh", "zh", "en", "de", "it", "pt", "es", "ja", "ko", "fr", "ru")
				fields["preview_text"] = aigc.Field{Type: "string", Label: "试听文本", Control: "textarea", Required: true, Default: "你好，欢迎收听这段声音试听，希望你喜欢我的声音。", MaxLength: 1024}
			} else {
				label = "Qwen 音色复刻（音频输入）"
				prompt = &aigc.PromptSpec{Label: "备注（不参与复刻）", Required: false, MaxLength: 600}
				fields["target_model"] = enum("驱动 TTS 模型", "qwen3-tts-vc-2026-01-22", "qwen3-tts-vc-2026-01-22")
				kinds, maxInputs = []string{"audio"}, 1
			}
		}
		m.Models = append(m.Models, aigc.Model{ID: id, Label: label, Capability: "voice.design", Hidden: id != "voice-enrollment", InputKinds: kinds, MaxInputs: maxInputs, Prompt: prompt, Fields: fields})
	}
}
func text(v any) string { s, _ := v.(string); return s }
func validateVoice(g aigc.Generation, params map[string]any, r aigc.Request) error {
	if g.Model == "qwen3-tts-flash" {
		id := text(params["voice"])
		if r.Voice != nil {
			id = r.Voice.ID
		}
		found := false
		for _, v := range voices.List("dashscope", g.Model) {
			found = found || v.ID == id
		}
		if !found {
			return errors.New("Qwen3 Flash 仅支持官方系统音色，请从官方音色库选择")
		}
	}
	if g.Capability == "speech.generate" {
		validID := r.Voice == nil || voiceID.MatchString(r.Voice.ID)
		if r.Voice != nil && r.Voice.Kind == "official" {
			validID = false
			for _, v := range voices.List("dashscope", g.Model) {
				validID = validID || v.ID == r.Voice.ID
			}
		}
		compatible := false
		for _, m := range catalog().Models {
			if m.ID == g.Model {
				compatible = aigc.AcceptsVoice(m, r.Voice)
			}
		}
		if r.Voice != nil && r.Voice.Kind != "official" && len(r.Voice.CompatibleModels) != 1 {
			compatible = false
		}
		if r.Voice != nil && (r.Voice.Provider != "dashscope" || !compatible || !validID) {
			return errors.New("音色与百炼目标 TTS 模型不兼容")
		}
		if r.Voice != nil && len(r.Inputs) > 0 {
			return errors.New("音色输入和音频复刻输入不能同时使用")
		}
		if r.Voice == nil && len(r.Inputs) == 0 && strings.TrimSpace(text(params["voice"])) == "" {
			return errors.New("请输入音色 ID 或连接兼容的音色输出")
		}
	}
	if g.Capability != "voice.design" {
		return nil
	}
	if g.Model == "voice-enrollment" || g.Model == "voice-enrollment-clone" {
		if !voicePrefix.MatchString(text(params["prefix"])) {
			return errors.New("音色前缀仅允许1至10位英文字母和数字")
		}
		if g.Model == "voice-enrollment" && utf8.RuneCountInString(strings.TrimSpace(text(params["preview_text"]))) < 15 {
			return errors.New("试听文本至少15个字符")
		}
	} else if !voiceName.MatchString(text(params["preferred_name"])) {
		return errors.New("音色前缀仅允许1至16位英文字母、数字和下划线")
	}
	if (g.Model == "qwen-voice-enrollment" || g.Model == "voice-enrollment-clone") && len(r.Inputs) != 1 {
		return errors.New("声音复刻需要连接1段音频（可使用设计节点的试听音频）")
	}
	return nil
}
func (p *Plugin) createVoice(ctx context.Context, base string, g aigc.Generation, params map[string]any, r aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	input := map[string]any{"action": "create", "target_model": params["target_model"]}
	parameters := map[string]any{}
	var sample []byte
	wireModel := g.Model
	switch g.Model {
	case "voice-enrollment":
		input["action"] = "create_voice"
		input["prefix"] = params["prefix"]
		input["language_hints"] = []string{text(params["language"])}
		input["voice_prompt"], input["preview_text"] = g.Prompt, params["preview_text"]
		parameters = map[string]any{"sample_rate": 24000, "response_format": "wav"}
	case "qwen-voice-design":
		input["preferred_name"] = params["preferred_name"]
		input["language"] = params["language"]
		input["voice_prompt"], input["preview_text"] = g.Prompt, params["preview_text"]
		parameters = map[string]any{"sample_rate": 24000, "response_format": "wav"}
	case "qwen-voice-enrollment":
		input["preferred_name"] = params["preferred_name"]
		sample = r.Inputs[0].Data
		mime := http.DetectContentType(sample)
		if len(sample) >= 12 && string(sample[4:8]) == "ftyp" && (string(sample[8:12]) == "M4A " || string(sample[8:12]) == "M4B ") {
			mime = "audio/mp4"
		}
		if mime == "audio/wave" || mime == "audio/x-wav" {
			mime = "audio/wav"
		}
		if len(sample) == 0 || len(sample) > 10<<20 || (mime != "audio/wav" && mime != "audio/mpeg" && mime != "audio/mp4") {
			return aigc.Result{}, errors.New("复刻样本需要10MB以内的完整WAV、MP3或M4A音频")
		}
		input["audio"] = map[string]any{"data": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(sample)}
	case "voice-enrollment-clone":
		wireModel = "voice-enrollment"
		sample = r.Inputs[0].Data
		if _, _, err := audioSample(sample); err != nil {
			return aigc.Result{}, err
		}
		fileURL, err := p.uploadAudio(ctx, base, r.Secrets["API_KEY"], wireModel, sample)
		if err != nil {
			return aigc.Result{}, err
		}
		input = map[string]any{"action": "create_voice", "target_model": params["target_model"], "prefix": params["prefix"], "url": fileURL}
	default:
		return aigc.Result{}, errors.New("unknown voice operation")
	}
	out, err := p.call(ctx, http.MethodPost, base, customizationPath, r.Secrets["API_KEY"], map[string]any{"model": wireModel, "input": input, "parameters": parameters}, false)
	if err != nil {
		return aigc.Result{}, err
	}
	id := out.Output.VoiceID
	if id == "" {
		id = out.Output.Voice
	}
	if !voiceID.MatchString(id) {
		return aigc.Result{}, &Failure{}
	}
	target := text(params["target_model"])
	voice := &aigc.Voice{Provider: "dashscope", ID: id, Label: g.Model, CompatibleModels: []string{target}}
	voice.Kind = "design"
	if g.Model == "qwen-voice-enrollment" || g.Model == "voice-enrollment-clone" {
		voice.Kind = "clone"
	}
	var meta map[string]any
	_ = json.Unmarshal(metadata(out, g.Model, customizationPath, ""), &meta)
	meta["voiceId"], meta["targetModel"] = id, target
	b, _ := json.Marshal(meta)
	if checkpoint != nil {
		checkpoint(aigc.Result{Metadata: b})
	} // Retain identity even if preview decoding fails.
	if out.Output.TargetModel != "" && out.Output.TargetModel != target {
		return aigc.Result{}, &Failure{}
	}
	if g.Model != "qwen-voice-enrollment" && g.Model != "voice-enrollment-clone" {
		sample, err = base64.StdEncoding.DecodeString(out.Output.PreviewAudio.Data)
		if err != nil || len(sample) == 0 {
			return aigc.Result{}, &Failure{}
		}
		if out.Output.PreviewAudio.Format == "pcm" {
			sample, err = pcmPreviewWAV(sample, out.Output.PreviewAudio.SampleRate)
			if err != nil {
				return aigc.Result{}, err
			}
		}
	}
	return aigc.Result{Media: []aigc.Media{{Data: sample, Voice: voice}}, Metadata: b}, nil
}

// Only wrap explicitly reported PCM with a reported rate; never infer a rate
// from a requested parameter when the provider did not echo one.
func pcmPreviewWAV(pcm []byte, rate int) ([]byte, error) {
	if rate < 8000 || rate > 48000 || len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > 10<<20 {
		return nil, errors.New("试听 PCM 采样信息不完整，音色 ID 已保留")
	}
	b := make([]byte, 44+len(pcm))
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(len(pcm)))
	copy(b[44:], pcm)
	return b, nil
}
