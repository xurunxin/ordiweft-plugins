package tapnowweb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Wallet struct {
	Total      float64 `json:"total"`
	Available  float64 `json:"available"`
	OnHold     float64 `json:"onHold"`
	ObservedAt int64   `json:"observedAt"`
}

func (c *Client) Wallet(ctx context.Context) (Wallet, error) {
	return c.walletForAccount(ctx, nil)
}

func (c *Client) walletForAccount(ctx context.Context, revision *uint64) (Wallet, error) {
	var d struct {
		Total     scalar `json:"total"`
		Available scalar `json:"available"`
		OnHold    scalar `json:"onHold"`
	}
	var w Wallet
	request := c.request
	if revision != nil {
		request = func(ctx context.Context, method, path string, body, out any) error {
			return c.requestForAccount(ctx, *revision, method, path, body, out)
		}
	}
	if e := request(ctx, "GET", "/api/billing/v2/wallet/balance", nil, &d); e != nil {
		return w, e
	}
	var e error
	w.Total, e = transactionNumber(d.Total)
	if e != nil {
		return w, e
	}
	w.Available, e = transactionNumber(d.Available)
	if e != nil {
		return w, e
	}
	w.OnHold, e = transactionNumber(d.OnHold)
	if e != nil {
		return w, e
	}
	w.ObservedAt = time.Now().UnixMilli()
	return w, nil
}

type Quote struct {
	EstimatedCost float64 `json:"estimatedCost"`
	Wallet        Wallet  `json:"wallet"`
	ObservedAt    int64   `json:"observedAt"`
}

func (c *Client) Quote(ctx context.Context, r Request, resolve Resolve) (Quote, error) {
	return c.quote(ctx, r, resolve, false, false)
}

// PreviewQuote caches only interactive previews; approval and submission use fresh quotes.
func (c *Client) PreviewQuote(ctx context.Context, r Request, resolve Resolve, refresh bool) (Quote, error) {
	return c.quote(ctx, r, resolve, true, refresh)
}

type previewQuote struct {
	cost float64
	at   time.Time
}

func (c *Client) quote(ctx context.Context, r Request, resolve Resolve, preview, refresh bool) (Quote, error) {
	var q Quote
	// Quote only inspects bytes. It never uploads; generation still requires the saved consent.
	r.UploadConsent = true
	if !isDraftFinal(r) && strings.TrimSpace(r.Prompt) == "" {
		r.Prompt = "Cost preview"
	}
	body, mode, e := c.Prepare(r)
	if e != nil {
		return q, e
	}
	var durations referenceDurations
	uploaded := map[string]string{}
	var identity Identity
	if len(r.References) > 0 {
		identity, e = c.Identity(ctx)
		if e != nil {
			return q, e
		}
	}
	for _, ref := range r.References {
		if strings.HasPrefix(ref.URL, "/files/") {
			if resolve == nil {
				return q, errors.New("缺少素材解析器")
			}
			m, e := resolve(ctx, ref.URL)
			if e != nil {
				return q, e
			}
			m, e = inspectMaterial(m, ref.Kind)
			if e != nil {
				return q, e
			}
			if e = durations.add(mode, ref.Kind, m.Duration); e != nil {
				return q, e
			}
			if u := c.cachedURL(identity, m); u != "" {
				uploaded[ref.URL] = u
			} else if ref.Kind == "video" || ref.Kind == "audio" {
				return q, errors.New("请先授权并准备视频/音频引用素材；上传不提交生成任务")
			}
		}
	}
	for k, v := range body {
		body[k] = replaceURLs(v, uploaded)
	}
	if e = durations.validateTotals(mode); e != nil {
		return q, e
	}
	var revision *uint64
	if isDraftFinal(r) {
		if r.Draft == nil {
			return q, errDraftUnavailable
		}
		revision = &r.Draft.revision
		body = r.Draft.pricing.estimateBody(body)
	} else {
		body = estimateWithDuration(r.Kind, body, durations.estimateDuration(mode))
	}
	type walletResult struct {
		wallet Wallet
		err    error
	}
	wallet := make(chan walletResult, 1)
	walletCtx, cancelWallet := context.WithCancel(ctx)
	defer cancelWallet()
	go func() {
		w, err := c.walletForAccount(walletCtx, revision)
		wallet <- walletResult{w, err}
	}()
	if preview {
		q.EstimatedCost, q.ObservedAt, e = c.previewEstimate(ctx, r.Kind, body, refresh, revision)
	} else {
		q.EstimatedCost, e = c.estimateForAccount(ctx, r.Kind, body, revision)
		q.ObservedAt = time.Now().UnixMilli()
	}
	if e != nil {
		cancelWallet()
		<-wallet
		return q, e
	}
	result := <-wallet
	q.Wallet = result.wallet
	return q, result.err
}

