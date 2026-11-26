package breezetts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func onlineVoice(r aigc.Request, params map[string]any) string {
	if r.Voice != nil {
		return r.Voice.ID
	}
	if len(r.Voices) == 1 {
		return r.Voices[0].ID
	}
	return stringValue(params["voice_id"])
}

func (p *Plugin) validateOnline(r aigc.Request, g aigc.Generation, params map[string]any) error {
	if g.Capability == "speech.generate" {
		if !identifier.MatchString(onlineVoice(r, params)) {
			return fmt.Errorf("请填写在线音色 ID 或连接 BreezeBlue 音色")
		}
		for _, voice := range append(append([]aigc.Voice{}, r.Voices...), voiceSlice(r.Voice)...) {
			if voice.Provider != p.Manifest().ID {
				return fmt.Errorf("Breeze 在线音色不能来自其他渠道")
			}
		}
		if len(r.Voices) > 1 {
			return fmt.Errorf("Breeze 每次只使用一个在线音色")
		}
		if r.Voice != nil && len(r.Voices) > 0 && r.Voice.ID != r.Voices[0].ID {
			return fmt.Errorf("Breeze 在线音色引用冲突")
		}
		return nil
	}
	if strings.TrimSpace(stringValue(params["voice_name"])) == "" {
		return fmt.Errorf("请填写保存音色名称")
	}
	if r.Voice != nil || len(r.Voices) > 0 {
		return fmt.Errorf("创建 Breeze 音色不接受已有音色引用")
	}
	if g.Model == "breeze-voice-design" && (utf8.RuneCountInString(g.Prompt) < 3 || utf8.RuneCountInString(stringValue(params["preview_text"])) < 3) {
		return fmt.Errorf("音色描述和试听文本至少需要 3 个字符")
	}
	for _, in := range r.Inputs {
		if in.URL != "" {
			return fmt.Errorf("Breeze 仅接受宿主读取的参考音频")
		}
	}
	return nil
}

func voiceSlice(v *aigc.Voice) []aigc.Voice {
	if v == nil {
		return nil
	}
	return []aigc.Voice{*v}
}

func (p *Plugin) onlineJSON(ctx context.Context, path, key, contentType string, body []byte, meta map[string]any, fn func(aigc.Result), out any) error {
	resp, err := p.request(ctx, "POST", path, key, contentType, body)
	if err != nil {
		return err
	}
	if path == "/v1/voice-previews/clone" {
		if id := resp.Header.Get("X-Request-ID"); identifier.MatchString(id) {
			meta["previewRequestId"] = id
			checkpoint(fn, meta)
		}
	}
	for header, field := range map[string]string{"history-item-id": "historyItemId", "generated-voice-id": "generatedVoiceId"} {
		if value := resp.Header.Get(header); identifier.MatchString(value) {
			meta[field] = value
			checkpoint(fn, meta)
		}
	}
	data, err := readResponse(resp, 48<<20)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, out) != nil {
		return fmt.Errorf("Breeze 在线响应无效；请保留回执，不要重新提交")
	}
	return nil
}

func (p *Plugin) executeOnline(ctx context.Context, r aigc.Request, g aigc.Generation, params map[string]any, fn func(aigc.Result)) (aigc.Result, error) {
	key := strings.TrimSpace(r.Secrets["API_KEY"])
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return aigc.Result{}, fmt.Errorf("请配置 BREEZE_TTS_API_KEY 的服务器环境变量引用")
	}
	meta := map[string]any{"hostJobId": r.JobID, "channel": "api", "model": g.Model, "state": "submitting"}
	checkpoint(fn, meta)
	if g.Capability == "speech.generate" {
		return p.onlineSpeech(ctx, r, g, params, key, meta, fn)
	}
	return p.onlineCreateVoice(ctx, r, g, params, key, meta, fn)
}

