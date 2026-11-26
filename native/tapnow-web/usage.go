package tapnowweb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type scalar string

func (s *scalar) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, (*string)(s))
	}
	if string(b) == "null" {
		*s = ""
		return nil
	}
	var n json.Number
	if e := json.Unmarshal(b, &n); e != nil {
		return e
	}
	*s = scalar(n.String())
	return nil
}

type rawTransaction struct {
	TaskID      string `json:"task_id"`
	ID          scalar `json:"id"`
	OldID       scalar `json:"transaction_id"`
	Time        scalar `json:"transactionTime"`
	OldTime     scalar `json:"created_at"`
	Contact     string `json:"userContact"`
	Description string `json:"description"`
	Amount      scalar `json:"amount"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Balance     scalar `json:"balanceAfter"`
}
type Transaction struct {
	AmountDecimal string   `json:"amountDecimal,omitempty"`
	TaskID        string   `json:"taskId,omitempty"`
	ID            string   `json:"id"`
	At            int64    `json:"at"`
	Operator      string   `json:"operator"`
	Model         string   `json:"model"`
	Kind          string   `json:"kind"`
	Category      string   `json:"category"`
	Amount        float64  `json:"amount"`
	Type          string   `json:"type"`
	Status        string   `json:"status"`
	Description   string   `json:"description"`
	Usage         *float64 `json:"usage"`
	Balance       *float64 `json:"balance"`
}
type Aggregate struct {
	Name      string  `json:"name"`
	Consumed  float64 `json:"consumed"`
	Deposited float64 `json:"deposited"`
	Count     int     `json:"count"`
}
type Totals struct {
	Consumed  float64 `json:"consumed"`
	Deposited float64 `json:"deposited"`
	Net       float64 `json:"net"`
}
type UsageSummary struct {
	Scope        string      `json:"scope"`
	Count        int         `json:"count"`
	Main         Totals      `json:"main"`
	Quota        Totals      `json:"quota"`
	ImageCount   float64     `json:"imageCount"`
	VideoSeconds float64     `json:"videoSeconds"`
	ByModel      []Aggregate `json:"byModel"`
	ByOperator   []Aggregate `json:"byOperator"`
	ByDay        []Aggregate `json:"byDay"`
}
type UsagePage struct {
	Page         int           `json:"page"`
	PageSize     int           `json:"pageSize"`
	Total        int           `json:"total"`
	Transactions []Transaction `json:"transactions"`
	Summary      UsageSummary  `json:"summary"`
	Complete     bool          `json:"complete"`
	SyncedAt     int64         `json:"syncedAt"`
}
type usageCache struct {
	Transactions []Transaction `json:"transactions"`
	Total        int           `json:"total"`
	Complete     bool          `json:"complete"`
	SyncedAt     int64         `json:"syncedAt"`
}
type UsageFilter struct{ From, To, Model, Operator string }

var quotedModel = regexp.MustCompile(`'([^']+)'`)
var agentModel = regexp.MustCompile(`^TapNow Agent\s+-\s+(.+?)\s+biz metric\b`)
var usageValue = regexp.MustCompile(`\(Usage:\s*(\d+(?:\.\d+)?)\s*\)|Metrics:\s*\{[^}]*?"consumed_points"\s*:\s*(\d+(?:\.\d+)?)`)
var videoModel = regexp.MustCompile(`(?i)(\bto\s+Video\b|i2v|r2v|t2v|flf2v|v2v|\bVideo\b)`)
var imageModel = regexp.MustCompile(`(?i)\bImage\b`)

