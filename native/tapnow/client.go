// Package tapnow owns an independent application MCP connection. No CLI credentials are reused.
package tapnow

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/mediafetch"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const Endpoint = "https://mcp.tapnow.media/api/agent-gateway/mcp/workbuddy/mcp"

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}
type Service struct {
	Tool        string         `json:"tool"`
	Kind        string         `json:"kind"`
	Group       string         `json:"group"`
	Models      []string       `json:"models"`
	InputSchema map[string]any `json:"inputSchema"`
}
type Status struct {
	Fixture          bool      `json:"fixture,omitempty"`
	Connected        bool      `json:"connected"`
	Reason           string    `json:"reason,omitempty"`
	Services         []Service `json:"services"`
	VideoInputReason string    `json:"videoInputReason,omitempty"`
}
type account struct {
	Access   string `json:"access_token"`
	Refresh  string `json:"refresh_token"`
	ClientID string `json:"client_id"`
	TokenURL string `json:"token_url"`
	Expires  int64  `json:"expires"`
}
type flow struct {
	State, Verifier, ClientID, Redirect, TokenURL, Owner string
	At                                                   time.Time
}
type Client struct {
	mu                               sync.Mutex
	receiptMu                        sync.Mutex
	endpoint, dir, session, protocol string
	account                          account
	pending                          *flow
	tools                            []Tool
	http                             *http.Client
	PollImage, PollVideo, Timeout    time.Duration
}

