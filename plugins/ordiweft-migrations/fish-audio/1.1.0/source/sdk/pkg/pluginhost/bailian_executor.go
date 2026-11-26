package pluginhost

import (
	"slices"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const BailianExecutionProfile = "dashscope-exec-v1"

const BailianBlockBytes = 48 << 10
const BailianMaxRequestBytes = 256 << 20
const BailianMaxResponseBytes = 32 << 20
const BailianBridgeWorkspace = "plugin-runtime"

var bailianOperations = []string{"context", "write", "request", "read", "upload-audio", "clone-get", "clone-put", "checkpoint", "stage"}

func BailianExecutionRequirements() []Requirement {
	return []Requirement{
		{Service: "assets", Operations: []string{"read"}, Version: Version},
		{Service: "bailian", Operations: slices.Clone(bailianOperations), Version: Version},
	}
}

func ValidBailianExecutionDescriptor(d Descriptor) bool {
	if d.ProviderProfile != BailianExecutionProfile || d.PackageID != BailianPackage ||
		d.Runtime != "process" || d.Generation == nil || d.Generation.ID != "dashscope" ||
		d.Generation.Runtime != "builtin" || d.Generation.Version != d.Version ||
		len(d.Contributions) != 1 || d.Contributions[0] != (Contribution{ID: "dashscope", Kind: "provider"}) ||
		len(d.Requirements) != 2 || len(d.Definitions) != 0 || d.Compatibility != nil {
		return false
	}
	for _, want := range BailianExecutionRequirements() {
		if !slices.ContainsFunc(d.Requirements, func(got Requirement) bool {
			if got.Service != want.Service || got.Version != Version || len(got.Operations) != len(want.Operations) {
				return false
			}
			for _, operation := range want.Operations {
				if !slices.Contains(got.Operations, operation) {
					return false
				}
			}
			return true
		}) {
			return false
		}
	}
	return true
}

type BailianExecutionContext struct {
	Region   string      `json:"region"`
	Voice    *aigc.Voice `json:"voice,omitempty"`
	TaskID   string      `json:"taskId,omitempty"`
	Endpoint string      `json:"endpoint,omitempty"`
}

type BailianBlock struct {
	Offset int    `json:"offset"`
	Data   []byte `json:"data"`
}

type BailianRequest struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Method  string `json:"method"`
	Async   bool   `json:"async,omitempty"`
	Resolve bool   `json:"resolve,omitempty"`
}

type BailianResponse struct {
	Status int `json:"status"`
	Bytes  int `json:"bytes"`
}