func modelName(s string) string {
	if m := quotedModel.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	for _, label := range []string{"On-demand charge:", "On-demand business-metric charge:", "Hybrid charge:", "TapNow On-demand charge:"} {
		if strings.HasPrefix(s, label) {
			s = strings.TrimSpace(strings.TrimPrefix(s, label))
			for _, marker := range []string{"(Usage:", "(Metrics:", "(Tapies payment:"} {
				if i := strings.Index(s, marker); i >= 0 {
					s = s[:i]
				}
			}
			return strings.TrimSpace(s)
		}
	}
	if m := agentModel.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return "未知模型"
}
func transactionNumber(s scalar) (float64, error) {
	v, e := strconv.ParseFloat(string(s), 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("交易金额格式无效")
	}
	return v, nil
}
func normalizeTransaction(r rawTransaction) (Transaction, error) {
	id := string(r.ID)
	if id == "" {
		id = string(r.OldID)
	}
	if id == "" {
		return Transaction{}, errors.New("交易缺少 ID")
	}
	at := r.Time
	if at == "" {
		at = r.OldTime
	}
	dt, e := time.Parse(time.RFC3339Nano, string(at))
	var ms int64
	if e == nil {
		ms = dt.UnixMilli()
	} else {
		n, err := transactionNumber(at)
		if err != nil {
			return Transaction{}, errors.New("交易时间无效")
		}
		if n < 1e12 {
			n *= 1000
		}
		if n < 0 || n > 253402300799999 {
			return Transaction{}, errors.New("交易时间无效")
		}
		ms = int64(n)
	}
	amount, e := transactionNumber(r.Amount)
	if e != nil || math.Abs(amount) > 1e12 {
		return Transaction{}, errors.New("交易金额无效")
	}
	exact, ok := new(big.Rat).SetString(string(r.Amount))
	if !ok {
		return Transaction{}, errors.New("交易金额格式无效")
	}
	t := Transaction{AmountDecimal: exact.FloatString(6), TaskID: r.TaskID, ID: id, At: ms, Operator: r.Contact, Model: modelName(r.Description), Amount: amount, Type: r.Type, Status: r.Status, Description: r.Description, Kind: "other", Category: "direct"}
	if strings.HasPrefix(r.Description, "TapNow Agent") {
		t.Category = "agent"
	}
	if videoModel.MatchString(t.Model) {
		t.Kind = "video"
	} else if imageModel.MatchString(t.Model) {
		t.Kind = "image"
	}
	if m := usageValue.FindStringSubmatch(r.Description); len(m) > 0 {
		s := m[1]
		if s == "" {
			s = m[2]
		}
		v, err := strconv.ParseFloat(s, 64)
		if err == nil && !math.IsInf(v, 0) && v <= 1e12 {
			t.Usage = &v
		}
	}
	if r.Balance != "" {
		v, e := transactionNumber(r.Balance)
		if e != nil || math.Abs(v) > 1e12 {
			return t, errors.New("交易余额无效")
		}
		t.Balance = &v
	}
	return t, nil
}
func summarize(rows []Transaction, scope string) UsageSummary {
	s := UsageSummary{Scope: scope, Count: len(rows), ByModel: []Aggregate{}, ByOperator: []Aggregate{}, ByDay: []Aggregate{}}
	models, operators, days := map[string]*Aggregate{}, map[string]*Aggregate{}, map[string]*Aggregate{}
	for _, t := range rows {
		if t.Status != "COMPLETED" && t.Status != "TAPIES_TRANSACTION_STATUS_COMPLETED" {
			continue
		}
		consume, deposit := strings.Contains(t.Type, "CONSUME"), strings.Contains(t.Type, "DEPOSIT")
		amount := math.Abs(t.Amount)
		total := &s.Main
		if strings.Contains(t.Type, "QUOTA") {
			total = &s.Quota
		}
		if consume {
			total.Consumed += amount
		}
		if deposit {
			total.Deposited += amount
		}
		if consume && t.Usage != nil {
			if t.Kind == "image" {
				s.ImageCount += *t.Usage
			}
			if t.Kind == "video" {
				s.VideoSeconds += *t.Usage
			}
		}
		if total == &s.Quota {
			continue
		}
		add := func(m map[string]*Aggregate, key string) {
			a := m[key]
			if a == nil {
				a = &Aggregate{Name: key}
				m[key] = a
			}
			a.Count++
			if consume {
				a.Consumed += amount
			}
			if deposit {
				a.Deposited += amount
			}
		}
		if consume {
			add(models, t.Model)
		}
		add(operators, t.Operator)
		add(days, time.UnixMilli(t.At).UTC().Format("2006-01-02"))
	}
	s.Main.Net = s.Main.Deposited - s.Main.Consumed
	s.Quota.Net = s.Quota.Deposited - s.Quota.Consumed
	for _, a := range models {
		s.ByModel = append(s.ByModel, *a)
	}
	for _, a := range operators {
		s.ByOperator = append(s.ByOperator, *a)
	}
	for _, a := range days {
		s.ByDay = append(s.ByDay, *a)
	}
	sort.Slice(s.ByModel, func(i, j int) bool { return s.ByModel[i].Consumed > s.ByModel[j].Consumed })
	sort.Slice(s.ByOperator, func(i, j int) bool { return s.ByOperator[i].Consumed > s.ByOperator[j].Consumed })
	sort.Slice(s.ByDay, func(i, j int) bool { return s.ByDay[i].Name < s.ByDay[j].Name })
	return s
}
func (c *Client) usageRemote(ctx context.Context, page, size int) (UsagePage, error) {
	p := UsagePage{Page: page, PageSize: size, Transactions: []Transaction{}}
	if page < 1 || page > 200 || size < 1 || size > 100 {
		return p, errors.New("分页范围无效")
	}
	var data struct {
		Rows  []rawTransaction `json:"transactions"`
		Total scalar           `json:"total"`
	}
	if e := c.request(ctx, "GET", fmt.Sprintf("/api/billing/v2/wallet/transactions?page=%d&pageSize=%d", page, size), nil, &data); e != nil {
		return p, e
	}
	total, e := strconv.Atoi(string(data.Total))
	if e != nil || total < 0 || len(data.Rows) > size {
		return p, errors.New("交易分页格式无效")
	}
	p.Total = total
	for _, r := range data.Rows {
		t, e := normalizeTransaction(r)
		if e != nil {
			return p, e
		}
		p.Transactions = append(p.Transactions, t)
	}
	p.Summary = summarize(p.Transactions, "当前页")
	return p, nil
}
func (c *Client) usageKey(ctx context.Context) (string, error) {
	i, e := c.Identity(ctx)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256([]byte(i.User + "\x00" + i.Org))
	return "usage-" + hex.EncodeToString(h[:16]) + ".json", nil
}
func (c *Client) SyncUsage(ctx context.Context) (UsagePage, error) {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	c.mu.Lock()
	revision := c.accountRevision
	c.mu.Unlock()
	key, e := c.usageKey(ctx)
	if e != nil {
		return UsagePage{}, e
	}
	cache := usageCache{Transactions: []Transaction{}, SyncedAt: time.Now().UnixMilli()}
	seen := map[string]bool{}
	for page := 1; page <= 200; page++ {
		p, e := c.usageRemote(ctx, page, 100)
		if e != nil {
			return UsagePage{}, e
		}
		if page == 1 {
			cache.Total = p.Total
		} else if p.Total != cache.Total {
			return UsagePage{}, errors.New("同步期间交易总数变化，请再次同步")
		}
		for _, t := range p.Transactions {
			if seen[t.ID] {
				return UsagePage{}, errors.New("交易分页出现重复，请再次同步")
			}
			seen[t.ID] = true
			cache.Transactions = append(cache.Transactions, t)
		}
		if len(cache.Transactions) >= cache.Total {
			cache.Complete = len(cache.Transactions) == cache.Total
			break
		}
		if len(p.Transactions) < 100 {
			break
		}
	}
	// Check the identity again before publishing so a concurrent login switch can't mix accounts.
	again, e := c.usageKey(ctx)
	c.mu.Lock()
	changed := c.accountRevision != revision
	c.mu.Unlock()
	if e != nil || again != key || changed {
		return UsagePage{}, errors.New("同步期间登录身份发生变化")
	}
	b, err := json.Marshal(cache)
	if err != nil || len(b) > 32<<20 || c.save(key, b) != nil {
		return UsagePage{}, errors.New("用量缓存保存失败")
	}
	return cache.page(1, 50, UsageFilter{})
}
func (c *Client) Usage(ctx context.Context, page, size int, cached bool, filter UsageFilter) (UsagePage, error) {
	if !cached {
		return c.usageRemote(ctx, page, size)
	}
	key, e := c.usageKey(ctx)
	if e != nil {
		return UsagePage{}, e
	}
	b, e := os.ReadFile(filepath.Join(c.dir, key))
	if e != nil {
		return UsagePage{}, errors.New("尚无用量缓存，请先同步")
	}
	var cache usageCache
	if len(b) > 32<<20 || json.Unmarshal(b, &cache) != nil {
		return UsagePage{}, errors.New("用量缓存无效")
	}
	return cache.page(page, size, filter)
}

