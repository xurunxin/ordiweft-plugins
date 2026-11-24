package dashscope

import (
	"context"
	"errors"
	"net/http"
)

// A task ID was accepted, but no terminal state was observed. Never free capacity by guessing.
type PendingTaskError struct{}

func (*PendingTaskError) PendingTask() bool { return true }

// ReconciliationAuthority identifies the region and workspace used by task checks.
func (p *Plugin) ReconciliationAuthority(secrets map[string]string) (string, error) {
	return p.endpoint(secrets)
}

func (*PendingTaskError) Error() string {
	return "百炼异步任务未确认终态；请保留任务ID并只读核实"
}
func (p *Plugin) ConfirmTask(ctx context.Context, id string, secrets map[string]string) (bool, string, error) {
	if !safeID.MatchString(id) {
		return false, "", errors.New("invalid task ID")
	}
	base, e := p.endpoint(secrets)
	if e != nil {
		return false, "", e
	}
	out, e := p.call(ctx, http.MethodGet, base, "/tasks/"+id, secrets["API_KEY"], nil, false)
	if e != nil {
		return false, "", e
	}
	state := out.Output.TaskStatus
	switch state {
	case "SUCCEEDED", "FAILED", "CANCELED":
		return true, state, nil
	case "PENDING", "RUNNING", "UNKNOWN":
		return false, state, nil
	}
	return false, "", &Failure{}
}
