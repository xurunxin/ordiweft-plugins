// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"os"

	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("host_generation_failed\n")
		os.Exit(1)
	}
}

func run() error {
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	ctx := context.Background()
	var response struct {
		Status int
		Body   pluginhost.AssetHandle
	}
	if err = task.Host.Call(ctx, "original-submit", "connections", "request", map[string]any{"operation": "generate"}, &response); err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		return errors.New("provider_response_rejected")
	}
	if err = task.Host.Call(ctx, "checkpoint", "jobs", "checkpoint", map[string]any{"metadata": map[string]any{"originalCall": "original-submit"}}, nil); err != nil {
		return err
	}
	var decoded struct{ Asset pluginhost.AssetHandle }
	if err = task.Host.Call(ctx, "decode", "assets", "decode", map[string]any{"id": response.Body.ID, "encoding": "json-base64", "pointer": "/data/0/b64_json", "maxBytes": 1048576}, &decoded); err != nil {
		return err
	}
	var staged struct{ Output pluginhost.AssetHandle }
	if err = task.Host.Call(ctx, "stage", "outputs", "stage", map[string]any{"id": decoded.Asset.ID}, &staged); err != nil {
		return err
	}
	if err = task.Host.Call(ctx, "release", "assets", "release", map[string]any{"id": response.Body.ID}, nil); err != nil {
		return err
	}
	return task.Complete([]string{staged.Output.ID}, nil)
}
