package pluginhost

import "context"

// JobContext contains only the original task's opaque correlation reference.
// It grants no business authority or ability to select another task.
type JobContext struct {
	JobReference string `json:"jobReference"`
}

func (c *Client) JobContext(ctx context.Context, id string) (JobContext, error) {
	if c == nil {
		return JobContext{}, ErrContract
	}
	var out JobContext
	if err := c.Call(ctx, id, "jobs", "context", struct{}{}, &out); err != nil {
		return JobContext{}, err
	}
	if !opaqueID.MatchString(out.JobReference) {
		return JobContext{}, ErrContract
	}
	return out, nil
}
