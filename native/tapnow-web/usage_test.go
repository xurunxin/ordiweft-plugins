package tapnowweb

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestUsageAccountingAndParsing(t *testing.T) {
	if tx, e := normalizeTransaction(rawTransaction{ID: "decimal", Time: "1791936000000", Amount: "-99999999999.123456"}); e != nil || tx.AmountDecimal != "-99999999999.123456" {
		t.Fatal("provider decimal lost precision", tx, e)
	}
	if tx, e := normalizeTransaction(rawTransaction{ID: "epoch", Time: "1791936000000", Amount: "1"}); e != nil || tx.At != 1791936000000 {
		t.Fatal("millisecond timestamp rejected", e)
	}
	if _, e := normalizeTransaction(rawTransaction{ID: "huge", Time: "2026-10-14T00:00:00Z", Amount: "1e300"}); e == nil {
		t.Fatal("unsafe aggregate magnitude accepted")
	}
	rows := []Transaction{}
	for _, r := range []rawTransaction{
		{ID: "1", Time: "2026-10-14T12:00:00Z", Description: "On-demand charge: Foo Image (Usage: 2)", Amount: "-12", Type: "TAPIES_TRANSACTION_TYPE_CONSUME", Status: "COMPLETED"},
		{ID: "2", Time: "2026-10-14T12:00:00Z", Description: "On-demand charge: Foo Image to Video (1080p) (Usage: 6.5)", Amount: "4", Type: "QUOTA_CONSUME", Status: "TAPIES_TRANSACTION_STATUS_COMPLETED"},
		{ID: "3", Time: "2026-10-14T12:00:00Z", Amount: "100", Type: "DEPOSIT", Status: "COMPLETED"},
		{ID: "4", Time: "2026-10-14T12:00:00Z", Amount: "999", Type: "CONSUME", Status: "FAILED"},
		{ID: "5", Time: "2026-10-14T12:00:00Z", Amount: "999", Type: "CONSUME", Status: "NOT_COMPLETED"},
	} {
		v, e := normalizeTransaction(r)
		if e != nil {
			t.Fatal(e)
		}
		rows = append(rows, v)
	}
	s := summarize(rows, "fixture")
	if s.Main.Consumed != 12 || s.Main.Deposited != 100 || s.Main.Net != 88 || s.Quota.Consumed != 4 || s.ImageCount != 2 || s.VideoSeconds != 6.5 || len(s.ByModel) != 1 {
		t.Fatalf("wrong wallet/usage %+v", s)
	}
	p, e := (usageCache{Transactions: rows, Complete: true}).page(1, 50, UsageFilter{From: "2026-10-14", To: "2026-10-14", Model: rows[1].Model})
	if e != nil || p.Total != 1 || p.Summary.VideoSeconds != 6.5 {
		t.Fatal("end date/filter incorrect")
	}
	if rows[1].Model != "Foo Image to Video (1080p)" || rows[1].Kind != "video" {
		t.Fatal("model variant lost")
	}
	if modelName(`TapNow Agent - Sonnet biz metric {}`) != "Sonnet" {
		t.Fatal("agent format")
	}
}
func TestUsageSyncPaginationCacheAndScope(t *testing.T) {
	fail := false
	org := "a"
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v1/users/me" {
			authJSON(w, Identity{"user", org})
			return
		}
		if fail {
			w.WriteHeader(502)
			return
		}
		page := r.URL.Query().Get("page")
		rows := []rawTransaction{}
		count := 100
		start := 0
		if page == "2" {
			count = 1
			start = 100
		}
		for i := range count {
			rows = append(rows, rawTransaction{ID: scalar(fmt.Sprint(i + start)), Time: "2026-10-14T00:00:00Z", Amount: "1", Type: "CONSUME", Status: "COMPLETED"})
		}
		authJSON(w, map[string]any{"transactions": rows, "total": "101"})
	})
	c.token = "fixture"
	p, e := c.SyncUsage(context.Background())
	if e != nil || !p.Complete || p.Total != 101 || p.Summary.Main.Consumed != 101 {
		t.Fatal("sync", e, p.Total)
	}
	fail = true
	if _, e = c.SyncUsage(context.Background()); e == nil {
		t.Fatal("failure accepted")
	}
	p, e = c.Usage(context.Background(), 3, 50, true, UsageFilter{})
	if e != nil || len(p.Transactions) != 1 {
		t.Fatal("cache discarded", e)
	}
	org = "b"
	if _, e = c.Usage(context.Background(), 1, 50, true, UsageFilter{}); e == nil {
		t.Fatal("other organization read cached data")
	}
}
func TestUsageDoesNotClaimMissingRowsComplete(t *testing.T) {
	c := authFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v1/users/me" {
			authJSON(w, Identity{"u", "o"})
			return
		}
		authJSON(w, map[string]any{"transactions": []rawTransaction{{ID: "1", Time: "2026-10-14T00:00:00Z", Amount: "1"}}, "total": "1000"})
	})
	c.token = "fixture"
	p, e := c.SyncUsage(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if p.Complete || p.Total != 1 {
		t.Fatal("partial rows reported complete")
	}
}
