package pluginhost

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
)

// ReceiptTarget selects an original private response, never a plugin URL.
type ReceiptTarget struct {
	Ref     string   `json:"ref"`
	Pointer string   `json:"pointer"`
	Hosts   []string `json:"hosts"`
}

func (p ReceiptTarget) Validate() error {
	if !opaqueID.MatchString(p.Ref) || len(p.Hosts) == 0 || len(p.Hosts) > 16 {
		return ErrContract
	}
	if _, err := pointerParts(p.Pointer); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, host := range p.Hosts {
		name := strings.TrimPrefix(host, "*.")
		if host != strings.ToLower(host) || seen[host] || len(name) == 0 || len(name) > 253 || strings.HasSuffix(name, ".") {
			return ErrContract
		}
		seen[host] = true
		if net.ParseIP(name) != nil {
			if name != host {
				return ErrContract
			}
			continue
		}
		if name != host && strings.Count(name, ".") < 1 {
			return ErrContract
		}
		for _, label := range strings.Split(name, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ErrContract
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
					return ErrContract
				}
			}
		}
	}
	return nil
}

func (p ReceiptTarget) Resolve(receipts map[string]json.RawMessage, originalOrigin string, allowHTTP bool) (string, error) {
	if p.Validate() != nil {
		return "", ErrContract
	}
	value, err := strictJSON(receipts[p.Ref])
	if err != nil {
		return "", err
	}
	selected, present, err := pointerValue(value, p.Pointer)
	raw, ok := selected.(string)
	if err != nil || !present || !ok || raw == "" || len(raw) > 8192 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\x00\r\n\\") {
		return "", ErrContract
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Scheme != "https" && u.Scheme != "http" {
		return "", ErrContract
	}
	base, err := url.Parse(originalOrigin)
	local := err == nil && allowHTTP && base.Scheme == u.Scheme && base.Host == u.Host
	if !local && (u.Scheme != "https" || u.Port() != "" && u.Port() != "443") {
		return "", ErrContract
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range p.Hosts {
		if host == allowed || strings.HasPrefix(allowed, "*.") && strings.HasSuffix(host, allowed[1:]) && host != allowed[2:] {
			return raw, nil
		}
	}
	return "", ErrContract
}
