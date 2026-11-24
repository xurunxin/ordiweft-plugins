package dashscope

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTaskReconciliationOnlyReadsAndNeverTreatsUnknownAsTerminal(t *testing.T) {
	calls := 0
	state := "UNKNOWN"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/tasks/accepted-task" {
			t.Fatal("reconciliation submitted work")
		}
		w.Write([]byte(`{"output":{"task_status":"` + state + `"}}`))
	}))
	defer s.Close()
	p := New()
	local(p, s)
	secrets := map[string]string{"API_KEY": "synthetic", "WORKSPACE_ID": "fixture"}
	terminal, status, e := p.ConfirmTask(context.Background(), "accepted-task", secrets)
	if e != nil || terminal || status != "UNKNOWN" {
		t.Fatal("unknown task released", e)
	}
	state = "SUCCEEDED"
	terminal, status, e = p.ConfirmTask(context.Background(), "accepted-task", secrets)
	if e != nil || !terminal || status != "SUCCEEDED" || calls != 2 {
		t.Fatal("confirmed task", e)
	}
}
