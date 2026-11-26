package comfyui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type Plugin struct {
	Definition   Definition
	Client       *http.Client
	PollInterval time.Duration
	MaxBytes     int64
}

// PendingTaskError preserves an already submitted prompt for read-only recovery.
type PendingTaskError struct {
	PromptID string
	Err      error
}

func (e *PendingTaskError) Error() string {
	return "ComfyUI 已提交，结果尚未确认；请读取原回执，不要重新生成"
}
func (e *PendingTaskError) Unwrap() error { return e.Err }

func (e *PendingTaskError) PendingTask() bool { return !errors.Is(e.Err, context.Canceled) }

// ExecutionError means the original history explicitly confirmed execution failure.
type ExecutionError struct{}

func (*ExecutionError) Error() string {
	return "ComfyUI 执行失败，请查看服务端任务记录；不会自动重提"
}

func New(d Definition) *Plugin {
	d = present(d)
	return &Plugin{Definition: d, Client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, PollInterval: time.Second, MaxBytes: 256 << 20}
}
func (p *Plugin) Manifest() aigc.Manifest { return p.Definition.Manifest }

// New definitions can only be created by a trusted administrator. Job parameters
// cannot change the endpoint, graph, output nodes, or upload destinations.
func (p *Plugin) Validate(_ context.Context, r aigc.Request) error {
	var g aigc.Generation
	_ = json.Unmarshal(r.Payload, &g)
	if strings.TrimSpace(g.Prompt) != "" && p.Definition.Prompt == nil {
		return fmt.Errorf("此工作流没有主提示词，请编辑参数表单")
	}
	if r.Voice != nil {
		return fmt.Errorf("ComfyUI 工作流不接受音色 ID，请连接音频素材")
	}
	if err := aigc.ValidateGeneration(p.Manifest(), r); err != nil {
		return err
	}
	for _, in := range r.Inputs {
		if in.URL != "" {
			return fmt.Errorf("ComfyUI 输入必须是宿主授权读取的素材")
		}
	}
	_, err := p.prepareGraph(r)
	return err
}

func (p *Plugin) request(ctx context.Context, method, route, contentType string, body io.Reader, limit int64) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, p.Definition.BaseURL+route, body)
	if e != nil {
		return nil, e
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if p.Definition.Access != nil {
		headers, err := p.Definition.Access.headers(p.Definition.BaseURL)
		if err != nil {
			return nil, err
		}
		for key, values := range headers {
			req.Header[key] = values
		}
	}
	response, e := p.Client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if method == "GET" {
			return nil, errReadTemporary
		}
		return nil, fmt.Errorf("ComfyUI 服务连接失败")
	}
	defer response.Body.Close()
	b, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil && method == "GET" && ctx.Err() == nil {
		return nil, errReadTemporary
	}
	if e != nil || int64(len(b)) > limit {
		return nil, fmt.Errorf("ComfyUI 响应过大或读取失败")
	}
	if method == "GET" && (response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500) {
		return nil, errReadTemporary
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ComfyUI 请求失败（HTTP %d）", response.StatusCode)
	}
	return b, nil
}

// Inspect fetches schemas only. It never submits a prompt or reads history.
func Inspect(ctx context.Context, base string, raw json.RawMessage) (ObjectInfo, error) {
	return inspect(ctx, base, raw, Access{})
}

func inspect(ctx context.Context, base string, raw json.RawMessage, access Access) (ObjectInfo, error) {
	base, e := Endpoint(base)
	if e != nil {
		return nil, e
	}
	g, e := Parse(raw)
	if e != nil {
		return nil, e
	}
	p := New(Definition{BaseURL: base, Access: &access})
	out := ObjectInfo{}
	for _, id := range keys(g) {
		class := g[id].Class
		if _, ok := out[class]; ok {
			continue
		}
		b, e := p.request(ctx, "GET", "/object_info/"+url.PathEscape(class), "", nil, 8<<20)
		if e != nil {
			return nil, e
		}
		var info ObjectInfo
		if json.Unmarshal(b, &info) != nil {
			return nil, fmt.Errorf("ComfyUI 节点定义无效")
		}
		obj, ok := info[class]
		if !ok {
			return nil, fmt.Errorf("ComfyUI 缺少节点类型：%s", class)
		}
		out[class] = obj
	}
	return out, nil
}

