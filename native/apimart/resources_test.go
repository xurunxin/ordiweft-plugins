// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func imageUpload(t *testing.T, size int) attachment {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return attachment{ref: reference{Kind: "image"}, input: aigc.Input{Kind: "image", Data: data.Bytes()}}
}

func TestReferenceUploadReuseExpiryAndQuota(t *testing.T) {
	var calls atomic.Int32
	var quotaFull atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/uploads/images" ||
			r.URL.Query().Get("retention_days") != "3" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("unexpected upload %s %s", r.Method, r.URL)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if quotaFull.Load() {
			w.Header().Set("Retry-After", "3600")
			w.Header().Set("X-Upload-Quota-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"type":"rate_limit_exceeded"},"overage":{"enabled":false}}`)
			return
		}
		fmt.Fprintf(w, `{"url":"https://example.com/upload-%d.png","file_type":"image","bytes":%d,"retention_days":3,"expires_at":%d,"quota":{"limit":50,"used":%d,"remaining":%d,"reset_at":%d}}`,
			call, len(data), time.Now().Add(36*time.Hour).Unix(), call, 50-call, time.Now().Add(time.Hour).Unix())
	}))
	defer server.Close()
	state := t.TempDir()
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	p.stateDir, p.base = state, server.URL
	second, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	second.stateDir, second.base = state, server.URL
	who := aigc.Identity{User: "alice", Team: "team"}
	file := imageUpload(t, 4)
	var wg sync.WaitGroup
	for _, plugin := range []*plugin{p, second} {
		wg.Go(func() {
			urls, summary, err := plugin.referenceURLs(t.Context(), "fixture", who, []attachment{file})
			if err != nil || len(urls) != 1 || urls[0] != "https://example.com/upload-1.png" || summary.EarliestExpiry <= time.Now().Unix() {
				t.Errorf("concurrent process reference did not reuse upload: %v", err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate uploads: %d", calls.Load())
	}

	other := aigc.Identity{User: "bob", Team: "team"}
	if _, summary, err := second.referenceURLs(t.Context(), "fixture", other, []attachment{file}); err != nil || summary.Uploaded != 1 {
		t.Fatalf("reuse crossed owner scope: %v", err)
	}
	quotaFull.Store(true)
	missing := imageUpload(t, 5)
	if _, _, err := p.referenceURLs(t.Context(), "fixture", who, []attachment{missing}); err != errUploadQuota {
		t.Fatalf("expected a quota refusal, got %v", err)
	}
	if _, _, err := second.referenceURLs(t.Context(), "fixture", who, []attachment{missing}); err != errUploadQuota || calls.Load() != 3 {
		t.Fatalf("quota refusal caused another POST: %v (%d)", err, calls.Load())
	}
	if _, summary, err := second.referenceURLs(t.Context(), "fixture", who, []attachment{file}); err != nil || summary.Reused != 1 {
		t.Fatalf("quota refusal blocked a usable cached reference: %v", err)
	}
	ledgers, err := filepath.Glob(filepath.Join(state, "*.json"))
	if err != nil || len(ledgers) != 1 {
		t.Fatal("expected one persistent upload ledger", err)
	}
	ledger, err := readUploadLedger(ledgers[0])
	if err != nil {
		t.Fatal(err)
	}
	record := ledger.Records[uploadDigest(who, file.input)]
	record.ExpiresAt = time.Now().Add(-time.Second).Unix()
	ledger.Records[uploadDigest(who, file.input)] = record
	ledger.QuotaBlocked = time.Now().Add(-time.Second).Unix()
	if err = writeUploadLedger(ledgers[0], ledger); err != nil {
		t.Fatal(err)
	}
	quotaFull.Store(false)
	urls, summary, err := second.referenceURLs(t.Context(), "fixture", who, []attachment{file})
	if err != nil || summary.Uploaded != 1 || len(urls) != 1 || urls[0] != "https://example.com/upload-4.png" {
		t.Fatal("expired URL was not replaced after quota became available", err, urls)
	}
}

func TestUnconfirmedUploadDoesNotRepeatAcrossProcesses(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"url":"https://example.com/missing-receipt.png"}`)
	}))
	defer server.Close()
	state := t.TempDir()
	who := aigc.Identity{User: "alice", Team: "team"}
	file := imageUpload(t, 4)
	for range 2 {
		p, err := loadPlugin()
		if err != nil {
			t.Fatal(err)
		}
		p.stateDir, p.base = state, server.URL
		if _, _, err = p.referenceURLs(t.Context(), "fixture", who, []attachment{file}); err != errUploadUnconfirmed {
			t.Fatalf("invalid receipt was treated as usable: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("an unconfirmed upload was automatically repeated: %d", calls.Load())
	}
}

func TestReferenceUploadQuotaRefusalStopsGenerationOnWire(t *testing.T) {
	var generationPosts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/uploads/images" {
			generationPosts.Add(1)
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"type":"rate_limit_exceeded","message":"private upstream error"}}`)
	}))
	defer upstream.Close()
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	p.base, p.stateDir = upstream.URL, t.TempDir()
	file := imageUpload(t, 8)
	r := generation(t, "magi-2-reference", "video.generate", "A folded boat", nil, []aigc.Input{file.input})
	r.Identity = aigc.Identity{User: "alice", Team: "team"}
	in, err := json.Marshal(aigc.WireCall{APIVersion: 2, Method: "execute", Request: r})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := aigc.Serve(t.Context(), bytes.NewReader(in), &out, p); err != nil {
		t.Fatal(err)
	}
	var reply aigc.WireReply
	if json.Unmarshal(out.Bytes(), &reply) != nil || reply.Kind != "result" || reply.Error != "plugin request failed" ||
		string(reply.Result.Metadata) != `{"uploadErrorCode":"APIMART_UPLOAD_QUOTA"}` || generationPosts.Load() != 0 {
		t.Fatal("quota refusal did not preserve the safe cause or stop the generation", out.String())
	}
}

