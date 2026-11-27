// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const (
	uploadRetention = 72 * time.Hour
	uploadRunway    = 15 * time.Minute
	resourceMax     = 32 << 20
)

var (
	errUploadQuota       = errors.New("APIMART_UPLOAD_QUOTA")
	errUploadThrottle    = errors.New("APIMART_UPLOAD_THROTTLED")
	errUploadUnconfirmed = errors.New("APIMART_UPLOAD_UNCONFIRMED")
	errUploadInvalid     = errors.New("APIMART_UPLOAD_INVALID")
)

type uploadQuota struct {
	Limit       *int  `json:"limit,omitempty"`
	Used        *int  `json:"used,omitempty"`
	Remaining   *int  `json:"remaining,omitempty"`
	WindowHours int   `json:"window_hours,omitempty"`
	ResetAt     int64 `json:"reset_at,omitempty"`
	Unlimited   bool  `json:"unlimited,omitempty"`
	ObservedAt  int64 `json:"observedAt,omitempty"`
}

// URLs stay in the adapter's private state, keyed by authenticated owner and
// exact source bytes. Only counts and expiry times appear in job metadata.
type uploadRecord struct {
	URL       string `json:"url,omitempty"`
	ExpiresAt int64  `json:"expiresAt"`
}

type uploadLedger struct {
	Records       map[string]uploadRecord `json:"records"`
	Quota         *uploadQuota            `json:"quota,omitempty"`
	QuotaBlocked  int64                   `json:"quotaBlocked,omitempty"`
	ThrottleUntil int64                   `json:"throttleUntil,omitempty"`
}

type uploadSummary struct {
	Uploaded       int           `json:"uploaded"`
	Reused         int           `json:"reused"`
	EarliestExpiry int64         `json:"earliestExpiry"`
	Quota          *uploadQuota  `json:"quota,omitempty"`
	ChargesUSD     []json.Number `json:"chargesUsd,omitempty"`
}

type uploadReceipt struct {
	URL           string       `json:"url"`
	FileType      string       `json:"file_type"`
	Bytes         int          `json:"bytes"`
	RetentionDays int          `json:"retention_days"`
	ExpiresAt     int64        `json:"expires_at"`
	Quota         *uploadQuota `json:"quota"`
	Overage       *struct {
		Charged bool        `json:"charged"`
		CostUSD json.Number `json:"cost_usd"`
	} `json:"overage"`
}

type uploadInput struct {
	slot      int
	digest    string
	extension string
	input     aigc.Input
}

func (p *plugin) referenceURLs(ctx context.Context, key string, identity aigc.Identity, files []attachment) ([]string, *uploadSummary, error) {
	urls := make([]string, len(files))
	var need []uploadInput
	for slot, file := range files {
		mime, extension, err := referenceType(file.input)
		if err != nil {
			return nil, nil, errUploadInvalid
		}
		limit := file.ref.MaxBytes
		if !file.ref.Inline {
			uploadLimit := map[string]int{"image": 30_000_000, "video": 50_000_000, "audio": 15_000_000}[file.input.Kind]
			if limit == 0 || uploadLimit < limit {
				limit = uploadLimit
			}
		}
		if limit != 0 && len(file.input.Data) > limit {
			return nil, nil, errUploadInvalid
		}
		switch {
		case len(file.input.Data) == 0:
			urls[slot] = file.input.URL
		case file.ref.Inline:
			urls[slot] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(file.input.Data)
		default:
			need = append(need, uploadInput{
				slot: slot, input: file.input, extension: extension,
				digest: uploadDigest(identity, file.input),
			})
		}
	}
	if len(need) == 0 {
		return urls, nil, nil
	}
	if identity.User == "" || !filepath.IsAbs(p.stateDir) {
		return nil, nil, errUploadInvalid
	}
	summary, err := p.resolveUploads(ctx, key, need, urls)
	if err != nil {
		return nil, nil, err
	}
	return urls, &summary, nil
}

