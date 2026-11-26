package tapnowweb

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordLoginMaintainsExpiredAccountAndForgetsPassword(t *testing.T) {
	old, next := authJWT(time.Now().Add(time.Hour)), authJWT(time.Now().Add(4*time.Hour))
	var logins, refreshes atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/v1/auth/login":
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["email"] != "fixture@example.test" || b["password"] != "private-password" || r.Header.Get("Authorization") != "" {
				t.Error("login request")
			}
			token := old
			if logins.Add(1) > 1 {
				token = next
			}
			authJSON(w, map[string]any{"token": map[string]string{"access_token": token, "refresh_token": "private-refresh"}})
		case "/api/public/v1/auth/tokens/refresh":
			refreshes.Add(1)
			w.WriteHeader(401)
		default:
			authJSON(w, Identity{"user", "org"})
		}
	})
	if e := c.LoginPassword(context.Background(), PasswordLogin{Email: "fixture@example.test", Password: "private-password"}); e != nil {
		t.Fatal(e)
	}
	reloaded := New("", "", "", c.dir, "")
	reloaded.origin = c.origin
	if !reloaded.AuthStatus().CredentialsSaved {
		t.Fatal("credentials not restored")
	}
	// Expired on startup: one refresh and one password login, shared by readers.
	reloaded.expiresAt = time.Now().Add(-time.Minute).UnixMilli()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if e := reloaded.Probe(context.Background()); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if logins.Load() != 2 || refreshes.Load() != 1 || reloaded.token != next || !reloaded.AuthStatus().Connected {
		t.Fatal("maintenance was not shared")
	}
	b, _ := json.Marshal(reloaded.AuthStatus())
	for _, secret := range []string{"private-password", "private-refresh", next, "fixture@example.test"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("status leaked")
		}
	}
	if e := reloaded.ForgetPassword(); e != nil {
		t.Fatal(e)
	}
	reloaded = New("", "", "", c.dir, "")
	if reloaded.AuthStatus().CredentialsSaved || reloaded.token != next || !reloaded.AuthStatus().Refreshable {
		t.Fatal("forget removed tokens or retained password")
	}
	if e := reloaded.Disconnect(); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(filepath.Join(c.dir, "account.bin"))
	if string(b) != "disconnected" {
		t.Fatal("disconnect retained credentials")
	}
}

func TestTemporaryRefreshFailureBacksOffThenRecovers(t *testing.T) {
	var failed atomic.Bool
	failed.Store(true)
	var calls atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/refresh") {
			calls.Add(1)
			if failed.Load() {
				w.WriteHeader(503)
				return
			}
			authJSON(w, map[string]string{"access_token": authJWT(time.Now().Add(time.Hour))})
			return
		}
		authJSON(w, Identity{"u", "o"})
	})
	if e := c.ImportLogin(context.Background(), Login{authJWT(time.Now().Add(time.Minute)), "refresh", ""}); e != nil {
		t.Fatal(e)
	}
	if e := c.ensureToken(context.Background()); e != nil {
		t.Fatal("valid token blocked by transient failure", e)
	}
	for range 5 {
		_ = c.ensureToken(context.Background())
	}
	if calls.Load() != 1 || c.authPaused || c.AuthStatus().RetryAt == 0 {
		t.Fatal("no backoff")
	}
	failed.Store(false)
	c.retryAt = 0
	if e := c.ensureToken(context.Background()); e != nil || calls.Load() != 2 || c.AuthStatus().RetryAt != 0 {
		t.Fatal("did not recover", e)
	}
}

func TestRejectedPasswordAndIdentityChangePreserveSavedAccount(t *testing.T) {
	var wrong atomic.Bool
	old := authJWT(time.Now().Add(time.Hour))
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/v1/auth/login":
			w.WriteHeader(401)
			w.Write([]byte("private-password"))
		case "/api/public/v1/auth/tokens/refresh":
			authJSON(w, map[string]string{"access_token": authJWT(time.Now().Add(2 * time.Hour))})
		default:
			if wrong.Load() {
				authJSON(w, Identity{"other-user", "o"})
			} else {
				authJSON(w, Identity{"u", "o"})
			}
		}
	})
	if e := c.ImportLogin(context.Background(), Login{old, "refresh", ""}); e != nil {
		t.Fatal(e)
	}
	e := c.LoginPassword(context.Background(), PasswordLogin{Email: "fixture@example.test", Password: "private-password"})
	if e == nil || strings.Contains(e.Error(), "private-password") || c.token != old {
		t.Fatal("failed login changed account or leaked password")
	}
	wrong.Store(true)
	if e := c.RefreshAuth(context.Background()); e == nil {
		t.Fatal("identity switched")
	}
	if c.token != old || New("", "", "", c.dir, "").token != old {
		t.Fatal("unverified refreshed login published")
	}
}

func TestPasswordValidationAndImportClearsSavedCredentials(t *testing.T) {
	var calls atomic.Int32
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); authJSON(w, Identity{"u", "o"}) })
	for _, p := range []PasswordLogin{{Email: "invalid", Password: "x"}, {Email: "a@b.test", PhoneNumber: "123456", Password: "x"}, {PhoneNumber: "123456", Password: "x"}, {Email: "a@b.test", Password: "x\nsecret"}} {
		if c.LoginPassword(context.Background(), p) == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid credentials sent upstream")
	}
	c.passwordLogin = &PasswordLogin{Email: "old@example.test", Password: "old-password"}
	if e := c.ImportLogin(context.Background(), Login{authJWT(time.Now().Add(time.Hour)), "r", ""}); e != nil {
		t.Fatal(e)
	}
	if c.AuthStatus().CredentialsSaved || New("", "", "", c.dir, "").AuthStatus().CredentialsSaved {
		t.Fatal("import retained old-account password")
	}
}
