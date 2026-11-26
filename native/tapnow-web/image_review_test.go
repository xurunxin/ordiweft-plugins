package tapnowweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestImageReviewUsesOriginalAssetAndNeverResubmitsUnknown(t *testing.T) {
	var posts atomic.Int32
	state := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public/v1/users/me":
			fmt.Fprint(w, `{"code":0,"data":{"user_id":"u","org_id":"o"}}`)
		case "/api/canvas/v1/asset-reviews":
			if r.Method == "POST" {
				posts.Add(1)
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if body["file_id"] != "file-1" || body["asset_type"] != "Image" {
					t.Error(body)
				}
				w.WriteHeader(502)
				return
			}
			if r.URL.Query().Get("file_ids") != "file-1" {
				t.Error("wrong file")
			}
			if state == "" {
				fmt.Fprint(w, `{"code":0,"data":{"reviews":[]}}`)
			} else {
				fmt.Fprintf(w, `{"code":0,"data":{"reviews":[{"file_id":"file-1","status":%q}]}}`, state)
			}
		default:
			t.Errorf("unexpected external operation %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	c := New("", "test-token", "", t.TempDir(), "")
	c.origin = srv.URL
	c.http = srv.Client()
	material := Material{Bytes: fixtureImage()}
	ctx := context.Background()
	result, err := c.ImageReview(ctx, material, false)
	if err != nil || result.Status != "unknown" {
		t.Fatal(result, err)
	}
	// A read above must not upload. Seed an already uploaded private material.
	if err = c.save("uploaded-"+materialKey(Identity{User: "u", Org: "o"}, material)+".json", []byte(`{"url":"https://files.tapnow.media/api/conversation/storage/uploads/file-1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ImageReview(ctx, material, true); err == nil {
		t.Fatal("lost response must remain unconfirmed")
	}
	fresh := New("", "test-token", "", c.dir, "")
	fresh.origin = srv.URL
	fresh.http = srv.Client()
	if _, err = fresh.ImageReview(ctx, material, true); err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 1 {
		t.Fatal("duplicate review", posts.Load())
	}
	state = "ASSET_REVIEW_STATUS_APPROVED"
	result, err = fresh.ImageReview(ctx, material, false)
	if err != nil || result.Status != "approved" {
		t.Fatal(result, err)
	}
}

func TestReviewAndGenerationFailureMessagesAreSafeAndActionable(t *testing.T) {
	cases := []struct{ detail, want string }{
		{"The input text may violate platform rules. Please modify the text and try again.", "请求未通过审核"},
		{"The generated video may contain sensitive content. Please modify the description and try again.", "生成的视频"},
		{"secret-token https://private.example", "请求未通过审核"},
	}
	for _, tc := range cases {
		message := (&TaskFailedError{Reason: "content_violation", Detail: tc.detail}).Error()
		if !strings.Contains(message, tc.want) || strings.Contains(message, "secret-token") {
			t.Fatal(message)
		}
	}
	for _, status := range []string{"PENDING", "PROCESSING", "APPROVED", "REJECTED"} {
		got := reviewResult(assetReview{Status: "ASSET_REVIEW_STATUS_" + status})
		if got.Status == "unknown" || got.Message == "" {
			t.Fatal(got)
		}
	}
	if got := reviewResult(assetReview{Status: "ASSET_REVIEW_STATUS_REJECTED", RejectReason: "ASSET_REJECT_REASON_INVALID_PARAMETER"}); got.Status != "rejected_invalid" {
		t.Fatal(got)
	}
}
