// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

//go:embed release.template.json
var release []byte

// Only presence is needed in the sandbox: decoding uses the original host-owned
// response. Avoid copying an 85MiB base64 string beside its 96MiB JSON buffer.
type audioPresent bool

func (present *audioPresent) UnmarshalJSON(value []byte) error {
	if string(value) == "null" {
		*present = false
		return nil
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return pluginhost.ErrContract
	}
	*present = len(value) > 2
	return nil
}

func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("seedaudio_original_result_unconfirmed\n")
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	var descriptor pluginhost.Descriptor
	if json.Unmarshal(release, &descriptor) != nil || descriptor.Generation == nil || len(descriptor.Generation.Models) != 1 {
		return pluginhost.ErrContract
	}
	g := task.Start.Generation
	if g.Provider != "doubao-audio" || g.Model != "seed-audio-1.0" || g.Capability != "music.generate" || len(task.Start.Inputs) != 0 {
		return pluginhost.ErrContract
	}
	g.Parameters, err = aigc.NormalizeParameters(descriptor.Generation.Models[0], g.Parameters)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(g)
	if err = aigc.ValidateGeneration(*descriptor.Generation, aigc.Request{Payload: payload}); err != nil {
		return err
	}
	original, err := task.Host.JobContext(ctx, "original-context")
	if err != nil {
		return err
	}
	metadata := map[string]any{"model": g.Model, "endpoint": "/api/v3/tts/create", "requestId": original.JobReference, "state": "submitting", "transport": "host-services"}
	if err = task.Host.Call(ctx, "original-phase", "jobs", "checkpoint", map[string]any{"metadata": metadata}, nil); err != nil {
		return err
	}
	var response struct {
		Status int                    `json:"status"`
		Body   pluginhost.AssetHandle `json:"body"`
	}
	if err = task.Host.Call(ctx, "original-submit", "connections", "request", map[string]string{"operation": "generate"}, &response); err != nil || response.Status < 200 || response.Status >= 300 {
		return errors.New("original_response_unconfirmed")
	}
	body, err := pluginhost.ReadAsset(ctx, task.Host, "original-response", response.Body, 96<<20)
	var result struct {
		Code             *int         `json:"code"`
		Audio            audioPresent `json:"audio"`
		Duration         float64      `json:"duration"`
		OriginalDuration float64      `json:"original_duration"`
	}
	if err != nil || json.Unmarshal(body, &result) != nil || result.Code != nil && *result.Code != 0 && *result.Code != 20000000 {
		return errors.New("original_result_invalid")
	}
	var source pluginhost.AssetHandle
	if result.Audio {
		var decoded struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		if err = task.Host.Call(ctx, "original-audio", "assets", "decode", pluginhost.AssetDecode{ID: response.Body.ID, Encoding: "json-base64", Pointer: "/audio", MaxBytes: 64 << 20}, &decoded); err != nil {
			return err
		}
		source = decoded.Asset
	} else {
		var downloaded struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		if err = task.Host.Call(ctx, "original-download", "transfers", "download", map[string]string{"name": "audio", "callId": "original-submit"}, &downloaded); err != nil {
			return err
		}
		source = downloaded.Asset
	}
	if source.Bytes < 1 || source.Bytes > 64<<20 {
		return errors.New("original_audio_invalid")
	}
	if err = task.Host.Call(ctx, "release-original-response", "assets", "release", map[string]string{"id": response.Body.ID}, nil); err != nil {
		return err
	}
	var prefix struct {
		Data []byte `json:"data"`
	}
	if err = task.Host.Call(ctx, "original-audio-prefix", "assets", "read", map[string]any{"id": source.ID, "offset": 0, "limit": min(source.Bytes, 12)}, &prefix); err != nil {
		return err
	}
	if len(prefix.Data) == 12 && string(prefix.Data[:4]) == "RIFF" && string(prefix.Data[8:]) == "WAVE" {
		var derived struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		if err = task.Host.Call(ctx, "original-wav-normalization", "media", "derive", map[string]any{"id": source.ID, "operation": "audio.normalize-wav", "version": 1}, &derived); err != nil {
			return err
		}
		if err = task.Host.Call(ctx, "release-original-decoded", "assets", "release", map[string]string{"id": source.ID}, nil); err != nil {
			return err
		}
		source = derived.Asset
	}
	var staged struct {
		Output pluginhost.AssetHandle `json:"output"`
	}
	if err = task.Host.Call(ctx, "original-stage", "outputs", "stage", map[string]string{"id": source.ID}, &staged); err != nil {
		return err
	}
	usage := map[string]any{}
	if result.Duration > 0 && result.Duration <= 3600 {
		usage["duration"] = result.Duration
	}
	if result.OriginalDuration > 0 && result.OriginalDuration <= 120 {
		usage["original_duration"] = result.OriginalDuration
	}
	metadata["state"], metadata["usage"] = "received", usage
	return task.Complete([]string{staged.Output.ID}, metadata)
}
