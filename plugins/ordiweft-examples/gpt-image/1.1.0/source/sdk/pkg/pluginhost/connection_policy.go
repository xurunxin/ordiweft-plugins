package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type HeaderSource struct {
	Name       string `json:"name"`
	Credential string `json:"credential,omitempty"`
	Prefix     string `json:"prefix,omitempty"`
	Literal    string `json:"literal,omitempty"`
	JobID      bool   `json:"jobId,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
}

type ConnectionOperation struct {
	Method           string               `json:"method"`
	Path             string               `json:"path"`
	Effect           string               `json:"effect"`
	Slot             string               `json:"slot,omitempty"`
	ContentType      string               `json:"contentType,omitempty"`
	TimeoutMS        int64                `json:"timeoutMs"`
	MaxRequestBytes  int64                `json:"maxRequestBytes"`
	MaxResponseBytes int64                `json:"maxResponseBytes"`
	JSONPolicy       *JSONPolicyReference `json:"jsonPolicy,omitempty"`
	ReceiptPath      *ReceiptPathSource   `json:"receiptPath,omitempty"`
	Multipart        []MultipartFile      `json:"multipart,omitempty"`
	InputBody        *InputBodySource     `json:"inputBody,omitempty"`
	HostOnlyResponse bool                 `json:"hostOnlyResponse,omitempty"`
	QueryParameters  bool                 `json:"queryParameters,omitempty"`
	ReceiptTarget    *ReceiptTarget       `json:"receiptTarget,omitempty"`
	RequiresUploads  map[string]string    `json:"requiresUploads,omitempty"`
	Streaming        bool                 `json:"streaming,omitempty"`
}

type ReceiptPathSource struct {
	Ref     string `json:"ref"`
	Pointer string `json:"pointer"`
	Suffix  string `json:"suffix,omitempty"`
}

// ResolvePath accepts one bounded original remote ticket, never a plugin URL.
func (o ConnectionOperation) ResolvePath(receipts map[string]json.RawMessage) (string, error) {
	if o.ReceiptPath == nil {
		return o.Path, nil
	}
	source := o.ReceiptPath
	raw, exists := receipts[source.Ref]
	if !exists {
		return "", ErrContract
	}
	value, err := strictJSON(raw)
	if err != nil {
		return "", err
	}
	ticket, present, err := pointerValue(value, source.Pointer)
	text, ok := ticket.(string)
	if err != nil || !present || !ok || !opaqueID.MatchString(text) {
		return "", ErrContract
	}
	return strings.TrimRight(o.Path, "/") + "/" + text + source.Suffix, nil
}

// ConnectionPolicy is host-installed configuration. A package may request an
// operation but cannot create this binding or supply its origin/key per call.
type ConnectionPolicy struct {
	Version          int                             `json:"version"`
	Origin           string                          `json:"origin,omitempty"`
	OriginCredential string                          `json:"originCredential,omitempty"`
	AllowHTTP        bool                            `json:"allowHttp,omitempty"`
	Headers          []HeaderSource                  `json:"headers"`
	Operations       map[string]ConnectionOperation  `json:"operations"`
	Downloads        map[string]OutputDownloadPolicy `json:"downloads,omitempty"`
	TaskIO           *TaskIOPolicy                   `json:"taskIO,omitempty"`
	MaxRuntimeMS     int64                           `json:"maxRuntimeMs,omitempty"`
	Waits            map[string]WaitPolicy           `json:"waits,omitempty"`
}

func DecodeConnectionPolicy(body []byte) (ConnectionPolicy, error) {
	var p ConnectionPolicy
	if len(body) == 0 || len(body) > 128<<10 {
		return p, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
		return p, ErrContract
	}
	return p, p.Validate()
}

func (p ConnectionPolicy) Validate() error {
	limits := p.EffectiveTaskIO()
	if limits.Validate() != nil || !ValidTaskRuntimeMS(p.EffectiveTaskRuntimeMS()) {
		return ErrContract
	}
	if p.Version != Version || (p.Origin == "") == (p.OriginCredential == "") || len(p.Headers) > 32 || len(p.Operations) == 0 || len(p.Operations) > 64 {
		return ErrContract
	}
	if p.OriginCredential != "" && !credentialName(p.OriginCredential) {
		return ErrContract
	}
	if p.Origin != "" && validConnectionOrigin(p.Origin, p.AllowHTTP) != nil {
		return ErrContract
	}
	seen := map[string]bool{}
	for _, header := range p.Headers {
		name := strings.ToLower(header.Name)
		if !headerToken(header.Name) || forbiddenHeader(name) || seen[name] || len(header.Prefix) > 64 || strings.ContainsAny(header.Prefix, "\x00\r\n") || len(header.Literal) > 1024 || strings.ContainsAny(header.Literal, "\x00\r\n") {
			return ErrContract
		}
		seen[name] = true
		sources := 0
		if header.Credential != "" {
			if !credentialName(header.Credential) {
				return ErrContract
			}
			sources++
		}
		if header.JobID {
			sources++
		}
		if header.Literal != "" {
			sources++
		}
		if sources != 1 || header.Credential == "" && (header.Prefix != "" || header.Optional) || (name == "authorization" || name == "x-api-key") && (header.Credential == "" || header.Optional) {
			return ErrContract
		}
	}
	for id, operation := range p.Operations {
		if len(operation.RequiresUploads) > 32 || len(operation.RequiresUploads) > 0 && operation.Effect != "submit" {
			return ErrContract
		}
		for slot, callID := range operation.RequiresUploads {
			if !aigc.IDPattern.MatchString(slot) || !opaqueID.MatchString(callID) || slot == operation.Slot {
				return ErrContract
			}
			found := false
			for _, dependency := range p.Operations {
				if dependency.Effect == "upload" && dependency.Slot == slot {
					found = true
					break
				}
			}
			if !found {
				return ErrContract
			}
		}
		if operation.MaxRequestBytes > limits.MaxRequestBytes || operation.MaxResponseBytes > limits.MaxResponseBytes {
			return ErrContract
		}
		if !aigc.IDPattern.MatchString(id) || !slices.Contains([]string{"GET", "POST", "PUT", "DELETE"}, operation.Method) || !slices.Contains([]string{"query", "submit", "upload"}, operation.Effect) || operation.Path == "" || operation.Path[0] != '/' || path.Clean(operation.Path) != operation.Path || strings.HasPrefix(operation.Path, "//") || strings.ContainsAny(operation.Path, "?#%\\\x00\r\n") || len(operation.Path) > 2048 || operation.TimeoutMS < 1 || operation.TimeoutMS > 900000 || operation.MaxRequestBytes < 0 || operation.MaxRequestBytes > MaxJSONRequestBytes || operation.MaxResponseBytes < 1 || operation.MaxResponseBytes > 256<<20 {
			return ErrContract
		}
		if operation.Effect == "query" {
			if operation.Slot != "" {
				return ErrContract
			}
		} else {
			if !aigc.IDPattern.MatchString(operation.Slot) {
				return ErrContract
			}
		}
		if operation.QueryParameters && (operation.Method != "GET" || operation.Effect != "query" || operation.ContentType != "" || operation.MaxRequestBytes != 0 || operation.JSONPolicy == nil || operation.InputBody != nil || len(operation.Multipart) != 0) {
			return ErrContract
		}
		if operation.Streaming && (operation.Effect != "submit" || operation.HostOnlyResponse || operation.ReceiptTarget != nil) {
			return ErrContract
		}
		if target := operation.ReceiptTarget; target != nil {
			if target.Validate() != nil || operation.Effect != "upload" || operation.InputBody == nil && len(operation.Multipart) == 0 || operation.Path != "/" || operation.ReceiptPath != nil || operation.QueryParameters {
				return ErrContract
			}
		}
		if source := operation.ReceiptPath; source != nil {
			if operation.Effect != "query" || !opaqueID.MatchString(source.Ref) || source.Suffix != "" && (source.Suffix == "/" || source.Suffix[0] != '/' || path.Clean(source.Suffix) != source.Suffix || strings.HasPrefix(source.Suffix, "//") || strings.ContainsAny(source.Suffix, "?#%\\\x00\r\n") || len(source.Suffix) > 1024) {
				return ErrContract
			}
			if _, err := pointerParts(source.Pointer); err != nil {
				return err
			}
		}
		if operation.InputBody != nil {
			if operation.Effect != "upload" || !slices.Contains([]string{"POST", "PUT"}, operation.Method) || operation.ContentType != "application/octet-stream" || operation.JSONPolicy != nil || len(operation.Multipart) != 0 || operation.ReceiptPath != nil || operation.MaxRequestBytes < 1 || !operation.InputBody.valid() {
				return ErrContract
			}
		} else if operation.Effect == "upload" && (operation.Method != "POST" || len(operation.Multipart) == 0 || operation.JSONPolicy == nil) {
			return ErrContract
		} else if operation.JSONPolicy == nil {
			// Bodies require the installed field/model/count policy, including
			// multipart's separately selected original files.
			if operation.Effect != "query" || operation.MaxRequestBytes != 0 || operation.ContentType != "" || len(operation.Multipart) != 0 {
				return ErrContract
			}
		} else {
			if !aigc.IDPattern.MatchString(operation.JSONPolicy.Operation) || !ValidDigest(operation.JSONPolicy.Digest) {
				return ErrContract
			}
			if !operation.QueryParameters {
				if operation.MaxRequestBytes < 2 {
					return ErrContract
				}
				if len(operation.Multipart) == 0 {
					if operation.ContentType != "application/json" {
						return ErrContract
					}
				} else if operation.ContentType != "multipart/form-data" || validateMultipartFiles(operation.Multipart) != nil {
					return ErrContract
				}
			}
		}
	}
	if len(p.Downloads) > 16 {
		return ErrContract
	}
	for name, download := range p.Downloads {
		if download.MaxBytes > limits.MaxResponseBytes {
			return ErrContract
		}
		if !aigc.IDPattern.MatchString(name) || download.Validate() != nil {
			return ErrContract
		}
		if _, exists := p.Operations[download.SourceOperation]; !exists {
			return ErrContract
		}
	}
	if len(p.Waits) > 16 {
		return ErrContract
	}
	for name, wait := range p.Waits {
		op, exists := p.Operations[wait.Operation]
		if !aigc.IDPattern.MatchString(name) || wait.Validate() != nil || !exists || op.Effect != "query" || op.MaxResponseBytes > 64<<10 {
			return ErrContract
		}
	}
	return nil
}

func (p ConnectionPolicy) CredentialKeys() []string {
	keys := map[string]bool{}
	if p.OriginCredential != "" {
		keys[p.OriginCredential] = true
	}
	for _, header := range p.Headers {
		if header.Credential != "" {
			keys[header.Credential] = true
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (p ConnectionPolicy) Digest() (string, error) {
	if p.Validate() != nil {
		return "", ErrContract
	}
	p.Headers = append([]HeaderSource{}, p.Headers...)
	slices.SortFunc(p.Headers, func(a, b HeaderSource) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for i := range p.Headers {
		p.Headers[i].Name = http.CanonicalHeaderKey(p.Headers[i].Name)
	}
	body, err := json.Marshal(p)
	if err != nil {
		return "", ErrContract
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}

// Resolve uses host-owned values. It does no environment lookup, network or
// credential storage and never accepts a plugin header map.
func (p ConnectionPolicy) Resolve(jobID string, credentials map[string]string) (string, http.Header, error) {
	if p.Validate() != nil {
		return "", nil, ErrContract
	}
	origin := p.Origin
	if p.OriginCredential != "" {
		origin = credentials[p.OriginCredential]
	}
	if validConnectionOrigin(origin, p.AllowHTTP) != nil {
		return "", nil, ErrContract
	}
	headers := make(http.Header)
	for _, header := range p.Headers {
		value := header.Literal
		if header.JobID {
			if !opaqueID.MatchString(jobID) {
				return "", nil, ErrContract
			}
			value = jobID
		}
		if header.Credential != "" {
			value = credentials[header.Credential]
			if value == "" && header.Optional {
				continue
			}
			if value == "" || len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
				return "", nil, ErrContract
			}
			value = header.Prefix + value
		}
		headers.Set(header.Name, value)
	}
	return origin, headers, nil
}

func credentialName(name string) bool { return len(name) <= 128 && aigc.EnvPattern.MatchString(name) }

func validConnectionOrigin(origin string, allowHTTP bool) error {
	u, err := url.Parse(origin)
	if err != nil || u == nil || len(origin) > 2048 || strings.ContainsAny(origin, "\x00\r\n") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Opaque != "" || u.Scheme != "https" && (u.Scheme != "http" || !allowHTTP) || u.Path != "" && path.Clean(u.Path) != u.Path {
		return ErrContract
	}
	return nil
}

func headerToken(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch) {
			continue
		}
		return false
	}
	return true
}

func forbiddenHeader(name string) bool {
	return strings.HasPrefix(name, "proxy-") || strings.HasPrefix(name, "x-forwarded-") || slices.Contains([]string{"host", "cookie", "set-cookie", "connection", "content-length", "content-type", "transfer-encoding", "trailer", "upgrade", "te", "forwarded", "x-original-url", "x-rewrite-url", "x-http-method-override"}, name)
}
