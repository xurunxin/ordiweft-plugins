// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

//go:embed release.template.json
var release []byte

func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("codex_original_result_unconfirmed\n")
		os.Exit(1)
	}
}

func run() (returned error) {
	debug.SetMemoryLimit(160 << 20)
	ctx := context.Background()
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	if task.Start.Compatibility == nil {
		return pluginhost.ErrIncompatible
	}
	if err = task.Host.Negotiate(ctx, *task.Start.Compatibility); err != nil {
		return err
	}
	defer func() {
		if returned != nil {
			_ = task.Host.ReportDiagnostic(ctx, "original-failure", pluginhost.Failure{Code: resultFailureCode(returned), Stage: "output", Submission: "unknown"})
		}
	}()
	var d pluginhost.Descriptor
	if json.Unmarshal(release, &d) != nil || d.Generation == nil {
		return errors.New("invalid_catalog")
	}
	g := task.Start.Generation
	if g.Provider != "codex" || g.Capability != "image.generate" || len(task.Start.Inputs) > 5 {
		return errors.New("invalid_original_request")
	}
	request := aigc.Request{}
	request.Payload, _ = json.Marshal(g)
	for i, input := range task.Start.Inputs {
		if input.Kind != "image" {
			return errors.New("invalid_reference")
		}
		request.Inputs = append(request.Inputs, aigc.Input{Kind: "image", Role: input.Role})
		var facts pluginhost.MediaInfo
		if task.Host.Call(ctx, fmt.Sprintf("inspect-%d", i), "media", "inspect", map[string]string{"id": input.Asset.ID}, &facts) != nil {
			return errors.New("reference_unconfirmed")
		}
	}
	if aigc.ValidateGeneration(*d.Generation, request) != nil {
		return errors.New("invalid_original_contract")
	}
	operation, endpoint := "generate", "generations"
	if len(task.Start.Inputs) > 0 {
		operation, endpoint = "edit", "edits"
	}
	metadata := map[string]any{"requestedModel": g.Model, "endpoint": endpoint, "transport": "host-services"}
	if err = task.Host.Call(ctx, "original-intent", "jobs", "checkpoint", map[string]any{"metadata": metadata}, nil); err != nil {
		return err
	}
	var response struct {
		Status int                    `json:"status"`
		Body   pluginhost.AssetHandle `json:"body"`
	}
	if err = task.Host.Call(ctx, "original-submit", "connections", "request", map[string]string{"operation": operation}, &response); err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		return errors.New("original_provider_rejected")
	}
	body, err := pluginhost.OpenAsset(ctx, task.Host, "original-response", response.Body, 256<<20)
	if err != nil {
		return err
	}
	defer body.Close()
	outputs := []string{}
	usage, err := readImages(body, 1, func(index int, image *decodedImage) error {
		output, e := pluginhost.StageOutputReaderAndReleaseBuffer(ctx, task.Host, fmt.Sprintf("original-output-%d", index), image.file, image.size, image.digest, maxImageBytes)
		if e == nil {
			outputs = append(outputs, output.ID)
		}
		return e
	}, metadata)
	if err != nil {
		return err
	}
	if !body.Verified() {
		return errors.New("original_response_unconfirmed")
	}
	if len(usage) > 0 {
		var value any
		if json.Unmarshal(usage, &value) != nil {
			return errors.New("invalid_usage")
		}
		metadata["usage"] = value
	}
	if reported, ok := metadata["model"]; ok {
		metadata["reportedModel"] = reported
		delete(metadata, "model")
	}
	if size, ok := metadata["size"]; ok {
		metadata["reportedSize"] = size
		delete(metadata, "size")
	}
	if format, ok := metadata["output_format"]; ok {
		metadata["outputFormat"] = format
		delete(metadata, "output_format")
	}
	return task.Complete(outputs, metadata)
}
