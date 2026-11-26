package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/breezetts"
)

func main() {
	channel := flag.String("channel", "local", "local or api")
	address := flag.String("url", "", "trusted inference service base URL")
	digest := flag.String("model-digest", "", "expected local checkpoint digest")
	manifest := flag.Bool("manifest", false, "print SDK manifest")
	flag.Parse()
	if *channel != "local" && *channel != "api" {
		os.Exit(2)
	}
	p := &breezetts.Plugin{Online: *channel == "api", URL: *address, ModelDigest: *digest}
	if *manifest {
		_ = json.NewEncoder(os.Stdout).Encode(p.Manifest())
		return
	}
	if aigc.Serve(context.Background(), os.Stdin, os.Stdout, p) != nil {
		os.Exit(1)
	}
}
