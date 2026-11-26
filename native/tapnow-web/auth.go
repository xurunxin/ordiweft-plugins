package tapnowweb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Only this type crosses the import boundary. It must never be serialized in a response.
type Login struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	OrgID        string `json:"orgId,omitempty"`
}
type account struct {
	Login
	LastRefreshAt int64          `json:"lastRefreshAt"`
	Credentials   *PasswordLogin `json:"credentials,omitempty"`
	UserID        string         `json:"userId,omitempty"`
}

// PasswordLogin is write-only at the HTTP boundary, and stored with the token.
type PasswordLogin struct {
	Email       string `json:"email,omitempty"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	PhoneRegion string `json:"phoneRegion,omitempty"`
	Password    string `json:"password"`
}

var errAuthRejected = errors.New("TapNow Web 请求被拒绝；请检查登录或在官网完成验证")

type AuthStatus struct {
	Configured       bool   `json:"configured"`
	Connected        bool   `json:"connected"`
	Refreshable      bool   `json:"refreshable"`
	ExpiresAt        int64  `json:"expiresAt"`
	LastRefreshAt    int64  `json:"lastRefreshAt"`
	Storage          string `json:"storage"`
	Reason           string `json:"reason,omitempty"`
	CredentialsSaved bool   `json:"credentialsSaved"`
	RetryAt          int64  `json:"retryAt"`
}

func tokenExpiry(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 32768 {
		return 0, errors.New("需要有效的 Web JWT")
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return 0, errors.New("Web JWT 格式无效")
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(b, &claims) != nil || claims.Exp <= 0 || claims.Exp > 253402300799 {
		return 0, errors.New("Web JWT 缺少有效到期时间")
	}
	return claims.Exp * 1000, nil
}
func validateLogin(a Login) (int64, error) {
	if strings.ContainsAny(a.AccessToken+a.RefreshToken+a.OrgID, "\r\n") || len(a.RefreshToken) > 32768 || len(a.OrgID) > 128 {
		return 0, errors.New("登录信息格式无效")
	}
	exp, e := tokenExpiry(a.AccessToken)
	if e != nil {
		return 0, e
	}
	if exp <= time.Now().UnixMilli() {
		return 0, errors.New("登录令牌已过期，请从网页重新获取")
	}
	return exp, nil
}
func (c *Client) AuthStatus() AuthStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	reason := c.authError
	if reason == "" && c.token == "" {
		reason = "尚未导入 Web 登录"
	}
	if reason == "" && c.expiresAt > 0 && c.expiresAt <= time.Now().UnixMilli() {
		reason = "令牌已到期，下一次请求会尝试刷新"
	}
	return AuthStatus{Configured: c.token != "", Connected: c.verified && (c.expiresAt == 0 || c.expiresAt > time.Now().UnixMilli()), Refreshable: c.refreshToken != "" || c.passwordLogin != nil, ExpiresAt: c.expiresAt, LastRefreshAt: c.lastRefreshAt, Storage: credentialStorage, Reason: reason, CredentialsSaved: c.passwordLogin != nil, RetryAt: c.retryAt}
}
func (c *Client) loadAccount() {
	if c.dir == "" {
		return
	}
	b, e := os.ReadFile(filepath.Join(c.dir, "account.bin"))
	if os.IsNotExist(e) {
		c.expiresAt, _ = tokenExpiry(c.token)
		return
	}
	if e == nil && string(b) == "disconnected" {
		c.token = ""
		return
	}
	var a account
	if e == nil {
		b, e = unprotectCredential(b)
	}
	if e != nil || json.Unmarshal(b, &a) != nil {
		c.token = ""
		c.authError = "保存的登录无法读取，请重新导入"
		return
	}
	exp, e := tokenExpiry(a.AccessToken)
	if e != nil {
		c.token = ""
		c.authError = "保存的登录无效，请重新导入"
		return
	}
	c.token, c.refreshToken, c.org, c.expiresAt, c.lastRefreshAt = a.AccessToken, a.RefreshToken, a.OrgID, exp, a.LastRefreshAt
	c.passwordLogin, c.userID = a.Credentials, a.UserID
}
func (c *Client) persistAccount(a account) error {
	b, e := json.Marshal(a)
	if e == nil {
		b, e = protectCredential(b)
	}
	if e == nil {
		e = c.save("account.bin", b)
	}
	if e != nil {
		return errors.New("登录信息安全保存失败")
	}
	return nil
}
func (c *Client) ImportLogin(ctx context.Context, a Login) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	a.AccessToken = strings.TrimSpace(a.AccessToken)
	a.RefreshToken = strings.TrimSpace(a.RefreshToken)
	a.OrgID = strings.TrimSpace(a.OrgID)
	exp, e := validateLogin(a)
	if e != nil {
		return e
	}
	var i Identity
	if e = c.requestWithToken(ctx, "GET", "/api/public/v1/users/me", nil, &i, a.AccessToken, a.OrgID); e != nil {
		return e
	}
	if i.User == "" || i.Org == "" || (a.OrgID != "" && a.OrgID != i.Org) {
		return errors.New("Web 身份或组织验证失败")
	}
	a.OrgID = i.Org
	return c.commitAccount(account{Login: a, UserID: i.User}, exp, true)
}

// Caller holds authMu. Verify first, then persist and publish one complete account.
func (c *Client) commitAccount(a account, exp int64, changed bool) error {
	if e := c.persistAccount(a); e != nil {
		return e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if changed {
		c.accountRevision++
	}
	c.token, c.refreshToken, c.org, c.expiresAt, c.lastRefreshAt = a.AccessToken, a.RefreshToken, a.OrgID, exp, a.LastRefreshAt
	c.passwordLogin, c.userID, c.verified, c.authError = a.Credentials, a.UserID, true, ""
	c.authPaused, c.retryAt, c.authFailures = false, 0, 0
	return nil
}
func (p PasswordLogin) valid() bool {
	if p.Password == "" || len(p.Password) > 1024 || strings.ContainsAny(p.Email+p.PhoneNumber+p.PhoneRegion+p.Password, "\r\n\x00") {
		return false
	}
	if p.Email != "" {
		a, e := mail.ParseAddress(p.Email)
		return e == nil && a.Address == p.Email && len(p.Email) <= 254 && p.PhoneNumber == "" && p.PhoneRegion == ""
	}
	if len(p.PhoneNumber) < 4 || len(p.PhoneNumber) > 32 || len(p.PhoneRegion) < 1 || len(p.PhoneRegion) > 8 {
		return false
	}
	for _, v := range []string{p.PhoneNumber, p.PhoneRegion} {
		for _, r := range v {
			if (r < '0' || r > '9') && r != '+' {
				return false
			}
		}
	}
	return true
}
func (c *Client) passwordAccount(ctx context.Context, p PasswordLogin) (account, int64, error) {
	body := map[string]string{"password": p.Password}
	if p.Email != "" {
		body["email"] = p.Email
	} else {
		body["phone_number"] = p.PhoneNumber
		body["phone_region"] = p.PhoneRegion
	}
	var result struct {
		Token struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"token"`
	}
	if e := c.requestWithToken(ctx, "POST", "/api/public/v1/auth/login", body, &result, "", ""); e != nil {
		return account{}, 0, e
	}
	a := account{Login: Login{AccessToken: result.Token.AccessToken, RefreshToken: result.Token.RefreshToken}, Credentials: &p}
	exp, e := validateLogin(a.Login)
	if e != nil {
		return account{}, 0, errAuthRejected
	}
	var i Identity
	if e = c.requestWithToken(ctx, "GET", "/api/public/v1/users/me", nil, &i, a.AccessToken, ""); e != nil {
		return account{}, 0, e
	}
	if i.User == "" || i.Org == "" {
		return account{}, 0, errAuthRejected
	}
	a.OrgID, a.UserID = i.Org, i.User
	return a, exp, nil
}
func (c *Client) LoginPassword(ctx context.Context, p PasswordLogin) error {
	p.Email, p.PhoneNumber, p.PhoneRegion = strings.TrimSpace(p.Email), strings.TrimSpace(p.PhoneNumber), strings.TrimSpace(p.PhoneRegion)
	if !p.valid() {
		return errors.New("请填写邮箱或手机号、国家区号及密码")
	}
	c.authMu.Lock()
	defer c.authMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	a, exp, e := c.passwordAccount(ctx, p)
	if e != nil {
		return errors.New("TapNow 账号登录失败；请检查账号密码，或在官网完成验证后同步网页登录")
	}
	return c.commitAccount(a, exp, true)
}
func (c *Client) ForgetPassword() error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.mu.Lock()
	a := account{Login: Login{c.token, c.refreshToken, c.org}, LastRefreshAt: c.lastRefreshAt, UserID: c.userID}
	c.mu.Unlock()
	if a.AccessToken == "" {
		return nil
	}
	if e := c.persistAccount(a); e != nil {
		return e
	}
	c.mu.Lock()
	c.passwordLogin = nil
	c.mu.Unlock()
	return nil
}
func (c *Client) Disconnect() error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	// Write a non-secret marker so a configured environment token cannot silently reappear after restart.
	if e := c.save("account.bin", []byte("disconnected")); e != nil {
		return errors.New("移除保存登录失败")
	}
	c.mu.Lock()
	c.accountRevision++
	c.token, c.refreshToken, c.org, c.expiresAt, c.lastRefreshAt, c.verified, c.authError = "", "", "", 0, 0, false, ""
	c.passwordLogin, c.userID, c.authPaused, c.retryAt, c.authFailures = nil, "", false, 0, 0
	c.mu.Unlock()
	return nil
}
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	token, exp, refresh, saved, paused, retryAt := c.token, c.expiresAt, c.refreshToken, c.passwordLogin != nil, c.authPaused, c.retryAt
	c.mu.Unlock()
	if paused {
		return errUnauthorized
	}
	if exp > 0 && exp < time.Now().Add(2*time.Minute).UnixMilli() {
		if (refresh != "" || saved) && retryAt <= time.Now().UnixMilli() {
			if e := c.refreshAfter(ctx, token); e == nil {
				return nil
			} else {
				c.mu.Lock()
				paused = c.authPaused
				c.mu.Unlock()
				if paused || exp <= time.Now().UnixMilli() {
					return e
				}
			}
		}
		if exp <= time.Now().UnixMilli() {
			return errUnauthorized
		}
	}
	return nil
}

