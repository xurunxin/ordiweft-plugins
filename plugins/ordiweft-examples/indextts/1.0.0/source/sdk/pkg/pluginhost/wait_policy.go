package pluginhost

import (
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// WaitPolicy observes an original task through one installed query operation.
type WaitPolicy struct {
	Operation    string   `json:"operation"`
	StatePointer string   `json:"statePointer"`
	Pending      []string `json:"pending"`
	Succeeded    []string `json:"succeeded"`
	Failed       []string `json:"failed"`
	IntervalMS   int64    `json:"intervalMs"`
	MaxAttempts  int      `json:"maxAttempts"`
	TimeoutMS    int64    `json:"timeoutMs"`
}

func (p WaitPolicy) Validate() error {
	if !aigc.IDPattern.MatchString(p.Operation) || p.IntervalMS < 100 || p.IntervalMS > 60000 || p.MaxAttempts < 1 || p.MaxAttempts > MaxTaskNetworkCalls || p.TimeoutMS < 1 || p.TimeoutMS > MaxTaskRuntimeMS || len(p.Pending) == 0 || len(p.Succeeded) == 0 || len(p.Failed) == 0 {
		return ErrContract
	}
	if _, err := pointerParts(p.StatePointer); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, group := range [][]string{p.Pending, p.Succeeded, p.Failed} {
		if len(group) > 16 {
			return ErrContract
		}
		for _, state := range group {
			if state == "" || len(state) > 64 || strings.TrimSpace(state) != state || strings.ContainsAny(state, "\x00\r\n") || seen[state] {
				return ErrContract
			}
			seen[state] = true
		}
	}
	return nil
}

func (p WaitPolicy) Classify(body []byte) (string, error) {
	if p.Validate() != nil || len(body) == 0 || len(body) > 64<<10 {
		return "", ErrContract
	}
	value, err := strictJSON(body)
	if err != nil {
		return "", err
	}
	state, present, err := pointerValue(value, p.StatePointer)
	text, ok := state.(string)
	if err != nil || !present || !ok {
		return "", ErrContract
	}
	for _, group := range []struct {
		values []string
		phase  string
	}{{p.Pending, "pending"}, {p.Succeeded, "succeeded"}, {p.Failed, "failed"}} {
		if slices.Contains(group.values, text) {
			return group.phase, nil
		}
	}
	return "", ErrContract
}
