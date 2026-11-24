package dashscope

import (
	"context"
	"time"
)

// Queue dispatch is smoothed before taking an execution slot. This final gate
// also smooths paid HTTP submissions after variable-duration downloads/uploads.
// It never retries a request, and all voice-enrollment operations share one key.
func (p *Plugin) waitSubmission(ctx context.Context, model string) error {
	interval := time.Duration(0)
	bucket := model
	for _, m := range catalog().Models {
		if m.ID == model && m.QueueLimit != nil {
			interval = time.Duration(m.QueueLimit.IntervalMS) * time.Millisecond
			bucket = m.QueueLimit.Bucket
			break
		}
	}
	if interval == 0 {
		return nil
	}
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		p.rateMu.Lock()
		if p.nextSubmit == nil {
			p.nextSubmit = map[string]time.Time{}
		}
		next := p.nextSubmit[bucket]
		now := time.Now()
		if !now.Before(next) {
			p.nextSubmit[bucket] = now.Add(interval)
			p.rateMu.Unlock()
			return nil
		}
		p.rateMu.Unlock()
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