func (c *Client) previewEstimate(ctx context.Context, kind string, body map[string]any, refresh bool, expected *uint64) (float64, int64, error) {
	encoded, err := json.Marshal(EstimateBody(kind, body))
	if err != nil {
		return 0, 0, err
	}
	c.mu.Lock()
	revision := c.accountRevision
	c.mu.Unlock()
	if expected != nil && *expected != revision {
		return 0, 0, errDraftUnavailable
	}
	hash := sha256.Sum256(encoded)
	key := strconv.FormatUint(revision, 10) + ":" + hex.EncodeToString(hash[:])
	c.quoteMu.Lock()
	now := time.Now()
	entry, ok := c.previewQuotes[key]
	c.quoteMu.Unlock()
	if ok && !refresh && now.Sub(entry.at) < 30*time.Second {
		return entry.cost, entry.at.UnixMilli(), nil
	}
	started := time.Now()
	cost, err := c.estimateForAccount(ctx, kind, body, expected)
	if err != nil {
		return 0, 0, err
	}
	c.mu.Lock()
	unchanged := c.accountRevision == revision
	c.mu.Unlock()
	if !unchanged {
		return 0, 0, errors.New("TapNow 登录身份已变更，请重新估价")
	}
	c.quoteMu.Lock()
	defer c.quoteMu.Unlock()
	if current, exists := c.previewQuotes[key]; exists && current.at.After(started) {
		return cost, time.Now().UnixMilli(), nil
	}
	if c.previewQuotes == nil || len(c.previewQuotes) >= 128 {
		c.previewQuotes = make(map[string]previewQuote)
	}
	now = time.Now()
	c.previewQuotes[key] = previewQuote{cost: cost, at: now}
	return cost, now.UnixMilli(), nil
}

// The website estimates edits using measured input duration, but generation preserves -1.
func (c *Client) estimateVerified(ctx context.Context, kind string, body map[string]any, duration float64) (float64, error) {
	return c.Estimate(ctx, kind, estimateWithDuration(kind, body, duration))
}

func estimateWithDuration(kind string, body map[string]any, duration float64) map[string]any {
	if kind == "video" && number(body["duration"]) <= 0 && duration > 0 {
		return merge(body, map[string]any{"duration": math.Ceil(duration)})
	}
	return body
}

func materialKey(i Identity, m Material) string {
	sum := sha256.New()
	sum.Write([]byte(i.User + "/" + i.Org + "/"))
	sum.Write(m.Bytes)
	return hex.EncodeToString(sum.Sum(nil))
}
func (c *Client) cachedURL(i Identity, m Material) string {
	b, e := os.ReadFile(filepath.Join(c.dir, "uploaded-"+materialKey(i, m)+".json"))
	if e != nil {
		return ""
	}
	var record struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(b, &record) != nil {
		return ""
	}
	if tapnowUploadID(record.URL) == "" {
		return ""
	}
	return record.URL
}
func (c *Client) cachedUpload(ctx context.Context, i Identity, m Material) (string, error) {
	c.uploadMu.Lock()
	defer c.uploadMu.Unlock()
	if u := c.cachedURL(i, m); u != "" {
		return u, nil
	}
	u, e := c.Upload(ctx, m)
	if e != nil {
		return "", e
	}
	b, _ := json.Marshal(map[string]string{"url": u})
	if e = c.save("uploaded-"+materialKey(i, m)+".json", b); e != nil {
		return "", errors.New("素材已上传，但缓存保存失败")
	}
	return u, nil
}
func (c *Client) PrepareReferences(ctx context.Context, r Request, resolve Resolve) error {
	if strings.TrimSpace(r.Prompt) == "" {
		r.Prompt = "Cost preview"
	}
	_, mode, e := c.Prepare(r)
	if e != nil {
		return e
	}
	if !r.UploadConsent {
		return errors.New("请先授权上传引用素材")
	}
	i, e := c.Identity(ctx)
	if e != nil {
		return e
	}
	materials := []Material{}
	var durations referenceDurations
	for _, ref := range r.References {
		if !strings.HasPrefix(ref.URL, "/files/") {
			continue
		}
		if resolve == nil {
			return errors.New("缺少素材解析器")
		}
		m, e := resolve(ctx, ref.URL)
		if e != nil {
			return e
		}
		m, e = inspectMaterial(m, ref.Kind)
		if e != nil || int64(len(m.Bytes)) > c.MaxReferenceBytes {
			return errors.New("素材实际格式或大小不支持")
		}
		if e = durations.add(mode, ref.Kind, m.Duration); e != nil {
			return e
		}
		materials = append(materials, m)
	}
	if e = durations.validateTotals(mode); e != nil {
		return e
	}
	for _, m := range materials {
		if _, e = c.cachedUpload(ctx, i, m); e != nil {
			return e
		}
	}
	return nil
}
