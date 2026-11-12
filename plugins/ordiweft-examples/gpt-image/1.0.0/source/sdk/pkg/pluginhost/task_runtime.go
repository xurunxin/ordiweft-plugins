package pluginhost

const DefaultTaskRuntimeMS int64 = 60_000
const MaxTaskRuntimeMS int64 = 900_000

func ValidTaskRuntimeMS(value int64) bool {
	return value >= 1000 && value <= MaxTaskRuntimeMS
}

func (p ConnectionPolicy) EffectiveTaskRuntimeMS() int64 {
	if p.MaxRuntimeMS == 0 {
		return DefaultTaskRuntimeMS
	}
	return p.MaxRuntimeMS
}
