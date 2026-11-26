package tapnowweb

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	mu                                     sync.Mutex
	authMu                                 sync.Mutex
	uploadMu                               sync.Mutex
	reviewMu                               sync.Mutex
	canvasMu                               sync.Mutex
	usageMu                                sync.Mutex
	quoteMu                                sync.Mutex
	previewQuotes                          map[string]previewQuote
	refreshToken                           string
	passwordLogin                          *PasswordLogin
	userID                                 string
	authPaused                             bool
	retryAt                                int64
	authFailures                           int
	expiresAt, lastRefreshAt               int64
	authError                              string
	accountRevision                        uint64
	catalog                                Catalog
	verified                               bool
	origin, token, org, dir, refreshScript string
	http                                   *http.Client
	download                               *tapnow.Client
	PollInterval, Timeout                  time.Duration
	MaxReferenceBytes                      int64
}
type Status struct {
	Connected bool    `json:"connected"`
	Reason    string  `json:"reason,omitempty"`
	Catalog   Catalog `json:"catalog"`
}

// DefaultPreparationTimeout bounds preflight/submission, not polling a saved
// task ticket. Remote queueing/execution is governed by task state and cancel.
const DefaultPreparationTimeout = 2 * time.Hour

const defaultOrigin = "https://app.tapnow.media"

func New(origin, token, org, dir, script string) *Client {
	if script == "" {
		if info, e := os.Stat("/opt/ordiweft/scripts/tapnow-catalog.mjs"); e == nil && info.Mode().IsRegular() {
			script = "/opt/ordiweft/scripts/tapnow-catalog.mjs"
		}
	}
	if origin == "" {
		origin = defaultOrigin
	}
	if origin != "https://app.tapnow.ai" && origin != "https://app.tapnow.media" {
		origin = defaultOrigin
		token = ""
	}
	c := &Client{catalog: Baseline(), origin: origin, token: token, org: org, dir: dir, refreshScript: script, http: &http.Client{Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, PollInterval: 15 * time.Second, Timeout: DefaultPreparationTimeout, download: tapnow.New("", ""), MaxReferenceBytes: 20 << 20}
	if b, e := os.ReadFile(filepath.Join(dir, "catalog.json")); e == nil {
		if catalog, e := ParseCatalog(b); e == nil && !catalog.observedBefore(c.catalog) {
			c.catalog = catalog
		}
	}
	c.loadAccount()
	return c
}
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	reason := ""
	if c.token == "" {
		reason = "Web 通道需管理员配置独立的 TapNow Web access token；MCP OAuth 不能复用"
	} else if !c.verified {
		reason = "凭据已配置，尚未通过 Web 身份验证；请管理员验证连接"
	}
	return Status{c.verified, reason, c.catalog.withDraftFinal()}
}
func (c *Client) Prepare(r Request) (map[string]any, Mode, error) {
	c.mu.Lock()
	cat := c.catalog
	c.mu.Unlock()
	return cat.Prepare(r)
}
func (c *Client) request(ctx context.Context, method, path string, body, out any) error {
	if e := c.ensureToken(ctx); e != nil {
		return e
	}
	c.mu.Lock()
	token, org := c.token, c.org
	c.mu.Unlock()
	e := c.requestWithToken(ctx, method, path, body, out, token, org)
	if errors.Is(e, errUnauthorized) {
		c.mu.Lock()
		if c.token == token {
			c.verified = false
		}
		c.mu.Unlock()
	}
	// Only read requests can be replayed. Never repeat a paid/write request.
	if method == "GET" && errors.Is(e, errUnauthorized) {
		if refreshErr := c.refreshAfter(ctx, token); refreshErr == nil {
			c.mu.Lock()
			token, org = c.token, c.org
			c.mu.Unlock()
			return c.requestWithToken(ctx, method, path, body, out, token, org)
		}
	}
	return e
}

