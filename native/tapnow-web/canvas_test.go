package tapnowweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRemoteCanvasReuseConcurrentAndRestart(t *testing.T) {
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/canvas/v1/canvases" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		id := creates.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"id": fmt.Sprintf("remote-%d", id)}})
	}))
	defer srv.Close()
	c := New("", "fixture", "", t.TempDir(), "")
	c.origin, c.http = srv.URL, srv.Client()
	identity := Identity{User: "user-a", Org: "org-a"}
	positions := make(chan string, 8)
	var wg sync.WaitGroup
	for i := 0; i < cap(positions); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, pos, err := c.remoteCanvas(context.Background(), identity, "local-a", 0, 0, "")
			if err != nil || id != "remote-1" {
				t.Errorf("canvas=%s position=%v error=%v", id, pos, err)
				return
			}
			positions <- fmt.Sprint(pos["x"], ",", pos["y"])
		}()
	}
	wg.Wait()
	close(positions)
	seen := map[string]bool{}
	for pos := range positions {
		if seen[pos] {
			t.Errorf("overlapping position %s", pos)
		}
		seen[pos] = true
	}
	if len(seen) != 8 || creates.Load() != 1 {
		t.Fatalf("positions=%d creates=%d", len(seen), creates.Load())
	}
	fresh := New("", "fixture", "", c.dir, "")
	fresh.origin, fresh.http = srv.URL, srv.Client()
	id, pos, err := fresh.remoteCanvas(context.Background(), identity, "local-a", 0, 0, "")
	if err != nil || id != "remote-1" || pos["y"] != 3300 || creates.Load() != 1 {
		t.Fatalf("restart: %s %v %v creates=%d", id, pos, err, creates.Load())
	}
	for _, scope := range []struct {
		identity Identity
		localID  string
	}{
		{identity, "local-b"},
		{Identity{User: "user-b", Org: "org-a"}, "local-a"},
		{Identity{User: "user-a", Org: "org-b"}, "local-a"},
	} {
		if _, _, err = fresh.remoteCanvas(context.Background(), scope.identity, scope.localID, 0, 0, ""); err != nil {
			t.Fatal(err)
		}
	}
	if creates.Load() != 4 {
		t.Fatalf("different canvases or identities reused: creates=%d", creates.Load())
	}
}

func TestRemoteCanvasUnknownCreateDoesNotRepeat(t *testing.T) {
	c, _, creates := fixture(t, "2", false)
	transport := c.http.Transport
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/canvas/v1/canvases" {
			response, err := transport.RoundTrip(r)
			if err == nil {
				_ = response.Body.Close()
			}
			return nil, errors.New("lost response")
		}
		return transport.RoundTrip(r)
	})
	identity := Identity{User: "user-a", Org: "org-a"}
	if _, _, err := c.remoteCanvas(context.Background(), identity, "local-a", 0, 0, ""); err == nil {
		t.Fatal("lost response accepted")
	}
	fresh := New("", "fixture-token", "", c.dir, "")
	fresh.origin, fresh.http = c.origin, c.http
	if _, _, err := fresh.remoteCanvas(context.Background(), identity, "local-a", 0, 0, ""); err == nil || creates.Load() != 1 {
		t.Fatalf("unknown creation was repeated: error=%v creates=%d", err, creates.Load())
	}
}

func TestRemoteCanvasRejectsChangedAccount(t *testing.T) {
	c, _, creates := fixture(t, "2", false)
	c.accountRevision = 1
	if _, _, err := c.remoteCanvas(context.Background(), Identity{User: "old", Org: "org"}, "local-a", 0, 0, ""); err == nil || creates.Load() != 0 {
		t.Fatalf("changed account accepted: error=%v creates=%d", err, creates.Load())
	}
	if err := c.requestForAccount(context.Background(), 0, "POST", "/api/canvas/v1/canvases", nil, nil); err == nil || creates.Load() != 0 {
		t.Fatalf("changed account write accepted: error=%v creates=%d", err, creates.Load())
	}
}

func TestRemoteCanvasPreflightFailureDoesNotLeavePendingMapping(t *testing.T) {
	c, _, creates := fixture(t, "2", false)
	identity := Identity{User: "user", Org: "org"}
	c.token = ""
	if _, _, err := c.remoteCanvas(context.Background(), identity, "local-a", 0, 0, ""); err == nil {
		t.Fatal("missing credentials accepted")
	}
	c.token = "fixture-token"
	if _, _, err := c.remoteCanvas(context.Background(), identity, "local-a", 0, 0, ""); err != nil || creates.Load() != 1 {
		t.Fatalf("preflight failure left a pending mapping: error=%v creates=%d", err, creates.Load())
	}
}

func TestActivePollAndWritebackRejectChangedAccount(t *testing.T) {
	c, _, _ := fixture(t, "2", false)
	c.accountRevision = 1
	ticket := Ticket{CanvasID: "fixture-canvas", NodeID: "fixture-node", TaskIDs: []string{"fixture-task"}}
	revision := uint64(0)
	if _, err := c.poll(context.Background(), ticket, "image", &revision); err == nil {
		t.Fatal("poll accepted changed account")
	}
	if err := c.writeBack(context.Background(), ticket, Request{}, Mode{}, []tapnow.Media{{URL: "https://example.invalid/result"}}, revision); err == nil {
		t.Fatal("writeback accepted changed account")
	}
}