// CachedBillingSnapshot reads the already synchronized invoice, without a network request or generation.
func (c *Client) CachedBillingSnapshot(ctx context.Context) ([]Transaction, int64, bool, error) {
	key, err := c.usageKey(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	b, err := os.ReadFile(filepath.Join(c.dir, key))
	if err != nil {
		return nil, 0, false, errors.New("用量缓存不可用")
	}
	var cache usageCache
	if len(b) > 32<<20 || json.Unmarshal(b, &cache) != nil {
		return nil, 0, false, errors.New("用量缓存无效")
	}
	return cache.Transactions, cache.SyncedAt, cache.Complete, nil
}
func (cache usageCache) page(page, size int, f UsageFilter) (UsagePage, error) {
	if page < 1 || page > 20000 || size < 1 || size > 100 {
		return UsagePage{}, errors.New("分页范围无效")
	}
	var start, end int64
	for _, v := range []struct {
		s    string
		dest *int64
	}{{f.From, &start}, {f.To, &end}} {
		if v.s != "" {
			t, e := time.Parse("2006-01-02", v.s)
			if e != nil {
				return UsagePage{}, errors.New("日期格式无效")
			}
			*v.dest = t.UnixMilli()
		}
	}
	if f.To != "" {
		end += 86400000
	}
	if f.From != "" && f.To != "" && start >= end {
		return UsagePage{}, errors.New("日期范围无效")
	}
	rows := []Transaction{}
	for _, t := range cache.Transactions {
		if (f.From != "" && t.At < start) || (f.To != "" && t.At >= end) || (f.Model != "" && t.Model != f.Model) || (f.Operator != "" && t.Operator != f.Operator) {
			continue
		}
		rows = append(rows, t)
	}
	p := UsagePage{Page: page, PageSize: size, Total: len(rows), Transactions: []Transaction{}, Complete: cache.Complete, SyncedAt: cache.SyncedAt, Summary: summarize(rows, "筛选后的全部缓存")}
	lo := (page - 1) * size
	if lo < len(rows) {
		hi := lo + size
		if hi > len(rows) {
			hi = len(rows)
		}
		p.Transactions = rows[lo:hi]
	}
	return p, nil
}
