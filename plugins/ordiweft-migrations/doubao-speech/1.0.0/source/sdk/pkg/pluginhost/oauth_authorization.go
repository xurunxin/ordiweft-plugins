package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// OAuthAuthorization is data supplied by the package. Only the deployment can
// approve a digest and provide a client registration for that exact profile.
type OAuthAuthorization struct {
	Version      int      `json:"version"`
	Label        string   `json:"label"`
	Issuer       string   `json:"issuer"`
	AuthorizeURL string   `json:"authorizeUrl"`
	TokenURL     string   `json:"tokenUrl"`
	Resource     string   `json:"resource"`
	Scopes       []string `json:"scopes"`
}

var oauthScope = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/\-]{0,127}$`)
var oauthHostname = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func DecodeOAuthAuthorization(body []byte) (OAuthAuthorization, error) {
	var profile OAuthAuthorization
	if len(body) == 0 || len(body) > 16<<10 {
		return profile, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return profile, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&profile) != nil || decoder.Decode(new(any)) != io.EOF ||
		profile.Version != 1 || !policyName(profile.Label, 128) {
		return profile, ErrContract
	}
	issuer := oauthURL(profile.Issuer)
	authorize := oauthURL(profile.AuthorizeURL)
	token := oauthURL(profile.TokenURL)
	resource := oauthURL(profile.Resource)
	if issuer == nil || authorize == nil || token == nil || resource == nil ||
		issuer.Path != "" || resource.Path != "" || authorize.Host != issuer.Host || token.Host != issuer.Host ||
		authorize.Path == "" || token.Path == "" || authorize.Path == token.Path || !ValidOAuthScopes(profile.Scopes) {
		return profile, ErrContract
	}
	return profile, nil
}

func oauthURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 512 || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		!oauthHostname.MatchString(u.Host) || net.ParseIP(u.Hostname()) != nil ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.String() != raw ||
		u.Path != "" && (path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\")) {
		return nil
	}
	return u
}

func (p OAuthAuthorization) Digest() string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