func (c *Client) accountCredentials(ctx context.Context, revision uint64) (string, string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", "", err
	}
	c.mu.Lock()
	token, org, current := c.token, c.org, c.accountRevision
	c.mu.Unlock()
	if current != revision {
		return "", "", errors.New("TapNow 登录身份已变更，请重新确认任务")
	}
	if token == "" {
		return "", "", errors.New("TapNow Web 尚未配置")
	}
	return token, org, nil
}

// A running job must not switch accounts between submission and writeback.
func (c *Client) requestForAccount(ctx context.Context, revision uint64, method, path string, body, out any) error {
	token, org, err := c.accountCredentials(ctx, revision)
	if err != nil {
		return err
	}
	err = c.requestWithToken(ctx, method, path, body, out, token, org)
	if errors.Is(err, errUnauthorized) {
		c.mu.Lock()
		if c.token == token {
			c.verified = false
		}
		c.mu.Unlock()
		if method == "GET" && c.refreshAfter(ctx, token) == nil {
			token, org, err = c.accountCredentials(ctx, revision)
			if err == nil {
				return c.requestWithToken(ctx, method, path, body, out, token, org)
			}
		}
	}
	return err
}

var errUnauthorized = errors.New("TapNow Web 登录失效，请刷新或重新登录")
var errReadTemporary = errors.New("Web 查询暂时中断；保留原任务票据")

// TaskFailedError is reserved for an explicit terminal failure from the task API.
type TaskFailedError struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

func (c *Client) requestWithToken(ctx context.Context, method, path string, body, out any, token, org string) error {
	if token == "" && path != "/api/public/v1/auth/login" && path != "/api/public/v1/auth/tokens/refresh" {
		return errors.New("TapNow Web 尚未配置")
	}
	if !strings.HasPrefix(path, "/api/") || strings.Contains(path, "..") || strings.Contains(path, "://") {
		return errors.New("Web 路径无效")
	}
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return errors.New("Web 参数序列化失败")
		}
		reader = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.origin+path, reader)
	if e != nil {
		return errors.New("Web 请求构造失败")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-tapnow-origin", strings.TrimPrefix(c.origin, "https://"))
	if org != "" {
		req.Header.Set("X-Org-ID", org)
	}
	res, e := c.http.Do(req)
	if e != nil {
		if method == "GET" {
			return errReadTemporary
		}
		return errors.New("Web 请求未完成；写入结果可能未知")
	}
	defer res.Body.Close()
	if method == "GET" && (res.StatusCode == 408 || res.StatusCode == 429 || res.StatusCode >= 500) {
		return errReadTemporary
	}
	if res.StatusCode == 401 {
		return errUnauthorized
	}
	if res.StatusCode >= 500 {
		return fmt.Errorf("TapNow Web HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Code json.RawMessage `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if e != nil && method == "GET" {
		return errReadTemporary
	}
	success := res.StatusCode >= 200 && res.StatusCode < 300
	if e != nil || len(b) > 8<<20 || json.Unmarshal(b, &envelope) != nil {
		if success {
			return errors.New("Web 响应格式或长度异常")
		}
		return responseFailure(res.StatusCode, nil, strings.HasPrefix(path, "/api/public/v1/auth/"))
	}
	if code := errorValue(envelope.Code); !success || code != "0" && code != "200" {
		return responseFailure(res.StatusCode, b, strings.HasPrefix(path, "/api/public/v1/auth/"))
	}
	if out != nil {
		if json.Unmarshal(envelope.Data, out) != nil {
			return errors.New("Web data 响应格式异常")
		}
	}
	return nil
}

type Identity struct {
	User string `json:"user_id"`
	Org  string `json:"org_id"`
}

func (c *Client) Identity(ctx context.Context) (Identity, error) {
	var i Identity
	e := c.request(ctx, "GET", "/api/public/v1/users/me", nil, &i)
	if e == nil && (i.User == "" || i.Org == "") {
		e = errors.New("Web 身份缺少用户或组织")
	}
	c.mu.Lock()
	org := c.org
	c.mu.Unlock()
	if e == nil && org != "" && i.Org != org {
		e = errors.New("所选组织与 Web 身份不一致")
	}
	return i, e
}
func (c *Client) Probe(ctx context.Context) error {
	_, e := c.Identity(ctx)
	c.mu.Lock()
	c.verified = e == nil
	c.mu.Unlock()
	return e
}
func (c *Client) Refresh(ctx context.Context) error {
	b, e := c.FetchCatalog(ctx)
	if e != nil {
		return e
	}
	return c.ApplyCatalog(b)
}

// FetchCatalog reads public static metadata only; it does not change live state.
func (c *Client) FetchCatalog(ctx context.Context) ([]byte, error) {
	if c.refreshScript == "" {
		return nil, errors.New("管理员需配置本项目的静态目录更新器路径")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", c.refreshScript)
	cmd.Env = []string{}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "HOME", "USERPROFILE"} {
		if v, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+v)
		}
	}
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return nil, errors.New("目录更新器失败；保留上次成功目录")
	}
	if _, e := ParseCatalog(output.Bytes()); e != nil {
		return nil, e
	}
	return output.Bytes(), nil
}