// This explicit opt-in exercises only APIMart file uploads against supplied
// synthetic fixtures. The second pass must reuse the returned URLs locally.
func TestLiveAPIMartReferenceUploads(t *testing.T) {
	dir := os.Getenv("APIMART_LIVE_UPLOAD_DIR")
	if dir == "" {
		t.Skip("requires explicitly selected live upload fixture directory")
	}
	key := os.Getenv("APIMART_API_KEY")
	if key == "" {
		t.Fatal("APIMART_API_KEY is unavailable")
	}
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	p.stateDir = filepath.Join(dir, "private-state")
	if err := os.Mkdir(p.stateDir, 0700); err != nil {
		t.Fatal("a new upload evidence directory is required")
	}
	who := aigc.Identity{User: "fixture", Team: "apimart-verification"}
	items := []struct{ kind, file string }{{"image", "reference.png"}, {"video", "reference.mp4"}, {"audio", "reference.wav"}}
	for index, item := range items {
		data, err := os.ReadFile(filepath.Join(dir, item.file))
		if err != nil {
			t.Fatal(err)
		}
		files := []attachment{{ref: reference{Kind: item.kind}, input: aigc.Input{Kind: item.kind, Data: data}}}
		urls, posted, err := p.referenceURLs(t.Context(), key, who, files)
		if err != nil || posted.Uploaded != 1 {
			t.Fatalf("%s upload failed: %v", item.kind, err)
		}
		again, cached, err := p.referenceURLs(t.Context(), key, who, files)
		if err != nil || cached.Reused != 1 || len(urls) != 1 || len(again) != 1 || urls[0] != again[0] {
			t.Fatalf("%s reference did not reuse its receipt: %v", item.kind, err)
		}
		row := struct {
			Kind   string         `json:"kind"`
			Upload *uploadSummary `json:"upload"`
			Reuse  *uploadSummary `json:"reuse"`
		}{item.kind, posted, cached}
		summary, _ := json.MarshalIndent(row, "", "  ")
		if err = os.WriteFile(filepath.Join(dir, item.kind+"-receipt-summary.json"), summary, 0600); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s upload receipt saved and reused", item.kind)
		if index < len(items)-1 && (posted.Quota == nil || !posted.Quota.Unlimited && (posted.Quota.Remaining == nil || *posted.Quota.Remaining < 1)) {
			t.Skip("supplier receipt did not confirm remaining free quota for another fixture")
		}
	}
}