// WeightedBillableUnits is the Developer API's CJK-weighted text count.
func WeightedBillableUnits(text string) int64 {
	var units int64
	for _, r := range text {
		units++
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			units++
		}
	}
	return units
}
func (p *Plugin) onlineSpeech(ctx context.Context, r aigc.Request, g aigc.Generation, params map[string]any, key string, meta map[string]any, fn func(aigc.Result)) (aigc.Result, error) {
	voiceID := onlineVoice(r, params)
	meta["usage"] = map[string]any{"billable_units": WeightedBillableUnits(g.Prompt)}
	body, _ := json.Marshal(map[string]any{"text": g.Prompt, "model_id": "breeze-tts-2", "language_code": params["language_code"], "instructions": params["instructions"], "voice_settings": map[string]any{"guidance_scale": params["guidance_scale"], "speed": params["speed"], "volume": params["volume"], "use_speaker_boost": params["use_speaker_boost"]}})
	var job struct {
		ID      string `json:"generation_job_id"`
		History string `json:"history_item_id"`
		Status  string `json:"status"`
		State   string `json:"state"`
	}
	if err := p.onlineJSON(ctx, "/v1/text-to-speech/"+voiceID+"?output_format=wav_24000&delivery=async", key, "application/json", body, meta, fn, &job); err != nil {
		return aigc.Result{}, err
	}
	if !identifier.MatchString(job.ID) || !identifier.MatchString(job.History) {
		return aigc.Result{}, fmt.Errorf("Breeze 在线任务票据无效；不会重新提交")
	}
	meta["generationJobId"] = job.ID
	meta["historyItemId"] = job.History
	meta["voiceId"] = voiceID
	meta["state"] = job.Status
	checkpoint(fn, meta)
	id := job.ID
	interval := p.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		if err := p.jsonRequest(ctx, "GET", "/v1/generation-jobs/"+id, key, nil, &job); err != nil {
			return aigc.Result{}, err
		}
		if job.ID != id || job.History != meta["historyItemId"] {
			return aigc.Result{}, fmt.Errorf("Breeze 在线任务票据不匹配")
		}
		if meta["state"] != job.State {
			meta["state"] = job.State
			checkpoint(fn, meta)
		}
		switch job.State {
		case "ready", "succeeded", "completed":
			resp, err := p.request(ctx, "GET", "/v1/generation-jobs/"+id+"/audio", key, "", nil)
			if err != nil {
				return aigc.Result{}, err
			}
			audio, err := readResponse(resp, maxAudioBytes)
			if err != nil {
				return aigc.Result{}, err
			}
			if err = validateWAV(audio, false); err != nil {
				return aigc.Result{}, err
			}
			b, _ := json.Marshal(meta)
			return aigc.Result{Media: []aigc.Media{{Data: audio}}, Metadata: b}, nil
		case "failed", "error", "cancelled", "canceled", "interrupted":
			checkpoint(fn, meta)
			return aigc.Result{}, fmt.Errorf("Breeze 在线任务失败或中断；票据已保留，不会重新生成")
		case "admitted", "queued", "pending", "running", "processing", "streaming":
		default:
			return aigc.Result{}, fmt.Errorf("Breeze 返回未知任务状态；票据已保留")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return aigc.Result{}, fmt.Errorf("Breeze 等待结束；远端任务可能仍在执行，请用票据查询")
		case <-timer.C:
		}
	}
}