func uploadDigest(identity aigc.Identity, input aigc.Input) string {
	h := sha256.New()
	scope, _ := json.Marshal([]string{identity.Scope, identity.Team, identity.User, input.Kind})
	h.Write(scope)
	h.Write(input.Data)
	return hex.EncodeToString(h.Sum(nil))
}

func (p *plugin) resolveUploads(ctx context.Context, key string, uploads []uploadInput, urls []string) (uploadSummary, error) {
	var summary uploadSummary
	if err := os.MkdirAll(p.stateDir, 0700); err != nil {
		return summary, errUploadInvalid
	}
	sum := sha256.Sum256([]byte(key))
	account := filepath.Join(p.stateDir, hex.EncodeToString(sum[:]))
	lock, err := lockUploads(ctx, account+".lock")
	if err != nil {
		return summary, errUploadInvalid
	}
	defer lock.Close()
	index := account + ".json"
	ledger, err := readUploadLedger(index)
	if err != nil {
		return summary, errUploadUnconfirmed
	}
	now := time.Now()
	changed := ledger.expire(now.Unix())
	if changed && writeUploadLedger(index, ledger) != nil {
		return summary, errUploadInvalid
	}
	// Check all records up front. An ambiguous earlier upload for one file
	// must not cause this job to consume quota uploading the other files.
	for _, item := range uploads {
		old, exists := ledger.Records[item.digest]
		if exists && old.URL == "" {
			return summary, errUploadUnconfirmed
		}
		if !old.usable(now) {
			if ledger.QuotaBlocked > now.Unix() {
				return summary, errUploadQuota
			}
			if ledger.ThrottleUntil > now.Unix() {
				return summary, errUploadThrottle
			}
		}
	}
	for _, item := range uploads {
		now = time.Now()
		current := ledger.Records[item.digest]
		if current.usable(now) {
			summary.Reused++
		} else {
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			// Persist the intent before the POST. A crash or malformed reply
			// leaves it pending for the documented 3-day retention window.
			ledger.Records[item.digest] = uploadRecord{ExpiresAt: now.Add(uploadRetention).Unix()}
			if writeUploadLedger(index, ledger) != nil {
				return summary, errUploadInvalid
			}
			receipt, uploadErr := p.submitUpload(ctx, key, item, &ledger)
			if uploadErr != nil {
				if uploadErr != errUploadUnconfirmed {
					delete(ledger.Records, item.digest)
				}
				if writeUploadLedger(index, ledger) != nil {
					return summary, errUploadUnconfirmed
				}
				return summary, uploadErr
			}
			current = uploadRecord{URL: receipt.URL, ExpiresAt: receipt.ExpiresAt}
			ledger.Records[item.digest] = current
			if writeUploadLedger(index, ledger) != nil {
				return summary, errUploadUnconfirmed
			}
			summary.Uploaded++
			if receipt.Overage != nil && receipt.Overage.Charged && receipt.Overage.CostUSD != "" {
				summary.ChargesUSD = append(summary.ChargesUSD, receipt.Overage.CostUSD)
			}
		}
		urls[item.slot] = current.URL
		if summary.EarliestExpiry == 0 || summary.EarliestExpiry > current.ExpiresAt {
			summary.EarliestExpiry = current.ExpiresAt
		}
	}
	summary.Quota = ledger.Quota
	return summary, nil
}

func (r uploadRecord) usable(now time.Time) bool {
	return r.ExpiresAt > now.Add(uploadRunway).Unix() && httpsURL(r.URL)
}

func (l *uploadLedger) expire(now int64) bool {
	changed := false
	for id, entry := range l.Records {
		if entry.ExpiresAt <= now {
			delete(l.Records, id)
			changed = true
		}
	}
	if l.Quota != nil && l.Quota.ResetAt > 0 && l.Quota.ResetAt <= now {
		l.Quota = nil
		changed = true
	}
	return changed
}

