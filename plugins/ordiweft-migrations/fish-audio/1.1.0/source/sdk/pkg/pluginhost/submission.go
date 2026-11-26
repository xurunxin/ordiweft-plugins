package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strings"
)

// SubmissionPolicy is installed with the connection, never supplied by a task.
type SubmissionPolicy struct {
	Preflight      string `json:"preflight"`
	ServiceVersion string `json:"serviceVersion"`
	Account        string `json:"account"`
	Space          string `json:"space"`
	MaxAgeMS       int64  `json:"maxAgeMs"`
	RequireQuote   bool   `json:"requireQuote,omitempty"`
}

func (s SubmissionPolicy) Validate(p ConnectionPolicy, operation string) error {
	op := p.Operations[operation]
	query, exists := p.Operations[s.Preflight]
	if op.Effect != "submit" || op.ContentType != "application/json" || op.JSONPolicy == nil || op.Streaming || op.ReceiptTarget != nil || !exists || s.Preflight == operation || query.Effect != "query" || query.Method != "POST" || query.ContentType != "application/json" || query.JSONPolicy == nil || query.Submission != nil || query.ReceiptPath != nil || query.ReceiptTarget != nil || query.QueryParameters || !query.HostOnlyResponse || query.MaxResponseBytes > 64<<10 || query.TimeoutMS > 10000 || s.MaxAgeMS < 1 || s.MaxAgeMS > 60000 || !VersionAtLeast(s.ServiceVersion, "0.0.0") || !opaqueID.MatchString(s.Account) || !opaqueID.MatchString(s.Space) {
		return ErrContract
	}
	return nil
}

type SubmissionQuote struct {
	Source        string   `json:"source"`
	Known         bool     `json:"known"`
	Amount        *float64 `json:"amount,omitempty"`
	Currency      string   `json:"currency"`
	Unit          string   `json:"unit,omitempty"`
	CapacityKnown bool     `json:"capacityKnown"`
	Available     *float64 `json:"available,omitempty"`
}

type SubmissionSnapshot struct {
	Version        int              `json:"version"`
	JobID          string           `json:"jobId"`
	RequestDigest  string           `json:"requestDigest"`
	ServiceVersion string           `json:"serviceVersion"`
	Account        string           `json:"account"`
	Space          string           `json:"space"`
	ObservedAt     int64            `json:"observedAt"`
	ExpiresAt      int64            `json:"expiresAt"`
	Decision       string           `json:"decision"`
	Quote          *SubmissionQuote `json:"quote,omitempty"`
}

func (s SubmissionPolicy) DecodeSnapshot(body []byte, job, digest string, now int64) (SubmissionSnapshot, error) {
	var out SubmissionSnapshot
	if len(body) > 64<<10 {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if !canonicalSubmissionKeys(body) || d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Version != 1 || out.JobID != job || out.RequestDigest != digest || !ValidDigest(digest) || out.ServiceVersion != s.ServiceVersion || out.Account != s.Account || out.Space != s.Space || out.Decision != "allow" || out.ObservedAt <= 0 || out.ObservedAt > now || out.ExpiresAt <= now || out.ExpiresAt <= out.ObservedAt || out.ExpiresAt-out.ObservedAt > s.MaxAgeMS || now-out.ObservedAt > s.MaxAgeMS {
		return out, ErrContract
	}
	if q := out.Quote; q != nil {
		validNumber := func(v *float64) bool {
			return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= 0 && *v <= 1e9
		}
		if !opaqueID.MatchString(q.Source) || q.Known && (!validNumber(q.Amount) || !opaqueID.MatchString(q.Currency)) || q.CapacityKnown && !validNumber(q.Available) || s.RequireQuote && (!q.Known || q.Unit != "" || !q.CapacityKnown || *q.Available < *q.Amount) {
			return out, ErrContract
		}
		if !q.Known {
			q.Amount, q.Currency, q.Unit = nil, "", ""
		}
		if !q.CapacityKnown {
			q.Available = nil
		}
	} else if s.RequireQuote {
		return out, ErrContract
	}
	return out, nil
}

// SubmissionResult is derived from the original trusted response, not diagnostics.
type SubmissionResult struct {
	CallID        string                   `json:"callId"`
	RequestDigest string                   `json:"requestDigest"`
	Submission    string                   `json:"submission"`
	Quote         *SubmissionQuoteSnapshot `json:"quote,omitempty"`
}

type SubmissionQuoteSnapshot struct {
	CallID     string   `json:"callId"`
	Source     string   `json:"source"`
	Known      bool     `json:"known"`
	Amount     *float64 `json:"amount,omitempty"`
	Currency   string   `json:"currency"`
	Unit       string   `json:"unit,omitempty"`
	ObservedAt int64    `json:"observedAt"`
	ExpiresAt  int64    `json:"expiresAt"`
}

func (s SubmissionPolicy) Classify(body []byte, job, digest string) string {
	var proof struct {
		Version                                                          int `json:"version"`
		JobID, RequestDigest, ServiceVersion, Account, Space, Submission string
	}
	if len(body) > 64<<10 {
		return "unknown"
	}
	if _, err := strictJSON(body); err != nil {
		return "unknown"
	}
	if !canonicalSubmissionKeys(body) || json.Unmarshal(body, &proof) != nil || proof.Version != 1 || proof.JobID != job || proof.RequestDigest != digest || !ValidDigest(digest) || proof.ServiceVersion != s.ServiceVersion || proof.Account != s.Account || proof.Space != s.Space || proof.Submission != "accepted" && proof.Submission != "not_accepted" {
		return "unknown"
	}
	if proof.Submission == "not_accepted" {
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil || len(fields) != 7 {
			return "unknown"
		}
	}
	return proof.Submission
}

func canonicalSubmissionKeys(body []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return false
	}
	for name, value := range fields {
		for _, reserved := range []string{"version", "jobId", "requestDigest", "serviceVersion", "account", "space", "submission", "observedAt", "expiresAt", "decision", "quote", "source", "known", "amount", "currency", "unit", "capacityKnown", "available"} {
			if strings.EqualFold(name, reserved) && name != reserved {
				return false
			}
		}
		if name == "quote" && !canonicalSubmissionKeys(value) {
			return false
		}
	}
	return true
}