func (p *Plugin) onlineCreateVoice(ctx context.Context, r aigc.Request, g aigc.Generation, params map[string]any, key string, meta map[string]any, fn func(aigc.Result)) (aigc.Result, error) {
	var previewID string
	var audio []byte
	kind := "design"
	if g.Model == "breeze-voice-design" {
		body, _ := json.Marshal(map[string]any{"voice_description": g.Prompt, "text": params["preview_text"], "model_id": "breeze-tts-2", "language_code": params["language_code"], "guidance_scale": params["guidance_scale"], "preview_count": 1})
		var result struct {
			Previews []struct {
				ID    string `json:"generated_voice_id"`
				Audio string `json:"audio_base_64"`
			} `json:"previews"`
		}
		if err := p.onlineJSON(ctx, "/v1/voice-previews/design?output_format=wav_24000", key, "application/json", body, meta, fn, &result); err != nil {
			return aigc.Result{}, err
		}
		if len(result.Previews) != 1 || !identifier.MatchString(result.Previews[0].ID) {
			return aigc.Result{}, fmt.Errorf("Breeze 音色试听票据无效")
		}
		previewID = result.Previews[0].ID
		meta["generatedVoiceId"] = previewID
		text, _ := params["preview_text"].(string)
		meta["usage"] = map[string]any{"billable_units": WeightedBillableUnits(text)}
		meta["state"] = "preview_ready"
		checkpoint(fn, meta)
		var err error
		audio, err = base64.StdEncoding.DecodeString(result.Previews[0].Audio)
		if err != nil {
			return aigc.Result{}, fmt.Errorf("Breeze 试听音频编码无效")
		}
	} else {
		kind = "clone"
		if len(r.Inputs) != 1 {
			return aigc.Result{}, fmt.Errorf("Breeze 克隆需要一段参考音频")
		}
		in := r.Inputs[0]
		if err := validateWAV(in.Data, true); err != nil {
			return aigc.Result{}, err
		}
		if len(in.Data) > 5_000_000 {
			return aigc.Result{}, fmt.Errorf("Breeze 在线参考音频不能超过 5 MB")
		}
		// validateWAV allows ancillary chunks, so compute duration from the data chunk.
		if wavDuration(in.Data) < 3 {
			return aigc.Result{}, fmt.Errorf("Breeze 在线克隆至少需要 3 秒参考音频")
		}
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		_ = w.WriteField("name", stringValue(params["voice_name"]))
		_ = w.WriteField("description", g.Prompt)
		_ = w.WriteField("remove_background_noise", fmt.Sprint(params["remove_background_noise"]))
		if params["language_code"] == "zh" {
			_ = w.WriteField("reference_language_code", "zh")
		}
		part, _ := w.CreateFormFile("files", "reference.wav")
		_, _ = part.Write(in.Data)
		_ = w.Close()
		var result struct {
			ID           string `json:"generated_voice_id"`
			Verification bool   `json:"requires_verification"`
		}
		if err := p.onlineJSON(ctx, "/v1/voice-previews/clone", key, w.FormDataContentType(), body.Bytes(), meta, fn, &result); err != nil {
			return aigc.Result{}, err
		}
		if !identifier.MatchString(result.ID) {
			return aigc.Result{}, fmt.Errorf("Breeze 克隆试听票据无效")
		}
		previewID = result.ID
		meta["generatedVoiceId"] = previewID
		meta["state"] = "preview_ready"
		checkpoint(fn, meta)
		if result.Verification {
			return aigc.Result{}, fmt.Errorf("Breeze 要求完成音色验证；请在官方平台处理已保留的试听票据")
		}
		resp, err := p.request(ctx, "GET", "/v1/voice-previews/"+previewID+"/stream?output_format=wav_24000", key, "", nil)
		if err != nil {
			return aigc.Result{}, err
		}
		audio, err = readResponse(resp, maxAudioBytes)
		if err != nil {
			return aigc.Result{}, err
		}
	}
	if err := validateWAV(audio, false); err != nil {
		return aigc.Result{}, err
	}
	body, _ := json.Marshal(map[string]any{"voice_name": params["voice_name"], "voice_description": g.Prompt, "language_code": params["language_code"]})
	var saved struct {
		ID   string `json:"voice_id"`
		Name string `json:"name"`
	}
	if err := p.onlineJSON(ctx, "/v1/voice-previews/"+previewID+"/save", key, "application/json", body, meta, fn, &saved); err != nil {
		return aigc.Result{}, err
	}
	if !identifier.MatchString(saved.ID) {
		return aigc.Result{}, fmt.Errorf("Breeze 保存音色票据无效；请检查已保留的试听")
	}
	meta["voiceId"] = saved.ID
	meta["state"] = "saved"
	if id, ok := meta["previewRequestId"].(string); kind == "clone" && ok {
		if units := p.cloneBillableUnits(ctx, key, id); units != nil {
			meta["usage"] = map[string]any{"billable_units": *units}
		}
	}
	b, _ := json.Marshal(meta)
	return aigc.Result{Media: []aigc.Media{{Data: audio, Voice: &aigc.Voice{Kind: kind, Provider: p.Manifest().ID, ID: saved.ID, Label: saved.Name, CompatibleModels: []string{"breeze-tts-2"}}}}, Metadata: b}, nil
}

func (p *Plugin) cloneBillableUnits(ctx context.Context, key, requestID string) *int64 {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var metering struct {
		Items []struct {
			RequestID     string `json:"request_id"`
			Endpoint      string `json:"endpoint"`
			Method        string `json:"method"`
			Status        int    `json:"status"`
			BillableUnits *int64 `json:"billable_units"`
		} `json:"items"`
	}
	if err := p.jsonRequest(ctx, "GET", "/v1/usage?days=1&page_size=100&method=POST&search=voice-previews%2Fclone", key, nil, &metering); err != nil {
		return nil
	}
	var units *int64
	for _, item := range metering.Items {
		if item.RequestID != requestID || item.Endpoint != "/v1/voice-previews/clone" || item.Method != "POST" {
			continue
		}
		if units != nil || item.Status < 200 || item.Status >= 300 || item.BillableUnits == nil || *item.BillableUnits <= 0 {
			return nil
		}
		units = item.BillableUnits
	}
	return units
}

func wavDuration(data []byte) float64 {
	var rate, size uint32
	for pos := 12; pos+8 <= len(data); {
		n := int(binary.LittleEndian.Uint32(data[pos+4:]))
		start := pos + 8
		if string(data[pos:pos+4]) == "fmt " {
			rate = binary.LittleEndian.Uint32(data[start+8:])
		}
		if string(data[pos:pos+4]) == "data" {
			size = uint32(n)
		}
		pos = start + n + n%2
	}
	return float64(size) / float64(rate)
}