func readUploadLedger(path string) (uploadLedger, error) {
	ledger := uploadLedger{Records: map[string]uploadRecord{}}
	data, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return ledger, err
	}
	defer data.Close()
	reader := json.NewDecoder(io.LimitReader(data, resourceMax+1))
	if reader.Decode(&ledger) != nil || reader.Decode(new(any)) != io.EOF || ledger.Records == nil {
		return ledger, errUploadUnconfirmed
	}
	return ledger, nil
}

func writeUploadLedger(path string, ledger uploadLedger) error {
	payload, err := json.Marshal(ledger)
	if err != nil || len(payload) > resourceMax {
		return errUploadInvalid
	}
	file, err := os.CreateTemp(filepath.Dir(path), "upload-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(payload)
	syncErr := file.Sync()
	if err = errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (p *plugin) submitUpload(ctx context.Context, key string, item uploadInput, ledger *uploadLedger) (uploadReceipt, error) {
	var receipt uploadReceipt
	body, contentType, err := multipartFile(item.input.Data, item.extension, nil)
	if err != nil {
		return receipt, errUploadInvalid
	}
	names := map[string]string{"image": "images", "video": "videos", "audio": "audios"}
	response, err := p.doRequest(ctx, http.MethodPost, "/v1/uploads/"+names[item.input.Kind]+"?retention_days=3", key, contentType, body)
	if err != nil {
		return receipt, errUploadUnconfirmed
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return receipt, errUploadUnconfirmed
	}
	now := time.Now()
	status := response.StatusCode
	if status == http.StatusOK {
		if json.Unmarshal(payload, &receipt) != nil || !httpsURL(receipt.URL) ||
			receipt.FileType != item.input.Kind || receipt.Bytes != len(item.input.Data) ||
			receipt.RetentionDays != 3 || receipt.ExpiresAt <= now.Add(uploadRunway).Unix() {
			return receipt, errUploadUnconfirmed
		}
		applyQuota(ledger, receipt.Quota, response.Header)
		return receipt, nil
	}
	var rejection struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(payload, &rejection)
	applyQuota(ledger, nil, response.Header)
	retry := retryAt(response.Header.Get("Retry-After"), now)
	if status == http.StatusTooManyRequests {
		if rejection.Error.Type == "rate_limit_error" || rejection.Error.Code == "rate_limit_error" {
			ledger.ThrottleUntil = max(retry, now.Add(time.Minute).Unix())
			return receipt, errUploadThrottle
		}
		if retry == 0 && ledger.Quota != nil {
			retry = ledger.Quota.ResetAt
		}
		ledger.QuotaBlocked = retry
		return receipt, errUploadQuota
	}
	switch status {
	case http.StatusPaymentRequired:
		return receipt, errUploadQuota
	case http.StatusServiceUnavailable:
		ledger.ThrottleUntil = retry
		return receipt, errUploadThrottle
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge:
		return receipt, errUploadInvalid
	default:
		return receipt, errUploadUnconfirmed
	}
}

func applyQuota(ledger *uploadLedger, quota *uploadQuota, header http.Header) {
	if quota != nil {
		quota.ObservedAt = time.Now().Unix()
		ledger.Quota = quota
		return
	}
	remaining, err := strconv.Atoi(header.Get("X-Upload-Quota-Remaining"))
	if err != nil || remaining < 0 {
		return
	}
	limit, err := strconv.Atoi(header.Get("X-Upload-Quota-Limit"))
	reset, _ := strconv.ParseInt(header.Get("X-Upload-Quota-Reset"), 10, 64)
	ledger.Quota = &uploadQuota{Remaining: &remaining, ResetAt: reset, ObservedAt: time.Now().Unix()}
	if err == nil && limit >= remaining {
		ledger.Quota.Limit = &limit
	}
}

func retryAt(value string, now time.Time) int64 {
	if seconds, err := strconv.ParseInt(value, 10, 32); seconds > 0 && err == nil {
		return now.Add(time.Duration(seconds) * time.Second).Unix()
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Unix()
	}
	return 0
}
