package breezetts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func (p *Plugin) executeLocal(ctx context.Context, r aigc.Request, g aigc.Generation, params map[string]any, fn func(aigc.Result)) (aigc.Result, error) {
	key := strings.TrimSpace(r.Secrets["API_KEY"])
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return aigc.Result{}, fmt.Errorf("请配置本地 Breeze 服务密钥的环境变量引用")
	}
	var health struct {
		Status     string `json:"status"`
		SampleRate int    `json:"sample_rate"`
	}
	if err := p.jsonRequest(ctx, "GET", "/health", key, nil, &health); err != nil {
		return aigc.Result{}, err
	}
	if health.Status != "ok" || health.SampleRate != 24000 {
		return aigc.Result{}, fmt.Errorf("本地 Breeze 尚未就绪或采样率不匹配")
	}
	var model struct {
		Digest string `json:"model_digest"`
	}
	if err := p.jsonRequest(ctx, "GET", "/v1/model", key, nil, &model); err != nil {
		return aigc.Result{}, err
	}
	if !digestPattern.MatchString(p.ModelDigest) || model.Digest != p.ModelDigest {
		return aigc.Result{}, fmt.Errorf("本地 Breeze 模型摘要不匹配")
	}
	inputs, err := aigc.ResolveInputRoles(p.Manifest().Models[0], params, r.Inputs)
	if err != nil {
		return aigc.Result{}, err
	}
	// The upstream reference-only template has no negative branch for CFG.
	if params["mode"] == "clone" {
		params["cfg_scale"] = float64(1)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, key := range []string{"instruction", "ref_text", "cfg_scale", "seed"} {
		if value, ok := params[key]; ok {
			encoded := fmt.Sprint(value)
			if key == "seed" {
				encoded = strconv.FormatFloat(value.(float64), 'f', 0, 64)
			}
			_ = w.WriteField(key, encoded)
		}
	}
	_ = w.WriteField("text", g.Prompt)
	for _, in := range inputs {
		if err = validateWAV(in.Data, true); err != nil {
			return aigc.Result{}, err
		}
		part, e := w.CreateFormFile("ref_audio", "reference.wav")
		if e != nil {
			return aigc.Result{}, e
		}
		_, _ = part.Write(in.Data)
	}
	_ = w.Close()
	meta := map[string]any{"hostJobId": r.JobID, "channel": "local", "modelDigest": model.Digest, "state": "submitting", "mode": params["mode"], "seed": params["seed"], "cfgScale": params["cfg_scale"]}
	checkpoint(fn, meta)
	start := time.Now()
	resp, err := p.request(ctx, "POST", "/v1/audio/speech", key, w.FormDataContentType(), body.Bytes())
	if err != nil {
		return aigc.Result{}, err
	}
	if resp.StatusCode == 200 && (resp.Header.Get("X-Sample-Rate") != "24000" || resp.Header.Get("X-Sample-Format") != "s16le" || resp.Header.Get("Content-Type") != "audio/pcm") {
		_ = resp.Body.Close()
		return aigc.Result{}, fmt.Errorf("Breeze 返回意外音频格式")
	}
	meta["state"] = "streaming"
	checkpoint(fn, meta)
	var first [2]byte
	if resp.StatusCode != 200 {
		_, err = readResponse(resp, 1024)
		if err == nil {
			err = fmt.Errorf("Breeze 返回意外 HTTP 状态；不会重新生成")
		}
		return aigc.Result{}, err
	}
	_, err = io.ReadFull(resp.Body, first[:])
	if err != nil {
		_ = resp.Body.Close()
		return aigc.Result{}, fmt.Errorf("Breeze 未返回完整音频；不会重新生成")
	}
	meta["ttfaMs"] = float64(time.Since(start).Microseconds()) / 1000
	data, err := readResponse(resp, maxAudioBytes-2)
	if err != nil {
		return aigc.Result{}, err
	}
	pcm := append(first[:], data...)
	if len(pcm)%2 != 0 {
		return aigc.Result{}, fmt.Errorf("Breeze PCM 数据不完整")
	}
	audio := pcmWAV(pcm)
	digest := sha256.Sum256(audio)
	meta["state"] = "succeeded"
	meta["sha256"] = hex.EncodeToString(digest[:])
	meta["sampleRate"] = 24000
	meta["durationSeconds"] = float64(len(pcm)) / 48000
	meta["elapsedMs"] = float64(time.Since(start).Microseconds()) / 1000
	b, _ := json.Marshal(meta)
	return aigc.Result{Media: []aigc.Media{{Data: audio}}, Metadata: b}, nil
}
