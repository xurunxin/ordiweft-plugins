package tapnowweb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func authJWT(exp time.Time) string {
	return "eyJhbGciOiJmaXh0dXJlIn0." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix()))) + ".fixture"
}
func authFixture(t *testing.T, fn http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(fn)
	t.Cleanup(s.Close)
	c := New("", "", "", t.TempDir(), "")
	c.origin = s.URL
	return c
}
func authJSON(w http.ResponseWriter, data any) {
	json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func TestSavedLoginUsesDefaultMediaOrigin(t *testing.T) {
	token := authJWT(time.Now().Add(time.Hour))
	c := New("https://app.tapnow.ai", "", "", t.TempDir(), "")
	mockIdentity := func(c *Client, host string) {
		c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Scheme != "https" || r.URL.Host != host || r.URL.Path != "/api/public/v1/users/me" || r.Method != "GET" {
				t.Fatal("unexpected identity request destination")
			}
			if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("x-tapnow-origin") != host || r.Header.Get("X-Org-ID") != "org" {
				t.Fatal("identity request lost credentials or origin header")
			}
			w := httptest.NewRecorder()
			authJSON(w, Identity{"user", "org"})
			return w.Result(), nil
		})
	}
	mockIdentity(c, "app.tapnow.ai")
	if err := c.ImportLogin(context.Background(), Login{token, "private-refresh", "org"}); err != nil {
		t.Fatal(err)
	}
	reloaded := New("", "", "", c.dir, "")
	mockIdentity(reloaded, "app.tapnow.media")
	if reloaded.refreshToken != "private-refresh" || reloaded.userID != "user" {
		t.Fatal("saved login was not restored")
	}
	if err := reloaded.Probe(context.Background()); err != nil || !reloaded.AuthStatus().Connected {
		t.Fatal("saved login failed on default origin", err)
	}
}

func TestSavedLoginRefreshSingleFlight(t *testing.T) {
	old, next := authJWT(time.Now().Add(time.Minute)), authJWT(time.Now().Add(7*24*time.Hour))
	var calls atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v1/auth/tokens/refresh" {
			calls.Add(1)
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["refresh_token"] != "refresh-private" {
				t.Error("missing refresh token")
			}
			authJSON(w, map[string]any{"access_token": next})
			return
		}
		authJSON(w, Identity{"user", "org"})
	})
	if e := c.ImportLogin(context.Background(), Login{old, "refresh-private", ""}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if e := c.Probe(context.Background()); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 || !c.AuthStatus().Connected || c.AuthStatus().LastRefreshAt == 0 {
		t.Fatal("refresh not single-flight", calls.Load())
	}
	reloaded := New("", "environment-token", "", c.dir, "")
	if reloaded.token != next || reloaded.refreshToken != "refresh-private" {
		t.Fatal("rotated login wasn't persisted")
	}
	status, _ := json.Marshal(c.AuthStatus())
	if strings.Contains(string(status), next) || strings.Contains(string(status), "refresh-private") {
		t.Fatal("secret in status")
	}
	// This encryption assertion only applies on Windows; Unix explicitly uses private file permissions.
	b, _ := os.ReadFile(filepath.Join(c.dir, "account.bin"))
	if strings.HasPrefix(credentialStorage, "Windows") && (strings.Contains(string(b), next) || strings.Contains(string(b), "refresh-private")) {
		t.Fatal("plaintext persisted")
	}
	if e := c.Disconnect(); e != nil {
		t.Fatal(e)
	}
	if New("", "environment-token", "", c.dir, "").AuthStatus().Configured {
		t.Fatal("disconnect resurrected environment login")
	}
}

func TestAuthMaintenanceHonorsGate(t *testing.T) {
	old, next := authJWT(time.Now().Add(time.Minute)), authJWT(time.Now().Add(time.Hour))
	var calls atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v1/auth/tokens/refresh" {
			calls.Add(1)
			authJSON(w, map[string]any{"access_token": next})
			return
		}
		authJSON(w, Identity{"user", "org"})
	})
	if err := c.ImportLogin(context.Background(), Login{old, "synthetic-refresh", "org"}); err != nil {
		t.Fatal(err)
	}
	c.maintainAuthOnce(context.Background(), func(func()) bool { return false })
	if calls.Load() != 0 {
		t.Fatal("refresh ran despite the maintenance gate")
	}
	c.maintainAuthOnce(context.Background(), func(run func()) bool { run(); return true })
	if calls.Load() != 1 {
		t.Fatalf("expected one synthetic refresh, got %d", calls.Load())
	}
}
func TestUnauthorizedReadRefreshButNeverReplayWrite(t *testing.T) {
	old, next := authJWT(time.Now().Add(time.Hour)), authJWT(time.Now().Add(2*time.Hour))
	var paid, refresh atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/v1/auth/tokens/refresh":
			refresh.Add(1)
			authJSON(w, map[string]string{"access_token": next})
		case "/api/public/v1/users/me":
			authJSON(w, Identity{"u", "o"})
		case "/api/read":
			if r.Header.Get("Authorization") == "Bearer "+old {
				w.WriteHeader(401)
			} else {
				authJSON(w, map[string]string{"ok": "yes"})
			}
		case "/api/paid":
			paid.Add(1)
			w.WriteHeader(401)
		}
	})
	if e := c.ImportLogin(context.Background(), Login{old, "r", ""}); e != nil {
		t.Fatal(e)
	}
	if c.request(context.Background(), "POST", "/api/paid", map[string]int{"n": 1}, nil) == nil || paid.Load() != 1 || refresh.Load() != 0 {
		t.Fatal("write retried")
	}
	if e := c.request(context.Background(), "GET", "/api/read", nil, nil); e != nil {
		t.Fatal(e)
	}
	if refresh.Load() != 1 {
		t.Fatal("read didn't refresh")
	}
}
func TestLoginRejectsExpiredAndRefreshFailureDoesNotLeak(t *testing.T) {
	var calls atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(401)
		w.Write([]byte("refresh-secret"))
	})
	if c.ImportLogin(context.Background(), Login{authJWT(time.Now().Add(-time.Hour)), "refresh-secret", ""}) == nil || calls.Load() != 0 {
		t.Fatal("expired import hit network")
	}
	c.token, c.refreshToken, c.expiresAt = authJWT(time.Now().Add(time.Minute)), "refresh-secret", time.Now().Add(time.Minute).UnixMilli()
	e := c.ensureToken(context.Background())
	if e == nil || strings.Contains(e.Error(), "refresh-secret") {
		t.Fatal("secret or missing failure")
	}
	_ = c.ensureToken(context.Background())
	if calls.Load() != 1 {
		t.Fatal("failed refresh hammered repeatedly")
	}
}
func TestRejectedImportPreservesSavedLogin(t *testing.T) {
	valid, bad := authJWT(time.Now().Add(time.Hour)), authJWT(time.Now().Add(2*time.Hour))
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+bad {
			w.WriteHeader(401)
			return
		}
		authJSON(w, Identity{"u", "o"})
	})
	if e := c.ImportLogin(context.Background(), Login{valid, "r", ""}); e != nil {
		t.Fatal(e)
	}
	if c.ImportLogin(context.Background(), Login{bad, "s", ""}) == nil {
		t.Fatal("bad login accepted")
	}
	if !c.AuthStatus().Connected || c.token != valid || New("", "", "", c.dir, "").token != valid {
		t.Fatal("failed import changed saved login")
	}
}