func New(endpoint, dir string) *Client {
	if endpoint == "" {
		endpoint = Endpoint
	}
	c := &Client{endpoint: endpoint, dir: dir, http: &http.Client{Timeout: 180 * time.Second}, PollImage: 15 * time.Second, PollVideo: 30 * time.Second, Timeout: 20 * time.Minute}
	c.http.CheckRedirect = func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	if dir != "" {
		b, err := os.ReadFile(filepath.Join(dir, "account.json"))
		if err == nil {
			_ = json.Unmarshal(b, &c.account)
		}
	}
	return c
}
func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (c *Client) saveAccount() error {
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(c.account)
	p := filepath.Join(c.dir, "account.json")
	if err := os.WriteFile(p+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Status{Connected: c.account.Access != "" || c.fixture(), Services: []Service{}, VideoInputReason: "当前发现的 MCP schema 未提供视频输入角色；视频引用会保留但禁止提交"}
	s.Fixture = c.fixture()
	if !s.Connected {
		s.Reason = "由服务器管理员连接 TapNow 账号"
	}
	for _, t := range c.tools {
		kind, group := serviceKind(t.Name)
		if kind == "" {
			continue
		}
		models := []string{}
		props, _ := t.InputSchema["properties"].(map[string]any)
		m, _ := props["model"].(map[string]any)
		for _, v := range values(m["enum"]) {
			if x, ok := v.(string); ok {
				models = append(models, x)
			}
		}
		for k := range props {
			if ReferenceProperty(k, schema(props, k)) == "video" {
				s.VideoInputReason = ""
			}
		}
		s.Services = append(s.Services, Service{t.Name, kind, group, models, t.InputSchema})
	}
	return s
}
func serviceKind(name string) (string, string) {
	switch name {
	case "create_hero_image":
		return "image", "精品单图"
	case "create_product_shots":
		return "image", "商品与场景"
	case "create_deck_visuals":
		return "image", "成套视觉"
	case "create_hero_video":
		return "video", "动态影像"
	}
	return "", ""
}
func (c *Client) fixture() bool {
	u, _ := url.Parse(c.endpoint)
	return u != nil && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
}
func (c *Client) trusted(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil {
		return false
	}
	if c.fixture() {
		base, _ := url.Parse(c.endpoint)
		return u.Scheme == base.Scheme && u.Host == base.Host
	}
	return u.Scheme == "https" && (u.Hostname() == "mcp.tapnow.media" || u.Hostname() == "oauth.tapnow.media")
}
func (c *Client) metadata(ctx context.Context, raw string, out any) error {
	if !c.trusted(raw) {
		return errors.New("不可信的 OAuth 元数据地址")
	}
	r, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
	res, e := c.http.Do(r)
	if e != nil {
		return errors.New("无法获取 OAuth 元数据")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("OAuth 元数据不可用")
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func (c *Client) post(ctx context.Context, raw string, body any, out any) error {
	if !c.trusted(raw) {
		return errors.New("不可信的 OAuth 地址")
	}
	b, _ := json.Marshal(body)
	r, _ := http.NewRequestWithContext(ctx, "POST", raw, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	res, e := c.http.Do(r)
	if e != nil {
		return errors.New("OAuth 请求未完成")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("OAuth 请求失败 HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
func (c *Client) Start(ctx context.Context, redirect, owner string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, _ := http.NewRequestWithContext(ctx, "GET", c.endpoint, nil)
	res, e := c.http.Do(r)
	if e != nil {
		return "", errors.New("TapNow 连接失败")
	}
	res.Body.Close()
	challenge := res.Header.Get("WWW-Authenticate")
	metadataURL := ""
	for _, part := range strings.Split(challenge, ",") {
		part = strings.TrimSpace(part)
		if i := strings.Index(part, "resource_metadata="); i >= 0 {
			metadataURL = strings.Trim(part[i+len("resource_metadata="):], "\"")
		}
	}
	if metadataURL == "" {
		return "", errors.New("提供方未返回 OAuth protected-resource 元数据")
	}
	var resource struct {
		Servers []string `json:"authorization_servers"`
	}
	if e = c.metadata(ctx, metadataURL, &resource); e != nil {
		return "", e
	}
	if len(resource.Servers) == 0 {
		return "", errors.New("OAuth 授权服务器缺失")
	}
	var meta struct {
		Authorize string   `json:"authorization_endpoint"`
		Token     string   `json:"token_endpoint"`
		Register  string   `json:"registration_endpoint"`
		PKCE      []string `json:"code_challenge_methods_supported"`
	}
	if e = c.metadata(ctx, strings.TrimRight(resource.Servers[0], "/")+"/.well-known/oauth-authorization-server", &meta); e != nil {
		return "", e
	}
	if !c.trusted(meta.Authorize) || !c.trusted(meta.Token) {
		return "", errors.New("OAuth 端点不可信")
	}
	pkce := false
	for _, m := range meta.PKCE {
		pkce = pkce || m == "S256"
	}
	if !pkce {
		return "", errors.New("提供方缺少 PKCE S256")
	}
	var reg struct {
		ID string `json:"client_id"`
	}
	if e = c.post(ctx, meta.Register, map[string]any{"client_name": "Image Canvas", "redirect_uris": []string{redirect}, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}}, &reg); e != nil {
		return "", e
	}
	if reg.ID == "" {
		return "", errors.New("OAuth 注册缺少 client_id")
	}
	f := &flow{State: random(), Verifier: random(), ClientID: reg.ID, Redirect: redirect, TokenURL: meta.Token, Owner: owner, At: time.Now()}
	c.pending = f
	sum := sha256.Sum256([]byte(f.Verifier))
	u, _ := url.Parse(meta.Authorize)
	q := u.Query()
	q.Set("client_id", f.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("scope", "mcp.tools.read mcp.tools.invoke")
	q.Set("state", f.State)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("code_challenge_method", "S256")
	q.Set("resource", c.endpoint)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (c *Client) exchange(ctx context.Context, endpoint string, v url.Values) (account, error) {
	r, _ := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, e := c.http.Do(r)
	if e != nil {
		return account{}, errors.New("OAuth 令牌交换失败")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return account{}, fmt.Errorf("OAuth 令牌交换失败 HTTP %d", res.StatusCode)
	}
	var t struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&t); e != nil || t.Access == "" {
		return account{}, errors.New("OAuth 令牌响应无效")
	}
	return account{Access: t.Access, Refresh: t.Refresh, Expires: time.Now().Unix() + t.Expires}, nil
}
func (c *Client) Complete(ctx context.Context, state, code, owner string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.pending
	if f == nil || f.State != state || f.Owner != owner || time.Since(f.At) > 10*time.Minute || code == "" {
		return errors.New("OAuth state 无效或已过期")
	}
	c.pending = nil
	a, e := c.exchange(ctx, f.TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {f.Verifier}, "client_id": {f.ClientID}, "redirect_uri": {f.Redirect}, "resource": {c.endpoint}})
	if e != nil {
		return e
	}
	a.ClientID = f.ClientID
	a.TokenURL = f.TokenURL
	c.account = a
	return c.saveAccount()
}
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.account = account{}
	c.pending = nil
	c.tools = nil
	c.session = ""
	e := os.Remove(filepath.Join(c.dir, "account.json"))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}
func (c *Client) auth(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fixture() && c.account.Access == "" {
		return "", nil
	}
	a := c.account
	if a.Access == "" {
		return "", errors.New("TapNow 未连接，请管理员授权")
	}
	if time.Now().Unix()+30 >= a.Expires {
		if a.Refresh == "" {
			return "", errors.New("TapNow 授权已过期，请重新连接")
		}
		fresh, e := c.exchange(ctx, a.TokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {a.Refresh}, "client_id": {a.ClientID}, "resource": {c.endpoint}})
		if e != nil {
			return "", e
		}
		fresh.ClientID = a.ClientID
		fresh.TokenURL = a.TokenURL
		if fresh.Refresh == "" {
			fresh.Refresh = a.Refresh
		}
		c.account = fresh
		if e = c.saveAccount(); e != nil {
			return "", errors.New("保存 TapNow 授权失败")
		}
	}
	return c.account.Access, nil
}
func (c *Client) rpc(ctx context.Context, method string, params any, out any) error {
	token, e := c.auth(ctx)
	if e != nil {
		return e
	}
	body := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	notification := strings.HasPrefix(method, "notifications/")
	if !notification {
		body["id"] = random()
	}
	b, _ := json.Marshal(body)
	r, _ := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	c.mu.Lock()
	session, protocol := c.session, c.protocol
	c.mu.Unlock()
	if session != "" {
		r.Header.Set("Mcp-Session-Id", session)
	}
	if protocol != "" {
		r.Header.Set("MCP-Protocol-Version", protocol)
	}
	res, e := c.http.Do(r)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("TapNow 响应未确认；不会自动重复提交")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("TapNow HTTP %d；请检查连接，不会自动重试生成", res.StatusCode)
	}
	if session := res.Header.Get("Mcp-Session-Id"); session != "" {
		c.mu.Lock()
		c.session = session
		c.mu.Unlock()
	}
	if notification {
		return nil
	}
	var envelope struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(io.LimitReader(res.Body, 8<<20))
		scanner.Buffer(make([]byte, 4096), 4<<20)
		data := ""
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
			if line == "" && data != "" {
				var m map[string]any
				if json.Unmarshal([]byte(data), &m) == nil && m["id"] == body["id"] {
					e = json.Unmarshal([]byte(data), &envelope)
					found = true
					break
				}
				data = ""
			}
		}
		if !found {
			return errors.New("MCP SSE 缺少匹配的响应")
		}
	} else {
		e = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&envelope)
	}
	if e != nil || envelope.Error != nil || envelope.ID != body["id"] {
		return errors.New("MCP 返回协议错误")
	}
	return json.Unmarshal(envelope.Result, out)
}
func (c *Client) Discover(ctx context.Context) error {
	var init struct {
		Protocol string `json:"protocolVersion"`
	}
	if e := c.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "image-canvas", "version": "1"}}, &init); e != nil {
		return e
	}
	c.mu.Lock()
	c.protocol = init.Protocol
	c.mu.Unlock()
	if e := c.rpc(ctx, "notifications/initialized", map[string]any{}, nil); e != nil {
		return e
	}
	all := []Tool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		var list struct {
			Tools []Tool `json:"tools"`
			Next  string `json:"nextCursor"`
		}
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if e := c.rpc(ctx, "tools/list", params, &list); e != nil {
			return e
		}
		all = append(all, list.Tools...)
		if list.Next == "" {
			c.mu.Lock()
			c.tools = all
			c.mu.Unlock()
			return nil
		}
		if cursor == list.Next {
			return errors.New("MCP 分页游标重复")
		}
		cursor = list.Next
	}
	return errors.New("MCP 工具分页超限")
}
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	c.mu.Lock()
	var tool Tool
	for _, t := range c.tools {
		if t.Name == name {
			tool = t
		}
	}
	c.mu.Unlock()
	if tool.Name == "" {
		return nil, errors.New("MCP 未发现所需工具：" + name)
	}
	// Canonicalize Go integer/slice values to the same JSON types the discovered schema sees.
	raw, _ := json.Marshal(args)
	var canonical map[string]any
	if e := json.Unmarshal(raw, &canonical); e != nil {
		return nil, errors.New("MCP 参数无法编码")
	}
	if e := Validate(tool.InputSchema, canonical, "arguments"); e != nil {
		return nil, e
	}
	args = canonical
	var res struct {
		Error      bool           `json:"isError"`
		Structured map[string]any `json:"structuredContent"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if e := c.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &res); e != nil {
		return nil, e
	}
	if res.Error {
		return nil, errors.New("TapNow 工具执行失败（isError）；不会自动重试")
	}
	if res.Structured != nil {
		return res.Structured, nil
	}
	for _, b := range res.Content {
		if b.Type == "text" {
			var m map[string]any
			if json.Unmarshal([]byte(b.Text), &m) == nil {
				return m, nil
			}
		}
	}
	return nil, errors.New("TapNow 工具没有返回可解析的结构化结果")
}

// PublicURL validates media under this connection's installed origin policy.
func (c *Client) PublicURL(ctx context.Context, raw string) error {
	return mediafetch.ValidateURL(ctx, raw, c.mediaPolicy())
}
func (c *Client) mediaPolicy() mediafetch.Policy {
	if c.fixture() {
		u, _ := url.Parse(c.endpoint)
		return mediafetch.Policy{TrustedOrigin: u.Scheme + "://" + u.Host}
	}
	return mediafetch.Policy{}
}
func (c *Client) Download(ctx context.Context, raw string, max int64) ([]byte, error) {
	return mediafetch.Download(ctx, raw, max, c.mediaPolicy())
}
