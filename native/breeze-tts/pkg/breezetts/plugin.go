// Package breezetts adapts self-hosted Breeze inference and the hosted BreezeBlue API.
package breezetts

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

//go:embed local.json
var localManifest []byte

//go:embed online.json
var onlineManifest []byte

const maxAudioBytes = 32 << 20

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Plugin struct {
	Online       bool
	URL          string
	ModelDigest  string
	Client       *http.Client
	PollInterval time.Duration
}

func (p *Plugin) Manifest() aigc.Manifest {
	data := localManifest
	if p.Online {
		data = onlineManifest
	}
	var m aigc.Manifest
	_ = json.Unmarshal(data, &m)
	return m
}

func (p *Plugin) generation(r aigc.Request) (aigc.Generation, map[string]any, error) {
	var g aigc.Generation
	if err := aigc.ValidateGeneration(p.Manifest(), r); err != nil {
		return g, nil, err
	}
	_ = json.Unmarshal(r.Payload, &g)
	for _, m := range p.Manifest().Models {
		if m.ID == g.Model {
			params, err := aigc.NormalizeParameters(m, g.Parameters)
			return g, params, err
		}
	}
	return g, nil, fmt.Errorf("Breeze 模型不存在")
}

func stringValue(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }
func (p *Plugin) Validate(_ context.Context, r aigc.Request) error {
	if !utf8.Valid(r.Payload) {
		return fmt.Errorf("Breeze 参数不是有效 UTF-8；请修正编码后再提交")
	}
	g, params, err := p.generation(r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(g.Prompt) == "" {
		return fmt.Errorf("Breeze 需要填写文本")
	}
	if strings.ContainsRune(g.Prompt, '\uFFFD') {
		return fmt.Errorf("Breeze 文本含编码替换字符；请检查中文编码")
	}
	for _, value := range params {
		if text, ok := value.(string); ok && strings.ContainsRune(text, '\uFFFD') {
			return fmt.Errorf("Breeze 参数含编码替换字符；请检查中文编码")
		}
	}
	if p.Online {
		return p.validateOnline(r, g, params)
	}
	if r.Voice != nil || len(r.Voices) > 0 {
		return fmt.Errorf("本地 Breeze 使用参考音频，不接受在线音色 ID")
	}
	mode := stringValue(params["mode"])
	if mode != "clone" && stringValue(params["instruction"]) == "" {
		return fmt.Errorf("请填写音色或表演描述")
	}
	if mode != "design" && stringValue(params["ref_text"]) == "" {
		return fmt.Errorf("请填写参考音频的准确逐字稿")
	}
	for _, in := range r.Inputs {
		if in.URL != "" {
			return fmt.Errorf("Breeze 仅接受宿主读取的音频")
		}
	}
	return nil
}

func (p *Plugin) base() (string, error) {
	base := p.URL
	if p.Online && base == "" {
		base = "https://api.breeze.blue"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("Breeze 服务地址配置无效")
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func (p *Plugin) request(ctx context.Context, method, path, key, contentType string, body []byte) (*http.Response, error) {
	base, err := p.base()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Breeze 请求无效")
	}
	if key != "" {
		req.Header.Set("xi-api-key", key)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	client := http.Client{Timeout: 14 * time.Minute}
	if p.Client != nil {
		client = *p.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Breeze 请求未确认；不会自动重试，请保留任务记录")
	}
	return resp, nil
}

func readResponse(resp *http.Response, limit int64) ([]byte, error) {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Breeze 请求失败（HTTP %d）；不会自动重试", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("Breeze 响应不完整或超出限制；不会重新生成")
	}
	return data, nil
}

func (p *Plugin) jsonRequest(ctx context.Context, method, path, key string, body any, out any) error {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("Breeze 参数编码失败")
		}
	}
	resp, err := p.request(ctx, method, path, key, "application/json", data)
	if err != nil {
		return err
	}
	data, err = readResponse(resp, 48<<20)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, out) != nil {
		return fmt.Errorf("Breeze 返回无效 JSON")
	}
	return nil
}

func checkpoint(fn func(aigc.Result), meta map[string]any) {
	if fn != nil {
		b, _ := json.Marshal(meta)
		fn(aigc.Result{Metadata: b})
	}
}

func (p *Plugin) Execute(ctx context.Context, r aigc.Request, fn func(aigc.Result)) (aigc.Result, error) {
	if err := p.Validate(ctx, r); err != nil {
		return aigc.Result{}, err
	}
	if r.JobID == "" || len(r.JobID) > 128 || strings.ContainsAny(r.JobID, "\r\n\x00") {
		return aigc.Result{}, fmt.Errorf("Breeze 缺少有效宿主任务 ID")
	}
	g, params, _ := p.generation(r)
	if p.Online {
		return p.executeOnline(ctx, r, g, params, fn)
	}
	return p.executeLocal(ctx, r, g, params, fn)
}

func pcmWAV(pcm []byte) []byte {
	out := make([]byte, 44+len(pcm))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], 1)
	binary.LittleEndian.PutUint32(out[24:], 24000)
	binary.LittleEndian.PutUint32(out[28:], 48000)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm)))
	copy(out[44:], pcm)
	return out
}

func validateWAV(data []byte, reference bool) error {
	if len(data) < 44 || len(data) > maxAudioBytes || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return fmt.Errorf("Breeze 需要完整 PCM WAV 音频")
	}
	if int64(binary.LittleEndian.Uint32(data[4:8]))+8 != int64(len(data)) {
		return fmt.Errorf("Breeze WAV 长度校验失败")
	}
	var byteRate, blockAlign, dataSize uint32
	var formatOK bool
	for pos := 12; pos+8 <= len(data); {
		n := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		end := int64(start) + n
		if end > int64(len(data)) {
			return fmt.Errorf("Breeze WAV 数据块不完整")
		}
		switch string(data[pos : pos+4]) {
		case "fmt ":
			if n < 16 {
				return fmt.Errorf("Breeze WAV 格式错误")
			}
			f := data[start:int(end)]
			channels := binary.LittleEndian.Uint16(f[2:4])
			rate := binary.LittleEndian.Uint32(f[4:8])
			byteRate = binary.LittleEndian.Uint32(f[8:12])
			blockAlign = uint32(binary.LittleEndian.Uint16(f[12:14]))
			formatOK = binary.LittleEndian.Uint16(f[:2]) == 1 && (channels == 1 || channels == 2) && binary.LittleEndian.Uint16(f[14:16]) == 16 && rate >= 8000 && rate <= 96000 && blockAlign == uint32(channels)*2 && byteRate == rate*blockAlign
		case "data":
			dataSize = uint32(n)
		}
		pos = int(end + n%2)
	}
	if !formatOK || dataSize == 0 || byteRate == 0 || blockAlign == 0 || dataSize%blockAlign != 0 {
		return fmt.Errorf("Breeze 需要非空的 16-bit PCM WAV")
	}
	seconds := float64(dataSize) / float64(byteRate)
	if reference && (seconds < 0.25 || seconds > 30) {
		return fmt.Errorf("Breeze 参考音频须为 0.25–30 秒")
	}
	return nil
}
