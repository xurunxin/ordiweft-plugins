package comfyui

import (
	"encoding/json"
	"math"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ExpectedWait is advisory host policy, never a failure deadline. A status-aware
// workflow continues polling until the provider reports a terminal state.
func (p *Plugin) ExpectedWait(r aigc.Request) time.Duration {
	const floor = 30 * time.Minute
	var g aigc.Generation
	if json.Unmarshal(r.Payload, &g) != nil {
		return floor
	}
	for _, m := range p.Manifest().Models {
		if m.ID != g.Model || m.Capability != "video.generate" {
			continue
		}
		params, err := aigc.NormalizeParameters(m, g.Parameters)
		if err != nil {
			return floor
		}
		seconds := float64(0)
		for key, f := range m.Fields {
			if f.Control != "duration" {
				continue
			}
			if v, ok := params[key].(float64); ok && !math.IsNaN(v) && !math.IsInf(v, 0) && v > seconds {
				seconds = v
			}
		}
		if seconds <= 0 {
			return 2 * time.Hour
		}
		// Clamp before converting to time.Duration to avoid numeric overflow.
		budget := float64(10*time.Minute) + math.Ceil(seconds)*float64(5*time.Minute)
		return max(floor, time.Duration(min(budget, float64(6*time.Hour))))
	}
	return floor
}
