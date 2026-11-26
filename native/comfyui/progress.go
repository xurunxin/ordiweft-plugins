package comfyui

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// Progress contains only this job's safe presentation state, never queue graphs.
type Progress struct {
	Stage         string  `json:"stage"`
	QueuePosition int     `json:"queuePosition,omitempty"`
	Ahead         int     `json:"ahead,omitempty"`
	NodeID        string  `json:"nodeId,omitempty"`
	NodeLabel     string  `json:"nodeLabel,omitempty"`
	Value         float64 `json:"value,omitempty"`
	Max           float64 `json:"max,omitempty"`
	Live          bool    `json:"live"`
}
type progressTracker struct {
	mu    sync.Mutex
	id    string
	state Progress
	nodes Graph
}

func (p *Plugin) watchProgress(ctx context.Context, clientID string) (*progressTracker, func()) {
	t := &progressTracker{state: Progress{Stage: "submitted"}, nodes: p.Definition.Graph}
	u, _ := url.Parse(p.Definition.BaseURL)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws"
	u.RawQuery = url.Values{"clientId": {clientID}}.Encode()
	cfg, err := websocket.NewConfig(u.String(), p.Definition.BaseURL)
	if err != nil {
		return t, func() {}
	}
	if p.Definition.Access != nil {
		cfg.Header, err = p.Definition.Access.headers(p.Definition.BaseURL)
		if err != nil {
			return t, func() {}
		}
	}
	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := cfg.DialContext(dialCtx)
	if err != nil {
		return t, func() {}
	}
	conn.MaxPayloadBytes = 1 << 20
	t.state.Live = true
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	go func() {
		defer func() { t.mu.Lock(); t.state.Live = false; t.state.Value = 0; t.state.Max = 0; t.mu.Unlock() }()
		for {
			var raw []byte
			if websocket.Message.Receive(conn, &raw) != nil {
				return
			}
			t.apply(raw)
		}
	}()
	return t, func() { stop(); _ = conn.Close() }
}
func (t *progressTracker) apply(raw []byte) {
	var event struct {
		Type string `json:"type"`
		Data struct {
			PromptID string  `json:"prompt_id"`
			Node     *string `json:"node"`
			Value    float64 `json:"value"`
			Max      float64 `json:"max"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.id == "" || event.Data.PromptID != t.id {
		return
	}
	switch event.Type {
	case "execution_start":
		t.state.Stage = "running"
		t.state.QueuePosition = 0
		t.state.Ahead = 0
	case "executing":
		if event.Data.Node == nil {
			return
		}
		n, ok := t.nodes[*event.Data.Node]
		if !ok {
			return
		}
		t.state.Stage = "running"
		t.state.NodeID = *event.Data.Node
		t.state.NodeLabel = n.Meta.Title
		if t.state.NodeLabel == "" {
			t.state.NodeLabel = n.Class
		}
		if len([]rune(t.state.NodeLabel)) > 100 {
			t.state.NodeLabel = string([]rune(t.state.NodeLabel)[:100])
		}
		t.state.Value = 0
		t.state.Max = 0
		t.state.QueuePosition = 0
		t.state.Ahead = 0
	case "progress":
		if event.Data.Max <= 0 || event.Data.Max > 1e9 || event.Data.Value < 0 || event.Data.Value > event.Data.Max {
			return
		}
		t.state.Stage = "running"
		t.state.Value = event.Data.Value
		t.state.Max = event.Data.Max
	}
}
func (t *progressTracker) snapshot() Progress { t.mu.Lock(); defer t.mu.Unlock(); return t.state }

func (p *Plugin) queueProgress(ctx context.Context, id string, t *progressTracker) bool {
	raw, err := p.request(ctx, "GET", "/queue", "", nil, 8<<20)
	if err != nil {
		return false
	}
	var queue struct {
		Running [][]json.RawMessage `json:"queue_running"`
		Pending [][]json.RawMessage `json:"queue_pending"`
	}
	if json.Unmarshal(raw, &queue) != nil {
		return false
	}
	match := func(row []json.RawMessage) bool {
		var candidate string
		return len(row) > 1 && json.Unmarshal(row[1], &candidate) == nil && candidate == id
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, row := range queue.Running {
		if match(row) {
			t.state.Stage = "running"
			t.state.QueuePosition = 0
			t.state.Ahead = 0
			return true
		}
	}
	sort.SliceStable(queue.Pending, func(i, j int) bool {
		var a, b float64
		if len(queue.Pending[i]) > 0 {
			_ = json.Unmarshal(queue.Pending[i][0], &a)
		}
		if len(queue.Pending[j]) > 0 {
			_ = json.Unmarshal(queue.Pending[j][0], &b)
		}
		return a < b
	})
	for i, row := range queue.Pending {
		if match(row) {
			t.state.Stage = "queued"
			t.state.QueuePosition = i + 1
			t.state.Ahead = len(queue.Running) + i
			t.state.NodeID = ""
			t.state.NodeLabel = ""
			t.state.Value = 0
			t.state.Max = 0
			return true
		}
	}
	return false
}
