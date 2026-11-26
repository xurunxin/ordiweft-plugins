// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

const maxAudio = 40 << 20

var voiceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var errUnconfirmed = errors.New("fish_audio_result_unconfirmed")

func main() {
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err == nil {
		err = run(context.Background(), task)
	}
	if err != nil {
		_, _ = os.Stderr.WriteString(errUnconfirmed.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, task *pluginhost.GenerationSession) error {
	if task.Start.Generation.Provider != "fish-audio" || task.Start.Compatibility == nil {
		return pluginhost.ErrContract
	}
	if err := task.Host.Negotiate(ctx, *task.Start.Compatibility); err != nil {
		return err
	}
	model := task.Start.Generation.Model
	switch model {
	case "drama-3", "s2.1-pro", "s2.1-pro-free":
		return speak(ctx, task)
	case "voice-design-1":
		return design(ctx, task)
	case "voice-clone":
		return clone(ctx, task)
	default:
		return pluginhost.ErrContract
	}
}

func request(ctx context.Context, task *pluginhost.GenerationSession, operation string) (pluginhost.AssetHandle, error) {
	var response struct {
		Status int                    `json:"status"`
		Body   pluginhost.AssetHandle `json:"body"`
	}
	err := task.Host.Call(ctx, "original-submit", "connections", "request", map[string]string{"operation": operation}, &response)
	if err != nil || response.Status < 200 || response.Status >= 300 || response.Body.Validate() != nil {
		return pluginhost.AssetHandle{}, errUnconfirmed
	}
	return response.Body, nil
}

func speak(ctx context.Context, task *pluginhost.GenerationSession) error {
	operation := "speech"
	if len(task.Start.Inputs) == 1 {
		operation = "zero-shot"
	}
	if len(task.Start.Inputs) > 1 {
		return pluginhost.ErrContract
	}
	source, err := request(ctx, task, operation)
	if err != nil || source.Bytes > maxAudio {
		return errUnconfirmed
	}
	var prefix struct {
		Data []byte `json:"data"`
	}
	if err = task.Host.Call(ctx, "audio-header", "assets", "read", map[string]any{"id": source.ID, "offset": 0, "limit": min(source.Bytes, 12)}, &prefix); err != nil {
		return err
	}
	if len(prefix.Data) == 12 && string(prefix.Data[:4]) == "RIFF" && string(prefix.Data[8:]) == "WAVE" {
		var derived struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		if err = task.Host.Call(ctx, "normalize-wav", "media", "derive", map[string]any{"id": source.ID, "operation": "audio.normalize-wav", "version": 1}, &derived); err != nil {
			return err
		}
		if err = task.Host.Call(ctx, "release-original", "assets", "release", map[string]string{"id": source.ID}, nil); err != nil {
			return err
		}
		source = derived.Asset
	}
	staged, err := stage(ctx, task, "stage-speech", source, nil)
	if err != nil {
		return err
	}
	return task.Complete([]string{staged}, map[string]any{
		"model": task.Start.Generation.Model, "endpoint": "/v1/tts", "zeroShot": operation == "zero-shot",
		"usage": map[string]int{"utf8_bytes": len(task.Start.Generation.Prompt)},
	})
}

func design(ctx context.Context, task *pluginhost.GenerationSession) error {
	source, err := request(ctx, task, "design")
	if err != nil {
		return err
	}
	body, err := pluginhost.ReadAsset(ctx, task.Host, "design-receipt", source, 64<<20)
	var result struct {
		Candidates []struct {
			Index int `json:"index"`
		} `json:"candidates"`
	}
	if err != nil || json.Unmarshal(body, &result) != nil || len(result.Candidates) < 1 || len(result.Candidates) > 4 {
		return errUnconfirmed
	}
	order := make([]int, len(result.Candidates))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(a, b int) bool { return result.Candidates[order[a]].Index < result.Candidates[order[b]].Index })
	outputs, indices := []string{}, []int{}
	for _, index := range order {
		prefix := fmt.Sprint("candidate-", index)
		var decoded struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		input := pluginhost.AssetDecode{ID: source.ID, Encoding: "json-base64", Pointer: fmt.Sprintf("/candidates/%d/audio_base64", index), MaxBytes: maxAudio, MaxSourceBytes: 64 << 20}
		if err = task.Host.Call(ctx, prefix+"-decode", "assets", "decode", input, &decoded); err != nil {
			return err
		}
		output, err := stage(ctx, task, prefix+"-stage", decoded.Asset, nil)
		if err != nil {
			return err
		}
		outputs, indices = append(outputs, output), append(indices, result.Candidates[index].Index)
	}
	return task.Complete(outputs, map[string]any{"model": "voice-design-1", "candidateIndices": indices, "persistentVoice": false})
}

func clone(ctx context.Context, task *pluginhost.GenerationSession) error {
	if len(task.Start.Inputs) < 1 || len(task.Start.Inputs) > 20 {
		return pluginhost.ErrContract
	}
	source, err := request(ctx, task, "clone")
	if err != nil {
		return err
	}
	body, err := pluginhost.ReadAsset(ctx, task.Host, "original-voice-receipt", source, 2<<20)
	var result struct {
		ID    string `json:"_id"`
		State string `json:"state"`
	}
	if err != nil || json.Unmarshal(body, &result) != nil || !voiceID.MatchString(result.ID) {
		return errUnconfirmed
	}
	metadata := map[string]string{"voiceId": result.ID, "state": result.State, "phase": "voice-created", "visibility": "private"}
	if err = task.Host.Call(ctx, "save-voice-identity", "jobs", "checkpoint", map[string]any{"metadata": metadata}, nil); err != nil {
		return err
	}
	if result.State != "created" && result.State != "trained" {
		return errUnconfirmed
	}
	label, _ := task.Start.Generation.Parameters["title"].(string)
	voice := &aigc.Voice{Provider: "fish-audio", ID: result.ID, Kind: "clone", Label: strings.TrimSpace(label), CompatibleModels: []string{"drama-3", "s2.1-pro", "s2.1-pro-free"}}
	output, err := stage(ctx, task, "original-sample", task.Start.Inputs[0].Asset, voice)
	if err != nil {
		return err
	}
	return task.Complete([]string{output}, metadata)
}

func stage(ctx context.Context, task *pluginhost.GenerationSession, id string, source pluginhost.AssetHandle, voice *aigc.Voice) (string, error) {
	var response struct {
		Output pluginhost.AssetHandle `json:"output"`
	}
	input := map[string]any{"id": source.ID}
	if voice != nil {
		input["voice"] = voice
	}
	if err := task.Host.Call(ctx, id, "outputs", "stage", input, &response); err != nil {
		return "", err
	}
	if response.Output.Validate() != nil {
		return "", pluginhost.ErrContract
	}
	return response.Output.ID, nil
}