// Keep idle logins fresh. Rejected credentials pause; temporary failures back off.
func (c *Client) MaintainAuth(ctx context.Context, gate func(func()) bool, enabled ...func() bool) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.maintainAuthOnce(ctx, gate, enabled...)
		}
	}
}

func (c *Client) maintainAuthOnce(ctx context.Context, gate func(func()) bool, enabled ...func() bool) {
	if len(enabled) > 0 && !enabled[0]() {
		return
	}
	c.mu.Lock()
	due := (c.refreshToken != "" || c.passwordLogin != nil) && !c.authPaused && c.retryAt <= time.Now().UnixMilli() && c.expiresAt > 0 && c.expiresAt < time.Now().Add(2*time.Minute).UnixMilli()
	c.mu.Unlock()
	if due {
		refresh := func() { _ = c.ensureToken(ctx) }
		if gate == nil {
			refresh()
		} else {
			gate(refresh)
		}
	}
}
func (c *Client) RefreshAuth(ctx context.Context) error { return c.refreshAfter(ctx, "") }
func (c *Client) refreshAfter(ctx context.Context, observed string) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.mu.Lock()
	a := account{Login: Login{c.token, c.refreshToken, c.org}, Credentials: c.passwordLogin, UserID: c.userID}
	blocked := c.authPaused || c.retryAt > time.Now().UnixMilli()
	c.mu.Unlock()
	if observed != "" && a.AccessToken != observed {
		return nil
	}
	if observed != "" && blocked {
		return errUnauthorized
	}
	if a.RefreshToken == "" && a.Credentials == nil {
		return errors.New("没有保存刷新令牌，请重新导入 Web 登录")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	// The refresh endpoint takes the refresh token in its JSON body; never recursively refresh it.
	e := errAuthRejected
	if a.RefreshToken != "" {
		e = c.requestWithToken(ctx, "POST", "/api/public/v1/auth/tokens/refresh", map[string]string{"refresh_token": a.RefreshToken}, &result, "", "")
	}
	if e != nil && a.Credentials != nil && (errors.Is(e, errUnauthorized) || errors.Is(e, errAuthRejected)) {
		next, exp, loginErr := c.passwordAccount(ctx, *a.Credentials)
		if loginErr == nil {
			if next.OrgID != a.OrgID || a.UserID != "" && next.UserID != a.UserID {
				c.authFailure(true)
				return errors.New("自动登录身份不一致，请管理员重新登录")
			}
			next.LastRefreshAt = time.Now().UnixMilli()
			if e := c.commitAccount(next, exp, false); e != nil {
				c.authFailure(false)
				return e
			}
			return nil
		}
		e = loginErr
	}
	if e != nil {
		c.authFailure(errors.Is(e, errUnauthorized) || errors.Is(e, errAuthRejected))
		return errors.New("Web 登录维护失败，请查看登录状态")
	}
	a.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		a.RefreshToken = result.RefreshToken
	}
	exp, e := validateLogin(a.Login)
	if e != nil {
		c.authFailure(true)
		return errors.New("Web 刷新响应无效，请重新登录")
	}
	// Refresh isn't proof of identity; only a successful probe sets connected.
	var i Identity
	if e = c.requestWithToken(ctx, "GET", "/api/public/v1/users/me", nil, &i, a.AccessToken, a.OrgID); e != nil {
		c.authFailure(errors.Is(e, errUnauthorized) || errors.Is(e, errAuthRejected))
		return errors.New("刷新后的 Web 身份验证失败")
	}
	if i.User == "" || i.Org != a.OrgID || a.UserID != "" && i.User != a.UserID {
		c.authFailure(true)
		return errors.New("刷新后的 Web 身份验证失败")
	}
	a.UserID, a.LastRefreshAt = i.User, time.Now().UnixMilli()
	if e = c.commitAccount(a, exp, false); e != nil {
		c.authFailure(false)
		return e
	}
	return nil
}
func (c *Client) authFailure(permanent bool) {
	c.mu.Lock()
	c.authPaused = permanent
	if permanent {
		c.verified = false
		c.retryAt = 0
		c.authError = "Web 登录已失效；请检查保存的账号密码，或同步网页登录"
	} else {
		c.authFailures++
		delay := time.Minute * time.Duration(1<<min(c.authFailures-1, 4))
		if delay > 15*time.Minute {
			delay = 15 * time.Minute
		}
		c.retryAt = time.Now().Add(delay).UnixMilli()
		c.authError = "Web 登录维护暂时失败，将自动重试"
	}
	c.mu.Unlock()
}
