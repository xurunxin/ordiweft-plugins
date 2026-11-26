package tapnowweb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestGenerationFailureSeparatesRejectionFromUnconfirmedSubmission(t *testing.T) {
	for _, tc := range []struct {
		name, reply, reason, phase string
		path                       string
		status                     int
	}{
		{"wallet-boundary", `{"code":0,"data":{"total":"1","available":"1","onHold":"0"}}`, "insufficient_credits", "rejected", "/api/billing/v2/wallet/balance", 200},
		{"credits-at-submit", `{"code":10006,"message":"Bearer private-token https://private.example"}`, "insufficient_credits", "rejected", "/api/conversation/v1/generations/image", 200},
		{"team-quota-is-distinct", `{"code":10006,"data":{"error_code":"10007"}}`, "team_quota", "rejected", "/api/conversation/v1/generations/image", 402},
		{"explicit-upstream-review", `{"code":"content_violation","detail":"secret prompt"}`, "content_violation", "rejected", "/api/conversation/v1/generations/image", 400},
		{"ambiguous-gateway", `{"code":10006}`, "", "unknown", "/api/conversation/v1/generations/image", 502},
		{"acknowledgement-missing", `{"code":0,"data":{"result":{}}}`, "", "unknown", "/api/conversation/v1/generations/image", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := fixture(t, "2.5", false)
			transport := c.http.Transport
			submissions := 0
			c.http.Transport = observeTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/conversation/v1/generations/image" {
					submissions++
				}
				if req.URL.Path == tc.path {
					return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.reply)), Request: req}, nil
				}
				return transport.RoundTrip(req)
			})
			req := Request{Model: "nano-banana-flash", Mode: "text_to_image", Kind: "image", Prompt: "fixture", MaxCost: 3}
			_, ticket, err := c.Produce(context.Background(), "test-tapnow-failure", "alice", "test-canvas", req, nil, nil)
			receipt, ok := c.Receipt("test-tapnow-failure")
			if err == nil || !ok || receipt.Phase != tc.phase || len(ticket.TaskIDs) != 0 || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "secret prompt") {
				t.Fatalf("unexpected failure or receipt: %v %q %+v", err, receipt.Phase, ticket)
			}
			if reason, _, known := KnownFailure(err); reason != tc.reason || known != (reason != "") || errors.Is(err, ErrSubmissionUnconfirmed) != (tc.phase == "unknown") {
				t.Fatal("wrong rejection classification", err, reason, known)
			}
			if tc.path == "/api/billing/v2/wallet/balance" && submissions != 0 || tc.path != "/api/billing/v2/wallet/balance" && (submissions != 1 || ticket.NodeID == "") {
				t.Fatal("unexpected submit count or missing original node", submissions, ticket)
			}
		})
	}
}

func TestExplicitSeedanceReferenceReviewRejectionStopsBeforeGeneration(t *testing.T) {
	c, paid, created := fixture(t, "2.5", false)
	transport := c.http.Transport
	c.http.Transport = observeTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == assetReviewPath {
			body := `{"code":0,"data":{"reviews":[{"file_id":"fixture-file-png","status":"ASSET_REVIEW_STATUS_REJECTED","reject_reason":"private upstream message"}]}}`
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
		return transport.RoundTrip(req)
	})
	request := Request{Model: "seedance-2.5", Mode: "image_to_video", Kind: "video", Prompt: "fixture", MaxCost: 3,
		VideoConsent: true, UploadConsent: true, References: []tapnow.Reference{{Kind: "image", URL: "/files/owned-image"}}}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureImage(), Mime: "image/png"}, nil
	}
	_, _, err := c.Produce(context.Background(), "reference-rejected", "alice", "test-canvas", request, resolve, nil)
	reason, _, known := KnownFailure(err)
	if !known || reason != "provider_media_rejected" || paid.Load() != 0 || created.Load() != 0 || strings.Contains(err.Error(), "private upstream") {
		t.Fatal("known reference rejection was not contained", err, paid.Load(), created.Load())
	}
}
