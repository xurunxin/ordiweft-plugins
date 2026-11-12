package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// WorkerPolicy is installed by a deployment administrator, never loaded from
// package files. AuthorizationEnv names a server environment reference only.
type WorkerPolicy struct {
	Version          int    `json:"version"`
	Origin           string `json:"origin"`
	AllowHTTP        bool   `json:"allowHTTP"`
	Implementation   string `json:"implementation"`
	AuthorizationEnv string `json:"authorizationEnv,omitempty"`
	TimeoutMS        int64  `json:"timeoutMs"`
}

var workerEnvironment = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

func DecodeWorkerPolicy(body []byte) (WorkerPolicy, error) {
	var out WorkerPolicy
	if decodeWorkerJSON(body, 4096, &out) != nil || out.Validate() != nil {
		return WorkerPolicy{}, ErrContract
	}
	return out, nil
}

func (p WorkerPolicy) Validate() error {
	u, err := url.Parse(p.Origin)
	if err != nil || p.Version != Version || len(p.Origin) > 2048 || strings.ContainsAny(p.Origin, "\x00\r\n\\") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Scheme != "https" && (u.Scheme != "http" || !p.AllowHTTP) || !ValidDigest(p.Implementation) || p.TimeoutMS < 100 || p.TimeoutMS > 30000 || p.AuthorizationEnv != "" && !workerEnvironment.MatchString(p.AuthorizationEnv) {
		return ErrContract
	}
	return nil
}

func (p WorkerPolicy) Digest() (string, error) {
	if p.Validate() != nil {
		return "", ErrContract
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
