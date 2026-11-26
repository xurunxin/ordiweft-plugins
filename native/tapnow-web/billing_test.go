package tapnowweb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type observeTransport func(*http.Request) (*http.Response, error)

func (f observeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQuoteWaitsForWalletAfterEstimateFailure(t *testing.T) {
	c, _, _ := fixture(t, "2.5", false)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/billing/v2/wallet/balance":
			close(started)
			<-r.Context().Done()
			close(canceled)
			<-release
			return nil, r.Context().Err()
		case "/api/billing/v2/estimate":
			<-started
			return nil, errors.New("synthetic estimate failure")
		default:
			return nil, errors.New("unexpected request")
		}
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	r := Request{Kind: "image", Model: "nano-banana-flash", Mode: mode.ID, Parameters: defaults(mode)}
	done := make(chan error, 1)
	go func() { _, err := c.Quote(context.Background(), r, nil); done <- err }()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("wallet request was not canceled after estimate failed")
	}
	select {
	case <-done:
		t.Fatal("quote returned before wallet request finished")
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected estimate error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("quote did not finish after wallet request ended")
	}
}

func TestQuoteNeverUploadsOrCreatesAndPreparationIsCached(t *testing.T) {
	c, paid, creates := fixture(t, "2.5", false)
	uploads := 0
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "file/upload-url") {
			uploads++
		}
		return transport.RoundTrip(r)
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "image_to_image")
	r := Request{Kind: "image", Model: "nano-banana-flash", Mode: mode.ID, Parameters: defaults(mode), References: []tapnow.Reference{{Kind: "image", Role: "image", URL: "/files/private"}}, MaxCost: 3}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureImage(), Mime: "image/jpeg"}, nil
	}
	q, e := c.Quote(context.Background(), r, resolve)
	if e != nil || q.EstimatedCost != 2.5 || q.Wallet.Available != 10000 || uploads != 0 || paid.Load() != 0 || creates.Load() != 0 {
		t.Fatal(q, e, uploads)
	}
	if c.PrepareReferences(context.Background(), r, resolve) == nil || uploads != 0 {
		t.Fatal("upload without consent")
	}
	r.UploadConsent = true
	for j := 0; j < 2; j++ {
		if e = c.PrepareReferences(context.Background(), r, resolve); e != nil {
			t.Fatal(e)
		}
	}
	if uploads != 1 || paid.Load() != 0 || creates.Load() != 0 {
		t.Fatal("preparation repeated upload or generated", uploads)
	}
	if c.cachedURL(Identity{User: "another", Org: "fixture-org"}, Material{Bytes: fixtureImage()}) != "" {
		t.Fatal("upload cache leaked to another account")
	}
}

func TestPreviewQuoteCacheDoesNotAffectFreshQuotes(t *testing.T) {
	c, paid, creates := fixture(t, "2.5", false)
	var estimates, wallets atomic.Int32
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/billing/v2/estimate":
			estimates.Add(1)
		case "/api/billing/v2/wallet/balance":
			wallets.Add(1)
		}
		return transport.RoundTrip(r)
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	r := Request{Kind: "image", Model: "nano-banana-flash", Mode: mode.ID, Parameters: defaults(mode)}
	first, err := c.PreviewQuote(context.Background(), r, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.PreviewQuote(context.Background(), r, nil, false)
	if err != nil || second.EstimatedCost != first.EstimatedCost || second.ObservedAt != first.ObservedAt || estimates.Load() != 1 || wallets.Load() != 2 {
		t.Fatalf("cached preview: first=%+v second=%+v estimates=%d wallets=%d error=%v", first, second, estimates.Load(), wallets.Load(), err)
	}
	if _, err = c.PreviewQuote(context.Background(), r, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Quote(context.Background(), r, nil); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	c.accountRevision++
	c.mu.Unlock()
	if _, err = c.PreviewQuote(context.Background(), r, nil, false); err != nil {
		t.Fatal(err)
	}
	c.quoteMu.Lock()
	for k, v := range c.previewQuotes {
		v.at = time.Now().Add(-31 * time.Second)
		c.previewQuotes[k] = v
	}
	c.quoteMu.Unlock()
	if _, err = c.PreviewQuote(context.Background(), r, nil, false); err != nil {
		t.Fatal(err)
	}
	if estimates.Load() != 5 || wallets.Load() != 6 || paid.Load() != 0 || creates.Load() != 0 {
		t.Fatalf("fresh, account and expiry checks: estimates=%d wallets=%d paid=%d creates=%d", estimates.Load(), wallets.Load(), paid.Load(), creates.Load())
	}
}

func TestPreviewEstimateKeysAllBillingParameters(t *testing.T) {
	c, _, _ := fixture(t, "2.5", false)
	var estimates atomic.Int32
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/billing/v2/estimate" {
			estimates.Add(1)
		}
		return transport.RoundTrip(r)
	})
	for _, duration := range []float64{4, 4, 5} {
		_, _, err := c.previewEstimate(context.Background(), "video", map[string]any{"model": "fixture", "provider": "fixture", "times": 1, "duration": duration}, false, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if estimates.Load() != 2 {
		t.Fatalf("different billing parameters reused a quote: %d requests", estimates.Load())
	}
}

func TestVideoEditQuoteUsesVerifiedDurationWithoutChangingGeneration(t *testing.T) {
	c, paid, creates := fixture(t, "126", false)
	_, mode, _ := c.catalog.Find("seedance-2.5-draft", "video", "video_edit")
	r := Request{Kind: "video", Model: "seedance-2.5-draft", Mode: mode.ID, Prompt: "fixture", Parameters: defaults(mode), References: []tapnow.Reference{{Kind: "video", Role: "video", URL: "/files/private"}}, UploadConsent: true, VideoConsent: true, MaxCost: 150}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureVideo(4), Mime: "video/mp4", Duration: 30}, nil
	}
	if _, e := c.Quote(context.Background(), r, resolve); e == nil {
		t.Fatal("unprepared private video quoted")
	}
	if e := c.PrepareReferences(context.Background(), r, resolve); e != nil {
		t.Fatal(e)
	}
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	seenDuration := 0.0
	c.http.Transport = observeTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/billing/v2/estimate" {
			var b map[string]any
			_ = json.NewDecoder(req.Body).Decode(&b)
			seenDuration = number(obj(b["videoParams"])["duration"])
			data, _ := json.Marshal(b)
			req.Body = io.NopCloser(bytes.NewReader(data))
		}
		return transport.RoundTrip(req)
	})
	q, e := c.Quote(context.Background(), r, resolve)
	if e != nil || q.EstimatedCost != 126 || seenDuration != 4 || paid.Load() != 0 || creates.Load() != 0 {
		t.Fatal(q, e, seenDuration)
	}
	body, _, e := c.Prepare(r)
	if e != nil || body["duration"] != -1 {
		t.Fatal("billing mutated generation duration", body, e)
	}
}

func TestWalletRejectsMissingAndNonFiniteAmounts(t *testing.T) {
	for _, value := range []string{"", "NaN", "Infinity", "bad"} {
		t.Run(value, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"total": "10", "available": value, "onHold": "0"}})
			}))
			defer srv.Close()
			c := New("", "fixture-token", "", t.TempDir(), "")
			c.origin = srv.URL
			c.http = srv.Client()
			if _, e := c.Wallet(context.Background()); e == nil {
				t.Fatal("invalid balance accepted")
			}
		})
	}
}
