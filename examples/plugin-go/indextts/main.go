// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

const maxAudioBytes = 40 << 20

type response struct {
	Status int                    `json:"status"`
	Body   pluginhost.AssetHandle `json:"body"`
}

type remoteJob struct {
	ID       string        `json:"id"`
	State    string        `json:"state"`
	Progress float64       `json:"progress"`
	Result   *remoteResult `json:"result"`
}

type remoteResult struct {
	AudioURL   string  `json:"audio_url"`
	SHA256     string  `json:"sha256"`
	Bytes      int64   `json:"bytes"`
	Backend    string  `json:"backend"`
	Duration   float64 `json:"duration_seconds"`
	SampleRate int     `json:"sample_rate"`
}

func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("indextts_adapter_unconfirmed\n")
		os.Exit(1)
	}
}

func run() error {
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	ctx := context.Background()
	generation := task.Start.Generation
	if generation.Provider != "indextts" || generation.Model != "IndexTTS-2.5" || strings.TrimSpace(generation.Prompt) == "" {
		return pluginhost.ErrContract
	}
	mode, _ := generation.Parameters["emotion_mode"].(string)
	operation, known := map[string]string{"speaker": "generate-speaker", "audio": "generate-audio", "vector": "generate-vector", "text": "generate-text", "auto": "generate-auto"}[mode]
	if !known {
		return pluginhost.ErrContract
	}
	if mode == "text" {
		text, _ := generation.Parameters["emotion_text"].(string)
		if strings.TrimSpace(text) == "" {
			return pluginhost.ErrContract
		}
	}
	if mode == "vector" {
		vector, ok := generation.Parameters["emotion_vector"].(map[string]any)
		if !ok {
			return pluginhost.ErrContract
		}
		var total float64
		for _, name := range []string{"happy", "angry", "sad", "afraid", "disgusted", "melancholic", "surprised", "calm"} {
			value, ok := vector[name].(float64)
			if !ok || value < 0 || value > 1 {
				return pluginhost.ErrContract
			}
			total += value
		}
		if total > 1.000001 {
			return pluginhost.ErrContract
		}
	}
	inputs := map[string]pluginhost.TaskInput{}
	for _, input := range task.Start.Inputs {
		if input.Kind != "audio" || input.Role != "speaker" && input.Role != "emotion_audio" || inputs[input.Role].Asset.ID != "" {
			return pluginhost.ErrContract
		}
		inputs[input.Role] = input
	}
	if inputs["speaker"].Asset.ID == "" || (mode == "audio") != (inputs["emotion_audio"].Asset.ID != "") {
		return pluginhost.ErrContract
	}
	var status struct{ Service, State, Backend string }
	if err = requestJSON(ctx, task, "status", "status", &status); err != nil || status.Service != "indextts-agent" || status.State != "ready" || status.Backend != "indextts" {
		return errors.New("indextts_service_unavailable")
	}
	var caps struct {
		Model       string `json:"model"`
		Backend     string `json:"backend"`
		TextEmotion bool   `json:"text_emotion_enabled"`
	}
	if err = requestJSON(ctx, task, "capabilities", "capabilities", &caps); err != nil || caps.Model != "IndexTeam/IndexTTS-2.5" || caps.Backend != status.Backend || (mode == "text" || mode == "auto") && !caps.TextEmotion {
		return errors.New("indextts_capabilities_unavailable")
	}
	for _, role := range []string{"speaker", "emotion_audio"} {
		input, present := inputs[role]
		if !present {
			continue
		}
		var uploaded response
		uploadOperation := "upload-speaker"
		if role == "emotion_audio" {
			uploadOperation = "upload-emotion"
		}
		if err = task.Host.Call(ctx, "original-upload-"+role, "transfers", "upload", map[string]string{"operation": uploadOperation}, &uploaded); err != nil {
			return err
		}
		var asset struct {
			ID string `json:"id"`
		}
		if err = decodeJSON(ctx, task, "uploaded-"+role, uploaded, &asset); err != nil || asset.ID != input.Asset.SHA256 {
			return errors.New("indextts_upload_unconfirmed")
		}
	}
	var submitted struct {
		Job      remoteJob `json:"job"`
		Replayed bool      `json:"replayed"`
	}
	if err = requestJSON(ctx, task, "original-submit", operation, &submitted); err != nil {
		return err
	}
	if !validJobID(submitted.Job.ID) {
		return errors.New("indextts_original_ticket_unconfirmed")
	}
	checkpoint := map[string]any{"remoteJobId": submitted.Job.ID, "state": submitted.Job.State, "progress": submitted.Job.Progress, "backend": caps.Backend, "replayed": submitted.Replayed}
	if err = task.Host.Call(ctx, "original-checkpoint", "jobs", "checkpoint", map[string]any{"metadata": checkpoint}, nil); err != nil {
		return err
	}
	job := submitted.Job
	if job.State == "queued" || job.State == "running" {
		var waited struct {
			Response response `json:"response"`
		}
		if err = task.Host.Call(ctx, "original-wait", "jobs", "wait", map[string]string{"name": "result"}, &waited); err != nil {
			return err
		}
		if err = decodeJSON(ctx, task, "original-observation", waited.Response, &job); err != nil || job.ID != submitted.Job.ID {
			return errors.New("indextts_original_observation_unconfirmed")
		}
	}
	result := job.Result
	if job.State != "succeeded" || result == nil || result.Backend != caps.Backend || result.Bytes < 1 || result.Bytes > maxAudioBytes || !pluginhost.ValidDigest(result.SHA256) || result.AudioURL != "/v1/jobs/"+job.ID+"/audio" {
		return errors.New("indextts_original_result_unconfirmed")
	}
	var downloaded response
	if err = task.Host.Call(ctx, "original-download", "connections", "request", map[string]string{"operation": "audio"}, &downloaded); err != nil || downloaded.Status != 200 {
		return errors.New("indextts_download_unconfirmed")
	}
	audio, err := pluginhost.ReadAsset(ctx, task.Host, "original-audio", downloaded.Body, maxAudioBytes)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(audio)
	if int64(len(audio)) != result.Bytes || hex.EncodeToString(digest[:]) != result.SHA256 || len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return errors.New("indextts_audio_integrity_unconfirmed")
	}
	var staged struct {
		Output pluginhost.AssetHandle `json:"output"`
	}
	if err = task.Host.Call(ctx, "original-stage", "outputs", "stage", map[string]string{"id": downloaded.Body.ID}, &staged); err != nil {
		return err
	}
	return task.Complete([]string{staged.Output.ID}, map[string]any{"remoteJobId": job.ID, "backend": caps.Backend, "testBackend": false, "durationSeconds": result.Duration, "sampleRate": result.SampleRate, "sha256": result.SHA256, "replayed": submitted.Replayed})
}

func validJobID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' && ch < 'a' || ch > 'f' {
			return false
		}
	}
	return true
}

func requestJSON(ctx context.Context, task *pluginhost.GenerationSession, callID, operation string, value any) error {
	var result response
	if err := task.Host.Call(ctx, callID, "connections", "request", map[string]string{"operation": operation}, &result); err != nil {
		return err
	}
	return decodeJSON(ctx, task, callID, result, value)
}

func decodeJSON(ctx context.Context, task *pluginhost.GenerationSession, prefix string, result response, value any) error {
	if result.Status < 200 || result.Status >= 300 {
		return errors.New("indextts_response_rejected")
	}
	var decoded struct {
		Asset pluginhost.AssetHandle `json:"asset"`
	}
	if err := task.Host.Call(ctx, prefix+"-decode", "assets", "decode", map[string]any{"id": result.Body.ID, "encoding": "json", "pointer": "", "maxBytes": 1 << 20}, &decoded); err != nil {
		return err
	}
	body, err := pluginhost.ReadAsset(ctx, task.Host, prefix+"-read", decoded.Asset, 1<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}