func (c *Client) ApplyCatalog(b []byte) error {
	cat, e := ParseCatalog(b)
	if e != nil {
		return e
	}
	if e = c.save("catalog.json", b); e != nil {
		return errors.New("新目录保存失败")
	}
	c.mu.Lock()
	c.catalog = cat
	c.mu.Unlock()
	return nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8<<20 {
		return 0, errors.New("too large")
	}
	return b.Buffer.Write(p)
}
func (c *Client) save(name string, b []byte) error {
	if e := os.MkdirAll(c.dir, 0700); e != nil {
		return e
	}
	path := filepath.Join(c.dir, name)
	f, e := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path+".tmp", path)
}

type Ticket struct {
	TaskIDs  []string `json:"taskIds"`
	CanvasID string   `json:"canvasId,omitempty"`
	NodeID   string   `json:"nodeId,omitempty"`
}
type Receipt struct {
	RequestPrompt string           `json:"requestPrompt,omitempty"`
	PromptBlocks  json.RawMessage  `json:"promptBlocks,omitempty"`
	Owner         string           `json:"owner"`
	Phase         string           `json:"phase"`
	Ticket        Ticket           `json:"ticket"`
	Cost          float64          `json:"estimatedCost"`
	Failure       *TaskFailedError `json:"failure,omitempty"`
	DraftPricing  *draftPricing    `json:"draftPricing,omitempty"`
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func (c *Client) Receipt(id string) (Receipt, bool) {
	if !safeID.MatchString(id) {
		return Receipt{}, false
	}
	b, e := os.ReadFile(filepath.Join(c.dir, "job-"+id+".json"))
	var r Receipt
	if e != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, true
}
func (c *Client) receipt(id string, r Receipt) error {
	b, _ := json.Marshal(r)
	return c.save("job-"+id+".json", b)
}

// ConfirmRecovered marks only an existing ticket whose local output was saved.
func (c *Client) ConfirmRecovered(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.Receipt(id)
	if !ok || len(r.Ticket.TaskIDs) == 0 || r.Phase == "canceled" {
		return errors.New("原任务票据不可更新")
	}
	r.Phase = "done"
	return c.receipt(id, r)
}
func EstimateBody(kind string, body map[string]any) map[string]any {
	params := merge(body, map[string]any{})
	for _, k := range []string{"context", "metadata", "prompt", "times", "provider", "model", "scene", "cameraControl"} {
		delete(params, k)
	}
	out := map[string]any{"type": "GT_IMAGE", "model": body["model"], "provider": body["provider"], "usageQuantity": fmt.Sprint(body["times"])}
	if kind == "image" {
		params["model"] = body["model"]
		out["imageParams"] = params
	} else {
		out["type"] = "GT_VIDEO"
		params["realModel"] = body["model"]
		if resolution := params["resolution"]; resolution != nil {
			params["quality"] = resolution
		}
		for _, k := range []string{"images", "videos", "audios"} {
			if list := urlList(params[k]); len(list) > 0 {
				params[k] = list
			}
		}
		for key, targets := range map[string][]string{"images": {"image_url", "first_frame_image_url", "last_frame_image_url", "first_frame_image", "last_frame_image", "image_list"}, "reference_images": {"reference_image_urls", "reference_images"}, "videos": {"video_url", "reference_video_urls", "video_list"}, "audios": {"reference_audio_urls", "audio_url", "kling_audio_refs"}} {
			if len(urlList(params[key])) > 0 {
				continue
			}
			urls := []string{}
			for _, target := range targets {
				urls = append(urls, urlList(params[target])...)
			}
			if len(urls) > 0 {
				params[key] = urls
			}
		}
		for _, content := range arr(params["kling_contents"]) {
			entry := obj(content)
			if uri := str(entry, "url"); uri != "" {
				key := "images"
				if strings.HasSuffix(str(entry, "type"), "_video") {
					key = "videos"
				}
				urls, _ := params[key].([]string)
				params[key] = append(urls, uri)
			}
		}
		out["videoParams"] = params
	}
	return out
}
func urlList(v any) []string {
	switch x := v.(type) {
	case string:
		if x != "" {
			return []string{x}
		}
	case []string:
		return x
	case []any:
		out := []string{}
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else {
				m := obj(item)
				for _, key := range []string{"url", "image_url", "video_url"} {
					if s := str(m, key); s != "" {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
	return nil
}
func (c *Client) Estimate(ctx context.Context, kind string, body map[string]any) (float64, error) {
	return c.estimateForAccount(ctx, kind, body, nil)
}

func (c *Client) estimateForAccount(ctx context.Context, kind string, body map[string]any, revision *uint64) (float64, error) {
	var quote struct {
		Cost string `json:"tapiesCost"`
	}
	path, payload := "/api/billing/v2/estimate", EstimateBody(kind, body)
	var e error
	if revision == nil {
		e = c.request(ctx, "POST", path, payload, &quote)
	} else {
		e = c.requestForAccount(ctx, *revision, "POST", path, payload, &quote)
	}
	if e != nil {
		return 0, e
	}
	cost, e := strconv.ParseFloat(quote.Cost, 64)
	if e != nil || strings.TrimSpace(quote.Cost) == "" || math.IsInf(cost, 0) || math.IsNaN(cost) || cost < 0 {
		return 0, errors.New("费用预估无可信 tapiesCost，禁止提交")
	}
	return cost, nil
}

type Material struct {
	Bytes    []byte
	Mime     string
	Duration float64
}
type Resolve func(context.Context, string) (Material, error)

func (c *Client) Upload(ctx context.Context, m Material) (string, error) {
	kind := "image"
	if strings.HasPrefix(m.Mime, "video/") {
		kind = "video"
	} else if strings.HasPrefix(m.Mime, "audio/") {
		kind = "audio"
	}
	var e error
	m, e = inspectMaterial(m, kind)
	if e != nil {
		return "", e
	}
	if len(m.Bytes) == 0 || len(m.Bytes) > 256<<20 || !(m.Mime == "image/png" || m.Mime == "image/jpeg" || m.Mime == "image/webp" || m.Mime == "video/mp4" || m.Mime == "audio/wav") {
		return "", errors.New("上传素材格式或大小不支持")
	}
	sum := md5.Sum(m.Bytes)
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "video/mp4": ".mp4", "audio/wav": ".wav"}[m.Mime]
	var reply struct {
		ID  string `json:"file_id"`
		URL string `json:"upload_url"`
	}
	if e := c.request(ctx, "POST", "/api/conversation/v1/file/upload-url", map[string]any{"file_name": "canvas-reference" + ext, "mime_type": m.Mime, "file_hash": hex.EncodeToString(sum[:]), "file_size": len(m.Bytes), "is_temporary": false, "is_resumable_uploaded": false}, &reply); e != nil {
		return "", fmt.Errorf("初始化素材上传：%w", e)
	}
	if !safeFileID(reply.ID) {
		return "", errors.New("上传响应缺少有效 file_id")
	}
	if reply.URL != "" {
		u, e := url.Parse(reply.URL)
		if e != nil || !storageHost(u.Hostname()) || publicURL(reply.URL) != nil {
			return "", errors.New("签名上传地址不可信")
		}
		req, _ := http.NewRequestWithContext(ctx, "PUT", reply.URL, bytes.NewReader(m.Bytes))
		req.Header.Set("Content-Type", m.Mime)
		req.Header.Set("x-goog-if-generation-match", "0")
		req.Header.Set("x-oss-forbid-overwrite", "true")
		req.Header.Set("Cache-Control", "public, max-age=86400")
		res, e := uploadClient().Do(req)
		if e != nil {
			return "", errors.New("上传写入未确认")
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
			res.Body.Close()
			code := regexp.MustCompile(`<Code>([A-Za-z0-9_]{1,80})</Code>`).FindSubmatch(b)
			if len(code) == 2 {
				return "", fmt.Errorf("上传写入 HTTP %d (%s)", res.StatusCode, code[1])
			}
			return "", fmt.Errorf("上传写入 HTTP %d", res.StatusCode)
		}
		res.Body.Close()
		if e = c.request(ctx, "PUT", "/api/conversation/v1/file/upload-url/"+url.PathEscape(reply.ID), nil, nil); e != nil {
			return "", fmt.Errorf("确认素材上传：%w", e)
		}
	}
	return "https://files.tapnow.media/api/conversation/storage/uploads/" + url.PathEscape(reply.ID), nil
}
func storageHost(host string) bool {
	for _, d := range []string{"tapnow.media", "tapnow.ai", "tapnow.art", "storage.googleapis.com", "aliyuncs.com"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
func safeFileID(id string) bool { return regexp.MustCompile(`^[A-Za-z0-9_-]{1,180}$`).MatchString(id) }
func (c *Client) Produce(ctx context.Context, id, owner, localCanvasID string, r Request, resolve Resolve, checkpoint func(Ticket)) ([]tapnow.Media, Ticket, error) {
	if !safeID.MatchString(id) {
		return nil, Ticket{}, errors.New("任务 ID 无效")
	}
	if !safeFileID(localCanvasID) {
		return nil, Ticket{}, errors.New("画布 ID 无效")
	}
	c.mu.Lock()
	if old, ok := c.Receipt(id); ok {
		c.mu.Unlock()
		return nil, old.Ticket, errors.New("任务已保存，禁止重复提交")
	}
	receipt := Receipt{Owner: owner, Phase: "preparing", PromptBlocks: r.PromptBlocks, RequestPrompt: r.Prompt}
	e := c.receipt(id, receipt)
	c.mu.Unlock()
	if e != nil {
		return nil, Ticket{}, errors.New("不能保存任务记录")
	}
	queueContext := ctx
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	finish := func(phase string) {
		if ctx.Err() == context.Canceled {
			phase = "canceled"
		}
		receipt.Phase = phase
		_ = c.receipt(id, receipt)
	}
	body, mode, e := c.Prepare(r)
	if e != nil {
		finish("rejected")
		return nil, Ticket{}, e
	}
	c.mu.Lock()
	accountRevision := c.accountRevision
	c.mu.Unlock()
	identity, e := c.Identity(ctx)
	if e != nil {
		finish("rejected")
		return nil, Ticket{}, e
	}
	if e = r.checkDraftAccount(accountRevision); e != nil || isDraftFinal(r) && r.UseEstimatedCost {
		finish("rejected")
		if e == nil {
			e = errors.New("正式视频必须确认 Tapies 费用上限")
		}
		return nil, Ticket{}, e
	}
	refs := append([]tapnow.Reference(nil), r.References...)
	materials := map[string]Material{}
	var durations referenceDurations
	// Inspect every private reference before the first upload. Never trust user duration metadata.
	for _, ref := range refs {
		if strings.HasPrefix(ref.URL, "/files/") {
			if resolve == nil {
				finish("rejected")
				return nil, Ticket{}, errors.New("缺少私人素材解析器")
			}
			m, e := resolve(ctx, ref.URL)
			if e != nil {
				finish("rejected")
				return nil, Ticket{}, e
			}
			m, e = inspectMaterial(m, ref.Kind)
			if e != nil || int64(len(m.Bytes)) > c.MaxReferenceBytes {
				finish("rejected")
				return nil, Ticket{}, errors.New("素材实际类型与连线不一致")
			}
			materials[ref.URL] = m
			if e = durations.add(mode, ref.Kind, m.Duration); e != nil {
				finish("rejected")
				return nil, Ticket{}, e
			}
		} else if ref.Kind == "image" {
			b, e := c.Download(ctx, ref.URL, c.MaxReferenceBytes)
			if e != nil {
				finish("rejected")
				return nil, Ticket{}, e
			}
			if _, e = inspectMaterial(Material{Bytes: b}, "image"); e != nil {
				finish("rejected")
				return nil, Ticket{}, e
			}
		}
	}
	if e = durations.validateTotals(mode); e != nil {
		finish("rejected")
		return nil, Ticket{}, e
	}
	uploaded := map[string]string{}
	for i, ref := range refs {
		if m, ok := materials[ref.URL]; ok {
			u := uploaded[ref.URL]
			if u == "" {
				u, e = c.cachedUpload(ctx, identity, m)
				if e != nil {
					finish("unknown")
					return nil, Ticket{}, e
				}
				uploaded[ref.URL] = u
			}
			refs[i].URL = u
		}
	}
	// Replace only verified graph inputs inside the already validated wire body.
	for _, key := range []string{"images", "videos", "audios", "image_url", "video_url", "audio_url", "reference_images", "reference_image_urls", "reference_video_urls", "reference_audio_urls", "first_frame_image", "last_frame_image", "first_frame_image_url", "last_frame_image_url", "image_list", "video_list", "reference_urls", "kling_contents", "kling_audio_refs"} {
		if value, ok := body[key]; ok {
			body[key] = replaceURLs(value, uploaded)
		}
	}
	if strings.HasPrefix(r.Model, "seedance-2") {
		if e = c.bindSeedanceImageReviews(ctx, accountRevision, arr(body["images"]), uploaded); e != nil {
			finish("rejected")
			return nil, Ticket{}, e
		}
	}
	if body["draft"] == true {
		receipt.DraftPricing = draftPricingFrom(body)
	}
	var cost float64
	if r.Draft != nil {
		cost, e = c.estimateForAccount(ctx, r.Kind, r.Draft.pricing.estimateBody(body), &accountRevision)
	} else {
		cost, e = c.estimateVerified(ctx, r.Kind, body, durations.estimateDuration(mode))
	}
	if e != nil || (!r.UseEstimatedCost && cost > r.MaxCost) {
		finish("rejected")
		if e == nil {
			e = fmt.Errorf("%w。预估 %.2f Tapies，本次上限 %.2f Tapies", ProviderFailure("cost_cap"), cost, r.MaxCost)
		}
		return nil, Ticket{}, e
	}
	receipt.Cost = cost
	var wallet Wallet
	if r.Draft != nil {
		wallet, e = c.walletForAccount(ctx, &accountRevision)
	} else {
		wallet, e = c.Wallet(ctx)
	}
	if e != nil || wallet.Available < cost {
		finish("rejected")
		if e == nil {
			e = ProviderFailure("insufficient_credits")
		}
		return nil, Ticket{}, e
	}
	receipt.Phase = "creating"
	if e = c.receipt(id, receipt); e != nil {
		return nil, Ticket{}, e
	}
	expectedCanvas := ""
	if r.Draft != nil {
		expectedCanvas = r.Draft.sourceCanvas
	}
	canvasID, position, e := c.remoteCanvas(ctx, identity, localCanvasID, accountRevision, len(refs), expectedCanvas)
	if e != nil {
		phase := "unknown"
		if errors.Is(e, errDraftUnavailable) {
			phase = "rejected"
		}
		finish(phase)
		return nil, Ticket{}, e
	}
	nodeID := r.Kind + "-" + id
	receipt.Ticket = Ticket{CanvasID: canvasID, NodeID: nodeID}
	if e = c.receipt(id, receipt); e != nil {
		return nil, receipt.Ticket, e
	}
	node := map[string]any{"id": nodeID, "canvas_id": canvasID, "type": r.Kind, "position": position, "data": map[string]any{"type": "generate", "prompt": r.Prompt, "params": r.remoteParameters(r.Parameters)}}
	if e = c.createRemoteNodes(ctx, accountRevision, receipt.Ticket, node, refs, position); e != nil {
		finish("unknown")
		return nil, receipt.Ticket, e
	}
	body["metadata"] = map[string]string{"canvas_id": canvasID, "node_id": nodeID}
	body["context"] = map[string]string{"canvas_id": canvasID, "node_id": nodeID, "user_id": identity.User, "org_id": identity.Org}
	receipt.Phase = "submitting"
	if e = c.receipt(id, receipt); e != nil {
		return nil, receipt.Ticket, e
	}
	var reply struct {
		Result struct {
			IDs []string `json:"ids"`
		} `json:"result"`
	}
	if e = c.requestForAccount(ctx, accountRevision, "POST", generationPath(r), body, &reply); e != nil {
		var rejected ProviderFailure
		if errors.As(e, &rejected) || errors.Is(e, errUnauthorized) {
			finish("rejected")
			return nil, receipt.Ticket, e
		}
		finish("unknown")
		return nil, receipt.Ticket, ErrSubmissionUnconfirmed
	}
	if len(reply.Result.IDs) == 0 || len(reply.Result.IDs) > 8 {
		finish("unknown")
		return nil, receipt.Ticket, ErrSubmissionUnconfirmed
	}
	seen := map[string]bool{}
	for _, tid := range reply.Result.IDs {
		if !safeFileID(tid) || seen[tid] {
			finish("unknown")
			return nil, receipt.Ticket, ErrSubmissionUnconfirmed
		}
		seen[tid] = true
	}
	receipt.Ticket.TaskIDs = reply.Result.IDs
	receipt.Phase = "pending"
	if e = c.receipt(id, receipt); e != nil {
		return nil, receipt.Ticket, e
	}
	if checkpoint != nil {
		checkpoint(receipt.Ticket)
	}
	// Once the one paid submission has a durable ticket, continue observing its
	// actual state instead of declaring failure at the preparation deadline.
	ctx = queueContext
	cancel()
	media, e := c.poll(ctx, receipt.Ticket, r.Kind, &accountRevision)
	if e != nil {
		phase := "pending"
		var failed *TaskFailedError
		if errors.As(e, &failed) {
			phase = "failed"
			receipt.Failure = failed.Safe()
			_ = c.writeFailure(ctx, receipt.Ticket, obj(node["data"]), failed, accountRevision)
		}
		finish(phase)
	} else {
		// Remote presentation metadata is best effort. A failed write must not hide
		// completed media or prevent the application's independent asset writeback.
		_ = c.writeBack(ctx, receipt.Ticket, r, mode, media, accountRevision)
		finish("done")
	}
	return media, receipt.Ticket, e
}
func replaceURLs(v any, urls map[string]string) any {
	switch x := v.(type) {
	case string:
		if u := urls[x]; u != "" {
			return u
		}
	case []string:
		o := make([]string, len(x))
		for i, s := range x {
			o[i] = replaceURLs(s, urls).(string)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, item := range x {
			o[i] = replaceURLs(item, urls)
		}
		return o
	case map[string]any:
		o := map[string]any{}
		for k, item := range x {
			o[k] = replaceURLs(item, urls)
		}
		return o
	}
	return v
}
func (c *Client) Poll(ctx context.Context, t Ticket, kind string) ([]tapnow.Media, error) {
	return c.poll(ctx, t, kind, nil)
}

func (c *Client) poll(ctx context.Context, t Ticket, kind string, revision *uint64) ([]tapnow.Media, error) {
	const maxUnconfirmedPolls = 8
	_, hasDeadline := ctx.Deadline()
	unconfirmed := 0
	for {
		var tasks []map[string]any
		path := "/api/conversation/v1/generations/tasks?ids=" + url.QueryEscape(strings.Join(t.TaskIDs, ","))
		var e error
		if revision == nil {
			e = c.request(ctx, "GET", path, nil, &tasks)
		} else {
			e = c.requestForAccount(ctx, *revision, "GET", path, nil, &tasks)
		}
		if e != nil {
			unconfirmed++
			if errors.Is(e, errReadTemporary) && (hasDeadline || unconfirmed < maxUnconfirmedPolls) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(c.PollInterval):
					continue
				}
			}
			return nil, e
		}
		unconfirmed = 0
		found := map[string]map[string]any{}
		for _, task := range tasks {
			tid := str(task, "id")
			if found[tid] != nil {
				return nil, errors.New("任务查询重复 ID")
			}
			found[tid] = task
		}
		done := true
		out := []tapnow.Media{}
		for _, tid := range t.TaskIDs {
			task := found[tid]
			if task == nil {
				return nil, errors.New("任务查询遗漏 ID；保留票据")
			}
			switch str(task, "status") {
			case "failed", "cancelled", "canceled":
				return nil, &TaskFailedError{Reason: errorScalar(task["failed_reason"]), Detail: str(task, "failed_reason_detail")}
			case "completed":
				urls := completedTaskURLs(task, kind)
				if len(urls) == 0 {
					return nil, errors.New("任务完成但没有产物")
				}
				for _, u := range urls {
					if c.download.PublicURL(ctx, u) != nil {
						return nil, errors.New("任务产物地址无效")
					}
					out = append(out, tapnow.Media{NodeID: tid, URL: u, Kind: kind})
				}
			case "pending", "running", "processing", "queued":
				done = false
			default:
				return nil, errors.New("未知任务状态；保留票据")
			}
		}
		if done {
			if len(out) > 32 {
				return nil, errors.New("任务产物数量超过 32 项上限，禁止静默裁剪")
			}
			return out, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.PollInterval):
		}
	}
}

func completedTaskURLs(task map[string]any, kind string) []string {
	key, field, outputKey := "generated_images", "image_url", "image_urls"
	if kind == "video" {
		key, field, outputKey = "generated_videos", "video_url", "video_urls"
	}
	var urls []string
	for _, item := range arr(task[key]) {
		if u := str(obj(item), field); u != "" {
			urls = append(urls, u)
		}
	}
	if list, ok := task["outputs"].([]any); ok {
		urls = append(urls, urlList(list)...)
	} else {
		urls = append(urls, urlList(obj(task["outputs"])[outputKey])...)
	}
	seen := map[string]bool{}
	unique := urls[:0]
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			unique = append(unique, u)
		}
	}
	return unique
}
func (c *Client) Download(ctx context.Context, u string, max int64) ([]byte, error) {
	return c.download.Download(ctx, u, max)
}
