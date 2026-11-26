package tapnow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type Ticket struct {
	ProjectID  string   `json:"project_id"`
	NodeIDs    []string `json:"node_ids"`
	ProjectURL string   `json:"project_url,omitempty"`
}
type Media struct {
	NodeID string
	URL    string
	Kind   string
}
type Receipt struct {
	Owner  string `json:"owner"`
	Phase  string `json:"phase"`
	Ticket Ticket `json:"ticket"`
	Error  string `json:"error,omitempty"`
}

var receiptID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func (c *Client) Receipt(id string) (Receipt, bool) {
	c.receiptMu.Lock()
	defer c.receiptMu.Unlock()
	if !receiptID.MatchString(id) {
		return Receipt{}, false
	}
	b, e := os.ReadFile(filepath.Join(c.dir, "tickets", id+".json"))
	var r Receipt
	if e != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, true
}
func (c *Client) SaveReceipt(id string, r Receipt) error {
	// Windows cannot replace a receipt while another goroutine has it open.
	// Serialize local reads and replacements, without retrying provider calls.
	c.receiptMu.Lock()
	defer c.receiptMu.Unlock()
	if !receiptID.MatchString(id) {
		return errors.New("任务键无效")
	}
	dir := filepath.Join(c.dir, "tickets")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, _ := json.Marshal(r)
	p := filepath.Join(dir, id+".json")
	if e := os.WriteFile(p+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func (c *Client) Produce(ctx context.Context, id, owner string, r Request, checkpoint func(Ticket)) ([]Media, Ticket, error) {
	if old, ok := c.Receipt(id); ok {
		return nil, old.Ticket, errors.New("该任务已经提交或状态不明确，禁止再次付费提交；请核实保留票据")
	}
	queueContext := ctx
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	args, e := c.Arguments(ctx, r)
	if e != nil {
		return nil, Ticket{}, e
	}
	receipt := Receipt{Owner: owner, Phase: "submitting"}
	if e = c.SaveReceipt(id, receipt); e != nil {
		return nil, Ticket{}, errors.New("无法保存提交记录，未提交制作")
	}
	response, e := c.Call(ctx, r.Tool, args)
	if e != nil {
		receipt.Phase = "unknown"
		if ctx.Err() == context.Canceled {
			receipt.Phase = "canceled"
		}
		receipt.Error = "提交响应未确认，禁止自动重发"
		_ = c.SaveReceipt(id, receipt)
		return nil, Ticket{}, e
	}
	ticket := Ticket{ProjectID: text(response, "project_id"), ProjectURL: text(response, "project_url"), NodeIDs: []string{}}
	for _, v := range values(response["node_ids"]) {
		if s, ok := v.(string); ok && s != "" {
			ticket.NodeIDs = append(ticket.NodeIDs, s)
		}
	}
	if ticket.ProjectID == "" || len(ticket.NodeIDs) == 0 || len(ticket.NodeIDs) > 20 {
		receipt.Phase = "unknown"
		_ = c.SaveReceipt(id, receipt)
		return nil, ticket, errors.New("制作响应缺少完整 project_id/node_ids；禁止重复提交")
	}
	receipt.Ticket = ticket
	receipt.Phase = "pending"
	if e = c.SaveReceipt(id, receipt); e != nil {
		return nil, ticket, errors.New("票据保存失败，禁止重复提交")
	}
	checkpoint(ticket)
	ctx = queueContext
	cancel()
	media, e := c.Poll(ctx, ticket, r.Kind)
	if e != nil {
		receipt.Phase = "failed"
		if ctx.Err() == context.Canceled {
			receipt.Phase = "canceled"
		}
		receipt.Error = "未取得完整产物；保留票据，不自动重新制作"
	} else {
		receipt.Phase = "done"
	}
	_ = c.SaveReceipt(id, receipt)
	return media, ticket, e
}
func (c *Client) Poll(ctx context.Context, t Ticket, kind string) ([]Media, error) {
	interval := c.PollImage
	if kind == "video" {
		interval = c.PollVideo
	}
	for {
		result, e := c.Call(ctx, "get_production_result", map[string]any{"project_id": t.ProjectID, "node_ids": t.NodeIDs})
		if e != nil {
			return nil, e
		}
		status := text(result, "status")
		if status == "failed" || status == "not_found" || status == "blocked" || len(values(result["failed_node_ids"])) > 0 {
			return nil, fmt.Errorf("TapNow 任务终止：%s", status)
		}
		found := map[string]Media{}
		for _, item := range values(result["items"]) {
			m, _ := item.(map[string]any)
			state := text(m, "status")
			if state == "failed" || state == "not_found" {
				return nil, fmt.Errorf("TapNow 子任务终止：%s", state)
			}
			if state != "done" {
				continue
			}
			id := text(m, "node_id")
			url := text(m, "media_url")
			mediaKind := text(m, "media_type")
			if url == "" {
				url = text(m, "image_url")
				if url != "" {
					mediaKind = "image"
				}
			}
			if id == "" || url == "" || mediaKind != kind {
				return nil, errors.New("完成项缺少 node_id/media_url 或媒体类型不匹配")
			}
			if _, exists := found[id]; exists {
				return nil, errors.New("产物节点 ID 重复，不能明确关联版本")
			}
			found[id] = Media{NodeID: id, URL: url, Kind: mediaKind}
		}
		all := []Media{}
		for _, id := range t.NodeIDs {
			if m, ok := found[id]; ok {
				all = append(all, m)
			}
		}
		if len(all) == len(t.NodeIDs) {
			return all, nil
		}
		if status == "done" {
			return nil, errors.New("任务声明完成但缺少票据对应的产物")
		}
		if status != "generating" && status != "pending" && status != "queued" && status != "running" {
			return nil, errors.New("提供方返回未知任务状态，停止轮询")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
