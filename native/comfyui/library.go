package comfyui

import "gitea.nkit.top/xurx/Ordiweft/pkg/aigc"

const Provider = "comfyui"

func CollectionManifest(version string) aigc.Manifest {
	return aigc.Manifest{
		ID: Provider, Name: "ComfyUI 工作流", Version: version, APIVersion: 2, Runtime: "builtin", Auth: "server",
		Secrets: []string{}, Nodes: []aigc.Node{}, Models: []aigc.Model{},
		Scheduling: aigc.Scheduling{Independent: true, Concurrency: 1, MaxQueued: 20, Cancelable: false},
	}
}

// CollectionWorkflow changes the catalog identity while preserving the
// graph, reference roles, sampled defaults and transport.
func CollectionWorkflow(d Definition, version string, disabled bool) Definition {
	model := d.Manifest.Models[0]
	model.ID = d.Manifest.ID
	model.DefaultDisabled = disabled
	d.Manifest = CollectionManifest(version)
	d.Manifest.Models = []aigc.Model{model}
	return d
}
