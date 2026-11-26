// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const apiOrigin = "https://api.higgsfield.ai"
const maxAPIResponse = 2 << 20

var requestIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

type client struct {
	http         *http.Client
	origin       string
	credentials  string
	pollInterval time.Duration
}

func newClient(credentials string) client {
	return client{
		http: &http.Client{
			Timeout:       90 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		origin: apiOrigin, credentials: credentials, pollInterval: 5 * time.Second,
	}
}

type remoteMedia struct {
	URL string `json:"url"`
}

type remoteRequest struct {
	ID        string        `json:"request_id"`
	Status    string        `json:"status"`
	StatusURL string        `json:"status_url"`
	Images    []remoteMedia `json:"images"`
	Video     *remoteMedia  `json:"video"`
}

type apiError struct {
	operation string
	status    int
	detail    string
}

func (e *apiError) Error() string {
	if e.detail != "" {
		return fmt.Sprintf("Higgsfield %s returned HTTP %d (%s)", e.operation, e.status, e.detail)
	}
	return fmt.Sprintf("Higgsfield %s returned HTTP %d", e.operation, e.status)
}

func (c client) json(ctx context.Context, method, path, operation string, input any, result any, idempotencyKey string) error {
	var data []byte
	if input != nil {
		var err error
		data, err = json.Marshal(input)
		if err != nil {
			return fmt.Errorf("invalid Higgsfield %s payload", operation)
		}
	}
	var body io.Reader
	if len(data) != 0 {
		// An opaque reader keeps net/http from replaying a request body.
		body = struct{ io.Reader }{bytes.NewReader(data)}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, body)
	if err != nil {
		return fmt.Errorf("invalid Higgsfield %s endpoint", operation)
	}
	req.Header.Set("Authorization", "Key "+c.credentials)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.ContentLength = int64(len(data))
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Higgsfield %s response unconfirmed", operation)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var failure struct {
			Detail string `json:"detail"`
		}
		rejected := &apiError{operation: operation, status: res.StatusCode}
		if json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&failure) == nil {
			switch failure.Detail {
			case "model_unavailable", "insufficient_credits":
				rejected.detail = failure.Detail
			}
		}
		return rejected
	}
	data, err = io.ReadAll(io.LimitReader(res.Body, maxAPIResponse+1))
	if err != nil || len(data) > maxAPIResponse || json.Unmarshal(data, result) != nil {
		return fmt.Errorf("invalid Higgsfield %s response", operation)
	}
	return nil
}

func (c client) submit(ctx context.Context, path string, body map[string]any, jobID string) (remoteRequest, error) {
	var result remoteRequest
	digest := sha256.Sum256([]byte("ordiweft-higgsfield:" + jobID))
	err := c.json(ctx, http.MethodPost, "/"+path, "submission", body, &result, hex.EncodeToString(digest[:]))
	if err != nil {
		return result, err
	}
	if !requestIDPattern.MatchString(result.ID) {
		return result, errors.New("Higgsfield submission has no confirmed request ID")
	}
	return result, nil
}

func (c client) wait(ctx context.Context, original remoteRequest) (remoteRequest, error) {
	statusPath := "/requests/" + original.ID + "/status"
	if original.StatusURL != "" && original.StatusURL != c.origin+statusPath {
		return original, errors.New("invalid Higgsfield status URL")
	}
	for current := original; ; {
		switch current.Status {
		case "completed":
			return current, nil
		case "failed", "nsfw", "canceled":
			return current, fmt.Errorf("Higgsfield request reached %s", current.Status)
		case "queued", "in_progress":
		default:
			return current, errors.New("unknown Higgsfield request status")
		}
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		case <-time.After(c.pollInterval):
		}
		var observed remoteRequest
		err := c.json(ctx, http.MethodGet, statusPath, "status", nil, &observed, "")
		if err != nil {
			var rejected *apiError
			if ctx.Err() == nil && (!errors.As(err, &rejected) || rejected.status == 429 || rejected.status >= 500) {
				continue
			}
			return current, err
		}
		if observed.ID != original.ID {
			return current, errors.New("Higgsfield status request ID mismatch")
		}
		current = observed
	}
}

type uploadGrant struct {
	PublicURL string            `json:"public_url"`
	UploadURL string            `json:"upload_url"`
	Headers   map[string]string `json:"upload_headers"`
}

func (c client) upload(ctx context.Context, data []byte, mime string) (string, error) {
	var grant uploadGrant
	err := c.json(ctx, http.MethodPost, "/files/generate-upload-url", "upload authorization", map[string]string{"content_type": mime}, &grant, "")
	if err != nil {
		return "", err
	}
	if _, ok := httpsURL(grant.PublicURL); !ok {
		return "", errors.New("invalid Higgsfield public input URL")
	}
	target, ok := httpsURL(grant.UploadURL)
	if !ok || target.Hostname() != "fnf-api-input-prod-20250414194641741400000002.s3.amazonaws.com" {
		return "", errors.New("unrecognized Higgsfield storage endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, grant.UploadURL, bytes.NewReader(data))
	if err != nil {
		return "", errors.New("invalid Higgsfield upload request")
	}
	for name, value := range grant.Headers {
		switch http.CanonicalHeaderKey(name) {
		case "Content-Type", "X-Amz-Tagging":
			req.Header.Set(name, value)
		default:
			return "", errors.New("unsupported Higgsfield upload header")
		}
	}
	if !strings.EqualFold(req.Header.Get("Content-Type"), mime) {
		return "", errors.New("Higgsfield upload content type mismatch")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", errors.New("Higgsfield upload response unconfirmed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", &apiError{operation: "upload", status: res.StatusCode}
	}
	return grant.PublicURL, nil
}

func httpsURL(raw string) (*url.URL, bool) {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, "\r\n\\") {
		return nil, false
	}
	u, err := url.Parse(raw)
	return u, err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.Opaque == "" && u.Fragment == "" && (u.Port() == "" || u.Port() == "443")
}
