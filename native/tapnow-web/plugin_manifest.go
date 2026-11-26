package tapnowweb

import (
	_ "embed"
	"encoding/json"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

//go:embed manifest.json
var pluginManifest []byte

func PluginManifest() (aigc.Manifest, error) {
	var manifest aigc.Manifest
	err := json.Unmarshal(pluginManifest, &manifest)
	return manifest, err
}
