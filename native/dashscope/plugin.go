package dashscope

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const multimodalPath = "/services/aigc/multimodal-generation/generation"
const videoPath = "/services/aigc/video-generation/video-synthesis"

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var workspaceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)
var imageMarker = regexp.MustCompile(`\{\{Image ([0-9]+)\}\}`)
var regions = map[string]bool{"cn-beijing": true, "ap-southeast-1": true, "us-east-1": true, "eu-central-1": true, "ap-northeast-1": true, "cn-hongkong": true}

type Plugin struct {
	VoiceCacheDir string
	managed       *ManagedRuntime
	voiceMu       sync.Mutex
	voiceCache    map[string]*aigc.Voice
	rateMu        sync.Mutex
	nextSubmit    map[string]time.Time
	Client        *http.Client
	region        string
	// Tests override only this package-private endpoint. Node parameters cannot set it.
	baseURL      string
	pollInterval time.Duration
}

func New() *Plugin {
	region := strings.TrimSpace(os.Getenv("DASHSCOPE_REGION"))
	if region == "" {
		region = "cn-beijing"
	}
	return &Plugin{region: region, pollInterval: 3 * time.Second, Client: &http.Client{Timeout: 12 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *Plugin) Manifest() aigc.Manifest {
	m := catalog()
	for i := range m.Models {
		m.Models[i].Pricing = modelPricing(m.Models[i].ID, p.region)
		if v := m.Models[i].QueueLimit; v != nil {
			v.Bucket = p.region + "/" + v.Bucket
		}
	}
	return m
}

// Failure never retains a response body, prompt, endpoint host, or credential.
type Failure struct{ kind string }

func (e *Failure) Error() string { return e.PublicMessage() }
func (e *Failure) PublicMessage() string {
	switch e.kind {
	case "auth":
		return "百炼凭据无效或权限不足，请核对 API Key、业务空间和地域。"
	case "quota":
		return "百炼额度不足或请求受限，请检查账号额度及并发限制。"
	case "input":
		return "百炼拒绝了请求参数或内容，请核对模型能力及参考素材。"
	case "model":
		return "百炼目标模型或接口不可用，请核对账号、地域及模型权限；已创建的音色 ID 已保留，请勿重复创建。"
	case "task":
		return "百炼任务失败或已取消；不会自动重新提交。"
	default:
		return "百炼请求失败或结果未确认；不会自动重新提交。"
	}
}
func (p *Plugin) Validate(_ context.Context, r aigc.Request) error {
	var requested aigc.Generation
	if json.Unmarshal(r.Payload, &requested) == nil {
		switch requested.Model {
		case "qwen-voice-design", "qwen-voice-enrollment", "qwen3-tts-vd-2026-01-26", "qwen3-tts-vc-2026-01-22":
			return errors.New("Qwen3 音色设计和复刻暂不可用，已禁用生成；已有音色 ID 和记录已保留。请使用 Qwen-Audio 3.1 Flash 或 3.0 Plus")
		case "qwen3-tts-flash":
			if len(r.Inputs) > 0 || r.Voice != nil && r.Voice.Kind != "official" {
				return errors.New("Qwen3 Flash 仅支持系统音色，请移除音频或自定义音色连线，或改用 Qwen-Audio 3.1 Flash / 3.0 Plus")
			}
		}
	}
	if err := aigc.ValidateGeneration(p.Manifest(), r); err != nil {
		return err
	}
	if !regions[p.region] {
		return errors.New("不支持的百炼地域，请配置 DASHSCOPE_REGION")
	}
	g, params, err := p.generation(r)
	if err != nil {
		return err
	}
	if strings.HasPrefix(g.Model, "qwen-image-") && len(r.Inputs) > 0 && params["prompt_extend_mode"] == "agent" {
		return errors.New("智能体提示词增强仅支持无参考图的文生图")
	}
	if strings.HasPrefix(g.Model, "wan2.7-image") {
		n, _ := params["n"].(float64)
		if params["enable_sequential"] != true && n > 4 {
			return errors.New("非组图模式最多生成4张图片")
		}
		if params["size"] == "4K" && (len(r.Inputs) > 0 || params["enable_sequential"] == true) {
			return errors.New("4K仅支持万相专业版无参考图的非组图生成")
		}
	}
	if g.Capability == "video.generate" {
		switch params["input_mode"] {
		case "first_frame":
			if len(r.Inputs) != 1 {
				return errors.New("首帧模式必须连接1张图片")
			}
		case "first_last_frame":
			if len(r.Inputs) != 2 {
				return errors.New("首尾帧模式必须按顺序连接2张图片")
			}
		}
	}
	if err := validateVoice(g, params, r); err != nil {
		return err
	}
	// Validate is also called before private input bytes are resolved by the host.
	return nil
}
func (p *Plugin) generation(r aigc.Request) (aigc.Generation, map[string]any, error) {
	var g aigc.Generation
	if err := json.Unmarshal(r.Payload, &g); err != nil {
		return g, nil, errors.New("invalid generation")
	}
	for _, m := range p.Manifest().Models {
		if m.ID == g.Model {
			params, err := aigc.NormalizeParameters(m, g.Parameters)
			return g, params, err
		}
	}
	return g, nil, errors.New("unknown model")
}
func (p *Plugin) endpoint(secrets map[string]string) (string, error) {
	if strings.TrimSpace(secrets["API_KEY"]) == "" || !workspaceID.MatchString(secrets["WORKSPACE_ID"]) {
		return "", errors.New("请配置百炼 API Key 和业务空间 ID 的环境变量引用")
	}
	if p.baseURL != "" {
		return p.baseURL, nil
	}
	return "https://" + secrets["WORKSPACE_ID"] + "." + p.region + ".maas.aliyuncs.com/api/v1", nil
}

type response struct {
	Code      string         `json:"code"`
	RequestID string         `json:"request_id"`
	Usage     map[string]any `json:"usage"`
	Output    struct {
		Voice        string `json:"voice"`
		VoiceID      string `json:"voice_id"`
		TargetModel  string `json:"target_model"`
		Status       string `json:"status"`
		PreviewAudio struct {
			Data       string `json:"data"`
			Format     string `json:"response_format"`
			SampleRate int    `json:"sample_rate"`
		} `json:"preview_audio"`
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		VideoURL   string `json:"video_url"`
		Code       string `json:"code"`
		Audio      struct {
			URL  string `json:"url"`
			Data string `json:"data"`
		} `json:"audio"`
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
}

func (p *Plugin) call(ctx context.Context, method, base, path, key string, body any, async bool) (response, error) {
	var out response
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return out, err
		}
	}
	if method == http.MethodPost {
		if request, ok := body.(map[string]any); ok {
			if model, ok := request["model"].(string); ok {
				if e := p.waitSubmission(ctx, model); e != nil {
					return out, e
				}
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(data))
	if err != nil {
		return out, &Failure{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	if bytes.Contains(data, []byte("oss://")) {
		req.Header.Set("X-DashScope-OssResourceResolve", "enable")
	}
	if async {
		req.Header.Set("X-DashScope-Async", "enable")
	}
	res, err := p.Client.Do(req)
	if err != nil {
		return out, &Failure{}
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		return out, &Failure{}
	}
	if json.Unmarshal(data, &out) != nil {
		return response{}, &Failure{}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || out.Code != "" {
		kind := ""
		switch res.StatusCode {
		case 401, 403:
			kind = "auth"
		case 402, 429:
			kind = "quota"
		case 400:
			kind = "input"
		case 404:
			kind = "model"
		}
		return response{}, &Failure{kind: kind}
	}
	return out, nil
}
func inputImage(in aigc.Input, model string) (string, error) {
	if len(in.Data) == 0 {
		return "", errors.New("百炼参考图必须由宿主提供已授权的图片字节")
	}
	limit := 10 << 20
	wan := strings.HasPrefix(model, "wan")
	if wan {
		limit = 20 << 20
	}
	if len(in.Data) > limit {
		return "", errors.New("百炼参考图超出模型大小限制")
	}
	mime := http.DetectContentType(in.Data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" && (mime != "image/gif" || wan) {
		return "", errors.New("百炼当前模型不支持该参考图格式")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(in.Data), nil
}
func (p *Plugin) Execute(ctx context.Context, r aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	if err := p.Validate(ctx, r); err != nil {
		return aigc.Result{}, err
	}
	base, err := p.endpoint(r.Secrets)
	if err != nil {
		return aigc.Result{}, err
	}
	g, params, err := p.generation(r)
	if err != nil {
		return aigc.Result{}, err
	}
	g.Prompt = imageMarker.ReplaceAllString(g.Prompt, "图${1}")
	queueContext := ctx
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	if g.Capability == "voice.design" {
		if strings.HasPrefix(g.Model, "qwen-voice-") {
			base = p.qwenBase(base)
		}
		return p.createVoice(ctx, base, g, params, r, checkpoint)
	}
	var cloned *aigc.Voice
	if g.Capability == "speech.generate" {
		if len(r.Inputs) > 0 {
			cloned, err = p.cloneForSpeech(ctx, base, g, r, checkpoint)
			if err != nil {
				return aigc.Result{}, err
			}
			r.Voice = cloned
		}
		if r.Voice != nil && r.Voice.Kind != "official" && strings.HasPrefix(g.Model, "qwen-audio-") {
			if err = p.waitVoiceReady(ctx, base, r.Secrets["API_KEY"], r.Voice); err != nil {
				return aigc.Result{}, err
			}
		}
	}
	// Qwen-TTS's documented non-realtime endpoint is the legacy DashScope
	// domain; Qwen-Audio-TTS and the other modalities use workspace domains.
	// Choose once before submission, never retry a paid request at another host.
	if p.baseURL == "" && g.Capability == "speech.generate" && strings.HasPrefix(g.Model, "qwen3-tts-") {
		if p.region == "cn-beijing" {
			base = "https://dashscope.aliyuncs.com/api/v1"
		} else if p.region == "ap-southeast-1" {
			base = "https://dashscope-intl.aliyuncs.com/api/v1"
		}
	}
	input := map[string]any{}
	path := multimodalPath
	if g.Capability == "speech.generate" {
		input = map[string]any{"text": g.Prompt, "voice": params["voice"], "language_type": params["language_type"]}
		if r.Voice != nil {
			input["voice"] = r.Voice.ID
		}
		if strings.HasPrefix(g.Model, "qwen-audio-") {
			path = "/services/audio/tts/SpeechSynthesizer"
			input = map[string]any{"text": g.Prompt, "voice": input["voice"], "format": "wav", "sample_rate": 24000}
			if v, ok := params["instruction"]; ok {
				input["instruction"] = v
			}
		}
		params = map[string]any{}
	} else {
		images := []string{}
		for _, in := range r.Inputs {
			v, e := inputImage(in, g.Model)
			if e != nil {
				return aigc.Result{}, e
			}
			images = append(images, v)
		}
		if g.Capability == "video.generate" {
			path = videoPath
			input["prompt"] = g.Prompt
			media := []map[string]string{}
			for i, v := range images {
				kind := "reference_image"
				if params["input_mode"] == "first_frame" || params["input_mode"] == "first_last_frame" {
					kind = "first_frame"
					if i == 1 {
						kind = "last_frame"
					}
				}
				media = append(media, map[string]string{"type": kind, "url": v})
			}
			if len(media) > 0 {
				input["media"] = media
			}
			delete(params, "input_mode")
		} else {
			content := []map[string]string{}
			for _, v := range images {
				content = append(content, map[string]string{"image": v})
			}
			content = append(content, map[string]string{"text": g.Prompt})
			input["messages"] = []any{map[string]any{"role": "user", "content": content}}
			if params["size"] == "auto" {
				delete(params, "size")
			}
			if strings.HasPrefix(g.Model, "wan2.7") && (len(images) > 0 || params["enable_sequential"] == true) {
				delete(params, "thinking_mode")
			}
		}
	}
	out, err := p.call(ctx, http.MethodPost, base, path, r.Secrets["API_KEY"], map[string]any{"model": g.Model, "input": input, "parameters": params}, g.Capability == "video.generate")
	if err != nil {
		return aigc.Result{}, err
	}
	taskID := ""
	if g.Capability == "video.generate" {
		taskID = out.Output.TaskID
		if !safeID.MatchString(taskID) {
			return aigc.Result{}, &Failure{}
		}
		if checkpoint != nil {
			checkpoint(aigc.Result{Metadata: metadata(out, g.Model, path, taskID)})
		}
		// The submission has a durable ticket. A local preparation deadline is
		// not evidence of remote failure while the provider reports it running.
		ctx = queueContext
		cancel()
		for {
			switch out.Output.TaskStatus {
			case "SUCCEEDED":
				return videoResult(out, g.Model, path, taskID)
			case "FAILED", "CANCELED", "UNKNOWN":
				if out.Output.TaskStatus == "UNKNOWN" {
					return aigc.Result{}, &PendingTaskError{}
				}
				return aigc.Result{}, &Failure{kind: "task"}
			case "PENDING", "RUNNING":
			default:
				return aigc.Result{}, &PendingTaskError{}
			}
			timer := time.NewTimer(p.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return aigc.Result{}, &PendingTaskError{}
			case <-timer.C:
			}
			out, err = p.call(ctx, http.MethodGet, base, "/tasks/"+taskID, r.Secrets["API_KEY"], nil, false)
			if err != nil {
				return aigc.Result{}, &PendingTaskError{}
			}
		}
	}
	result := aigc.Result{Media: []aigc.Media{}, Metadata: metadata(out, g.Model, path, taskID)}
	if g.Capability == "speech.generate" {
		// Prefer the complete file. A data chunk may be raw PCM rather than a WAV file.
		u, e := mediaURL(out.Output.Audio.URL)
		if e != nil {
			return aigc.Result{}, e
		}
		result.Media = append(result.Media, aigc.Media{URL: u, Voice: cloned})
		if cloned != nil {
			var meta map[string]any
			_ = json.Unmarshal(result.Metadata, &meta)
			meta["voiceId"] = cloned.ID
			meta["targetModel"] = cloned.CompatibleModels[0]
			result.Metadata, _ = json.Marshal(meta)
		}
	} else {
		for _, choice := range out.Output.Choices {
			for _, content := range choice.Message.Content {
				if content.Image != "" {
					u, e := mediaURL(content.Image)
					if e != nil {
						return aigc.Result{}, e
					}
					result.Media = append(result.Media, aigc.Media{URL: u})
				}
			}
		}
	}
	if len(result.Media) == 0 || len(result.Media) > 16 {
		return aigc.Result{}, &Failure{}
	}
	return result, nil
}
func videoResult(out response, model, path, taskID string) (aigc.Result, error) {
	u, err := mediaURL(out.Output.VideoURL)
	if err != nil {
		return aigc.Result{}, err
	}
	return aigc.Result{Media: []aigc.Media{{URL: u}}, Metadata: metadata(out, model, path, taskID)}, nil
}
func mediaURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", &Failure{}
	}
	// Official TTS examples return HTTP OSS links; use TLS for the same object.
	if u.Scheme == "http" && strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		u.Scheme = "https"
	}
	if u.Scheme != "https" {
		return "", &Failure{}
	}
	return u.String(), nil // Host downloader enforces public DNS, redirects and size limits.
}
func metadata(out response, model, path, taskID string) json.RawMessage {
	m := map[string]any{"model": model, "endpoint": path}
	if safeID.MatchString(out.RequestID) {
		m["requestId"] = out.RequestID
	}
	if taskID != "" {
		m["taskId"] = taskID
	}
	usage := map[string]any{}
	for _, k := range []string{"input_tokens", "output_tokens", "total_tokens", "image_count", "output_width", "output_height", "input_image_count", "output_image_count", "characters", "duration", "video_count", "count"} {
		if v, ok := out.Usage[k].(float64); ok {
			usage[k] = v
		}
	}
	m["usage"] = usage
	b, _ := json.Marshal(m)
	return b
}
