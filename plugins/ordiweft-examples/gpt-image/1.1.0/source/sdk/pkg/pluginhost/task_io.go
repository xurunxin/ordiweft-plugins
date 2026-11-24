package pluginhost

const (
	MaxTaskNetworkCalls        = 1024
	MaxTaskRequestBytes  int64 = 256 << 20
	MaxTaskResponseBytes int64 = 256 << 20
)

// TaskIOPolicy is host-installed, frozen at original admission. Byte limits
// cover body data; private readback and local handle release do not replenish it.
type TaskIOPolicy struct {
	MaxCalls         int   `json:"maxCalls"`
	MaxRequestBytes  int64 `json:"maxRequestBytes"`
	MaxResponseBytes int64 `json:"maxResponseBytes"`
}

func DefaultTaskIOPolicy() TaskIOPolicy {
	return TaskIOPolicy{MaxCalls: MaxTaskNetworkCalls, MaxRequestBytes: MaxTaskRequestBytes, MaxResponseBytes: MaxTaskResponseBytes}
}

func (p TaskIOPolicy) Validate() error {
	if p.MaxCalls < 1 || p.MaxCalls > MaxTaskNetworkCalls || p.MaxRequestBytes < 0 || p.MaxRequestBytes > MaxTaskRequestBytes || p.MaxResponseBytes < 1 || p.MaxResponseBytes > MaxTaskResponseBytes {
		return ErrContract
	}
	return nil
}

func (p ConnectionPolicy) EffectiveTaskIO() TaskIOPolicy {
	if p.TaskIO != nil {
		return *p.TaskIO
	}
	return DefaultTaskIOPolicy()
}
