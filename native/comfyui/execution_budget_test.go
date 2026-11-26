package comfyui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestExpectedVideoWaitIncludesColdStartAndSlowSeconds(t *testing.T) {
	d := Definition{Graph: Graph{}, Manifest: aigc.Manifest{Models: []aigc.Model{{ID: "workflow", Capability: "video.generate", Fields: map[string]aigc.Field{"seconds": {Type: "number", Control: "duration", Default: 6.0}}}}}}
	p := New(d)
	for _, tt := range []struct {
		seconds float64
		want    time.Duration
	}{{1, 30 * time.Minute}, {6, 40 * time.Minute}, {15, 85 * time.Minute}, {100000, 6 * time.Hour}} {
		raw, _ := json.Marshal(aigc.Generation{Model: "workflow", Parameters: map[string]any{"seconds": tt.seconds}})
		if got := p.ExpectedWait(aigc.Request{Payload: raw}); got != tt.want {
			t.Fatal(tt.seconds, got, tt.want)
		}
	}
}

func TestCollectWaitsThroughBusyQueriesAndNeverSubmits(t *testing.T) {
	var reads, posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			posts.Add(1)
			t.Error("read-only collection wrote upstream")
		}
		n := reads.Add(1)
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		if n < 4 {
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `{"original":{"status":{"status_str":"error","completed":false}}}`)
	}))
	defer srv.Close()
	p := New(Definition{BaseURL: srv.URL})
	p.PollInterval = time.Millisecond
	_, err := p.Collect(context.Background(), "original")
	var failed *ExecutionError
	var pending *PendingTaskError
	if !errors.As(err, &failed) || errors.As(err, &pending) || reads.Load() != 4 || posts.Load() != 0 {
		t.Fatal(err, reads.Load(), posts.Load())
	}
}

func TestCollectStopsWhenOriginalStatusStaysUnavailable(t *testing.T) {
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/history/original" {
			t.Error("unexpected request", r.Method, r.URL.Path)
		}
		reads.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := New(Definition{BaseURL: srv.URL})
	p.PollInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := p.Collect(ctx, "original")
	var pending *PendingTaskError
	if !errors.As(err, &pending) || pending.PromptID != "original" || !errors.Is(err, errReadTemporary) || reads.Load() != 8 {
		t.Fatalf("original unavailable ticket=%+v error=%v reads=%d", pending, err, reads.Load())
	}
}

func TestCollectUnknownKeepsOriginalTicketAndCallerCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer srv.Close()
	p := New(Definition{BaseURL: srv.URL})
	p.PollInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := p.Collect(ctx, "original")
	var pending *PendingTaskError
	if !errors.As(err, &pending) || pending.PromptID != "original" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("ticket or cause lost", err)
	}
}
