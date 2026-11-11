package pluginhost

import (
	"net/url"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// OutputDownloadPolicy is an installed output selection, not a URL supplied by
// a plugin call. Network address validation remains the host transport's job.
type OutputDownloadPolicy struct {
	SourceOperation string `json:"sourceOperation"`
	Pointer         string `json:"pointer"`
	MaxBytes        int64  `json:"maxBytes"`
	TimeoutMS       int64  `json:"timeoutMs"`
}

func (p OutputDownloadPolicy) Validate() error {
	if !aigc.IDPattern.MatchString(p.SourceOperation) || p.MaxBytes < 1 || p.MaxBytes > 64<<20 || p.TimeoutMS < 1 || p.TimeoutMS > 120000 {
		return ErrContract
	}
	_, err := pointerParts(p.Pointer)
	return err
}

func (p OutputDownloadPolicy) ResolveURL(body []byte) (string, error) {
	if p.Validate() != nil || len(body) == 0 || len(body) > 64<<10 {
		return "", ErrContract
	}
	value, err := strictJSON(body)
	if err != nil {
		return "", err
	}
	selected, present, err := pointerValue(value, p.Pointer)
	raw, ok := selected.(string)
	if err != nil || !present || !ok || len(raw) == 0 || len(raw) > 8192 || strings.TrimSpace(raw) != raw {
		return "", ErrContract
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", ErrContract
	}
	return raw, nil
}