func (p *Plugin) upload(ctx context.Context, r aigc.Request, in aigc.Input, index int) (string, error) {
	if len(in.Data) == 0 || int64(len(in.Data)) > p.MaxBytes {
		return "", fmt.Errorf("ComfyUI 输入素材为空或过大")
	}
	// ComfyUI's /upload/image accepts generic multipart file bytes; loader nodes
	// determine decoding. Extensions are derived from bytes, never client names.
	ext := ""
	b := in.Data
	switch {
	case len(b) >= 4 && string(b[:4]) == "fLaC":
		ext = ".flac"
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		ext = ".png"
	case len(b) >= 3 && b[0] == 255 && b[1] == 216 && b[2] == 255:
		ext = ".jpg"
	case len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a"):
		ext = ".gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		ext = ".webp"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		ext = ".wav"
	case len(b) >= 8 && string(b[4:8]) == "ftyp":
		ext = ".mp4"
	case len(b) >= 3 && string(b[:3]) == "ID3" || len(b) > 1 && b[0] == 255 && b[1]&224 == 224:
		ext = ".mp3"
	default:
		return "", fmt.Errorf("ComfyUI 不支持此输入文件格式")
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	// The hash prevents path injection even if a caller uses an arbitrary JobID.
	filename := fmt.Sprintf("ordiweft-%x-%d%s", digest(r.JobID), index, ext)
	part, e := w.CreateFormFile("image", filename)
	if e != nil {
		return "", e
	}
	if _, e = part.Write(b); e != nil {
		return "", e
	}
	_ = w.WriteField("type", "input")
	_ = w.WriteField("overwrite", "false")
	if e = w.Close(); e != nil {
		return "", e
	}
	data, e := p.request(ctx, "POST", "/upload/image", w.FormDataContentType(), &body, 1<<20)
	if e != nil {
		return "", e
	}
	var file struct {
		Name      string `json:"name"`
		Subfolder string `json:"subfolder"`
		Type      string `json:"type"`
	}
	if json.Unmarshal(data, &file) != nil || !safeFile(file.Name, file.Subfolder) || (file.Type != "" && file.Type != "input") {
		return "", fmt.Errorf("ComfyUI 上传回执无效")
	}
	return path.Join(strings.ReplaceAll(file.Subfolder, `\`, "/"), file.Name), nil
}
func digest(s string) [16]byte {
	var out [16]byte
	h := sha256.Sum256([]byte(s))
	copy(out[:], h[:])
	return out
}

func (p *Plugin) Execute(ctx context.Context, r aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	if e := p.Validate(ctx, r); e != nil {
		return aigc.Result{}, e
	}
	if report := checkpoint; report != nil {
		expected := p.ExpectedWait(r).Milliseconds()
		checkpoint = func(v aigc.Result) {
			var meta map[string]any
			if json.Unmarshal(v.Metadata, &meta) == nil && meta != nil {
				meta["estimatedWaitMs"] = expected
				v.Metadata, _ = json.Marshal(meta)
			}
			report(v)
		}
	}
	var g aigc.Generation
	_ = json.Unmarshal(r.Payload, &g)
	model := p.Manifest().Models[0]
	params, e := aigc.NormalizeParameters(model, g.Parameters)
	if e != nil {
		return aigc.Result{}, e
	}
	graph, e := p.prepareGraph(r)
	if e != nil {
		return aigc.Result{}, e
	}
	inputs, e := aigc.ResolveInputRoles(model, params, r.Inputs)
	if e != nil {
		return aigc.Result{}, e
	}
	for i, in := range inputs {
		binding, ok := p.Definition.Roles[in.Role]
		if !ok {
			return aigc.Result{}, fmt.Errorf("ComfyUI 输入角色不存在")
		}
		filename, e := p.upload(ctx, r, in, i)
		if e != nil {
			return aigc.Result{}, e
		}
		n := graph[binding.Node]
		n.Inputs[binding.Input] = filename
	}
	clientID := fmt.Sprintf("ordiweft-%x", digest(r.JobID))
	tracker, closeProgress := p.watchProgress(ctx, clientID)
	defer closeProgress()
	payload, _ := json.Marshal(map[string]any{"prompt": graph, "client_id": clientID})
	// Exactly one submission. A lost reply is uncertain and MUST NOT be retried.
	data, e := p.request(ctx, "POST", "/prompt", "application/json", bytes.NewReader(payload), 2<<20)
	if e != nil {
		return aigc.Result{}, e
	}
	var receipt struct {
		ID         string                     `json:"prompt_id"`
		Error      json.RawMessage            `json:"error"`
		NodeErrors map[string]json.RawMessage `json:"node_errors"`
	}
	if json.Unmarshal(data, &receipt) != nil || receipt.ID == "" || len(receipt.NodeErrors) > 0 {
		return aigc.Result{}, fmt.Errorf("ComfyUI 工作流校验失败或提交结果未确认")
	}
	metadata, _ := json.Marshal(map[string]any{"promptId": receipt.ID, "provider": p.Manifest().ID})
	if checkpoint != nil {
		checkpoint(aigc.Result{Metadata: metadata})
	}
	tracker.mu.Lock()
	tracker.id = receipt.ID
	tracker.mu.Unlock()
	return p.collect(ctx, receipt.ID, tracker, checkpoint)
}

// Collect reads an already submitted prompt's history and media. It never
// uploads inputs or submits a prompt; callers must supply their durable ticket.
func (p *Plugin) Collect(ctx context.Context, promptID string) (aigc.Result, error) {
	return p.collect(ctx, promptID, nil, nil)
}
func (p *Plugin) collect(ctx context.Context, promptID string, tracker *progressTracker, checkpoint func(aigc.Result)) (result aigc.Result, err error) {
	if promptID == "" || len(promptID) > 128 || strings.ContainsAny(promptID, "/\\\x00\r\n") {
		return aigc.Result{}, fmt.Errorf("ComfyUI 任务票据无效")
	}
	defer func() {
		var failed *ExecutionError
		if err != nil && !errors.As(err, &failed) {
			err = &PendingTaskError{PromptID: promptID, Err: err}
		}
	}()
	metadata, _ := json.Marshal(map[string]any{"promptId": promptID, "provider": p.Manifest().ID})
	lastProgress := ""
	publish := func() {
		if tracker == nil {
			return
		}
		metadata, _ = json.Marshal(map[string]any{"promptId": promptID, "provider": p.Manifest().ID, "progress": tracker.snapshot()})
		if checkpoint != nil && string(metadata) != lastProgress {
			checkpoint(aigc.Result{Metadata: metadata})
			lastProgress = string(metadata)
		}
	}
	publish()
	interval := p.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	const maxUnconfirmedPolls = 8
	unconfirmed := 0
	pollUnconfirmed := func() bool {
		unconfirmed++
		return unconfirmed >= maxUnconfirmedPolls
	}
pollLoop:
	for {
		select {
		case <-ctx.Done():
			return aigc.Result{}, ctx.Err()
		case <-ticker.C:
		}
		visibleInQueue := false
		if tracker != nil {
			visibleInQueue = p.queueProgress(ctx, promptID, tracker)
			publish()
		}
		data, e := p.request(ctx, "GET", "/history/"+url.PathEscape(promptID), "", nil, 8<<20)
		if e != nil {
			if errors.Is(e, errReadTemporary) {
				if visibleInQueue {
					unconfirmed = 0
					continue
				}
				if !pollUnconfirmed() {
					continue
				}
			}
			return aigc.Result{}, e
		}
		var history map[string]struct {
			Outputs map[string]json.RawMessage `json:"outputs"`
			Status  struct {
				Completed bool   `json:"completed"`
				State     string `json:"status_str"`
			} `json:"status"`
		}
		if json.Unmarshal(data, &history) != nil {
			return aigc.Result{}, fmt.Errorf("ComfyUI 任务回执无效")
		}
		entry, ok := history[promptID]
		if !ok {
			if visibleInQueue {
				unconfirmed = 0
			} else if tracker != nil && pollUnconfirmed() {
				return aigc.Result{}, errors.New("ComfyUI 原任务长期无法观察；请核对原任务票据")
			}
			continue
		}
		if entry.Status.State == "error" {
			return aigc.Result{}, &ExecutionError{}
		}
		if !entry.Status.Completed {
			unconfirmed = 0
			continue
		}
		if tracker != nil {
			tracker.mu.Lock()
			tracker.state.Stage = "receiving"
			tracker.state.Value = 0
			tracker.state.Max = 0
			tracker.mu.Unlock()
			publish()
		}
		result := aigc.Result{Metadata: metadata, Media: []aigc.Media{}}
		seen := map[string]bool{}
		for _, id := range keys(p.Definition.Outputs) {
			raw, ok := entry.Outputs[id]
			if !ok {
				continue
			}
			var files []outputFile
			if e = collectFiles(raw, &files, 0); e != nil {
				return aigc.Result{}, e
			}
			for _, file := range files {
				if !safeFile(file.Name, file.Subfolder) || (file.Type != "output" && file.Type != "temp") {
					return aigc.Result{}, fmt.Errorf("ComfyUI 产物路径无效")
				}
				q := url.Values{"filename": {file.Name}, "subfolder": {file.Subfolder}, "type": {file.Type}}
				key := q.Encode()
				if seen[key] {
					continue
				}
				seen[key] = true
				if len(result.Media) >= 16 {
					return aigc.Result{}, fmt.Errorf("ComfyUI 单次产物超过 16 份")
				}
				b, e := p.request(ctx, "GET", "/view?"+key, "", nil, p.MaxBytes)
				if e != nil {
					if errors.Is(e, errReadTemporary) && !pollUnconfirmed() {
						continue pollLoop
					}
					return aigc.Result{}, e
				}
				kind := mediaKind(b)
				if kind == "" {
					return aigc.Result{}, fmt.Errorf("ComfyUI 产物不是支持的图像、视频或音频")
				}
				result.Media = append(result.Media, aigc.Media{Data: b, Kind: kind})
			}
		}
		if len(result.Media) == 0 {
			return aigc.Result{}, fmt.Errorf("ComfyUI 已完成但没有可保存的媒体产物")
		}
		return result, nil
	}
}

var errReadTemporary = errors.New("ComfyUI 状态查询暂时中断，继续查询原任务")

func mediaKind(b []byte) string {
	if len(b) >= 4 && string(b[:4]) == "fLaC" {
		return "audio"
	}
	mime := http.DetectContentType(b)
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if len(b) >= 8 && string(b[4:8]) == "ftyp" {
		return "video"
	}
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE" || len(b) >= 3 && string(b[:3]) == "ID3" || len(b) > 1 && b[0] == 255 && b[1]&224 == 224 {
		return "audio"
	}
	return ""
}

type outputFile struct {
	Name      string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

func safeFile(name, folder string) bool {
	// Windows ComfyUI returns relative subfolders with native separators.
	// Normalize separators before traversal checks, never before filename checks.
	folder = strings.ReplaceAll(folder, `\`, "/")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:\x00\r\n") || strings.ContainsAny(folder, ":\x00\r\n") || strings.HasPrefix(folder, "/") {
		return false
	}
	for _, part := range strings.Split(folder, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}
func collectFiles(raw json.RawMessage, out *[]outputFile, depth int) error {
	if depth > 8 || len(*out) > 64 {
		return fmt.Errorf("ComfyUI 输出结构过大")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		if obj["filename"] != nil {
			var f outputFile
			if json.Unmarshal(raw, &f) != nil {
				return fmt.Errorf("ComfyUI 文件描述无效")
			}
			*out = append(*out, f)
			return nil
		}
		for _, k := range keys(obj) {
			if e := collectFiles(obj[k], out, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, v := range list {
			if e := collectFiles(v, out, depth+1); e != nil {
				return e
			}
		}
	}
	return nil
}
