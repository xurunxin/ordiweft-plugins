package tapnowweb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestParallelPollingCompletionAndFailureLeaveSlowTaskRunning(t *testing.T) {
	var release atomic.Bool
	var base string
	seen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/conversation/v1/generations/tasks" {
			t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		id := r.URL.Query().Get("ids")
		if id == "slow" && !release.Load() {
			select {
			case seen <- struct{}{}:
			default:
			}
			fmt.Fprint(w, `{"code":0,"data":[{"id":"slow","status":"running"}]}`)
			return
		}
		if id == "failed" {
			fmt.Fprint(w, `{"code":0,"data":[{"id":"failed","status":"failed","failed_reason":"content_violation","failed_reason_detail":"The input text may violate platform rules. Please modify the text and try again."}]}`)
			return
		}
		fmt.Fprintf(w, `{"code":0,"data":[{"id":%q,"status":"completed","outputs":{"image_urls":[%q]}}]}`, id, base+"/result")
	}))
	defer server.Close()
	base = server.URL
	c := New("", "fixture-token", "", t.TempDir(), "")
	c.origin = base
	c.http = server.Client()
	c.download = tapnow.New(base, "")
	c.PollInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	slow := make(chan error, 1)
	go func() { _, err := c.poll(ctx, Ticket{TaskIDs: []string{"slow"}}, "image", nil); slow <- err }()
	select {
	case <-seen:
	case <-ctx.Done():
		t.Fatal("slow task not polled")
	}
	if media, err := c.poll(ctx, Ticket{TaskIDs: []string{"first"}}, "image", nil); err != nil || len(media) != 1 {
		t.Fatal(media, err)
	}
	_, err := c.poll(ctx, Ticket{TaskIDs: []string{"failed"}}, "image", nil)
	var failed *TaskFailedError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "请求未通过审核") {
		t.Fatal(err)
	}
	select {
	case err := <-slow:
		t.Fatal("peer terminated", err)
	default:
	}
	release.Store(true)
	select {
	case err := <-slow:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("slow result lost")
	}
}
