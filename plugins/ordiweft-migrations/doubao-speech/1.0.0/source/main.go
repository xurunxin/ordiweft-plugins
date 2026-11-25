// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

const (
	endpoint         = "/api/v3/tts/unidirectional"
	maxResponseBytes = 96 << 20
	maxAudioBytes    = 64 << 20
)

//go:embed release.template.json
var release []byte

var errUnconfirmed = errors.New("speech_result_unconfirmed")

func main() {
	if serve(os.Stdin, os.Stdout) != nil {
		_, _ = os.Stderr.WriteString(errUnconfirmed.Error() + "\n")
		os.Exit(1)
	}
}

func serve(reader io.Reader, writer io.Writer) error {
	ctx := context.Background()
	task, err := pluginhost.OpenGeneration(reader, writer)
	if err != nil {
		return err
	}
	var descriptor pluginhost.Descriptor
	if json.Unmarshal(release, &descriptor) != nil || descriptor.Generation == nil {
		return pluginhost.ErrContract
	}
	if task.Start.Compatibility == nil {
		return pluginhost.ErrIncompatible
	}
	if err = task.Host.Negotiate(ctx, *task.Start.Compatibility); err != nil {
		return err
	}
	g := task.Start.Generation
	if g.Provider != "doubao-speech" || g.Model != "seed-tts-2.0" || g.Capability != "speech.generate" || len(task.Start.Inputs) != 0 {
		return pluginhost.ErrContract
	}
	payload, err := json.Marshal(g)
	if err != nil {
		return err
	}
	if err = aigc.ValidateGeneration(*descriptor.Generation, aigc.Request{Payload: payload}); err != nil {
		return err
	}
	original, err := task.Host.JobContext(ctx, "original-context")
	if err != nil {
		return err
	}
	metadata := map[string]any{
		"endpoint": endpoint, "model": g.Model, "requestId": original.JobReference,
		"transport": "host-services", "state": "submitting",
	}
	if err = task.Host.Call(ctx, "original-phase", "jobs", "checkpoint", map[string]any{"metadata": metadata}, nil); err != nil {
		return err
	}
	stream, err := task.Host.OpenStream(ctx, "original-submit", "generate")
	if err != nil {
		return err
	}
	defer stream.Close()
	audio, words, err := readSpeech(stream)
	if err != nil || stream.Status() < 200 || stream.Status() >= 300 {
		return errUnconfirmed
	}
	if err = stream.Close(); err != nil {
		return err
	}

	digest := sha256.Sum256(audio)
	output, err := pluginhost.StageOutputReaderAndReleaseBuffer(
		ctx, task.Host, "original-audio", bytes.NewReader(audio), int64(len(audio)), hex.EncodeToString(digest[:]), maxAudioBytes,
	)
	if err != nil {
		return err
	}
	usage := map[string]any{}
	if words > 0 {
		usage["text_words"] = words
	}
	metadata["state"], metadata["usage"] = "received", usage
	return task.Complete([]string{output.ID}, metadata)
}

func readSpeech(source io.Reader) ([]byte, float64, error) {
	limited := &io.LimitedReader{R: source, N: maxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	var audio []byte
	var words float64
	finished := false
	for {
		var chunk struct {
			Code  *int   `json:"code"`
			Data  string `json:"data"`
			Usage struct {
				Words float64 `json:"text_words"`
			} `json:"usage"`
		}
		err := decoder.Decode(&chunk)
		if err == io.EOF && limited.N > 0 {
			break
		}
		if err != nil || limited.N <= 0 || finished || chunk.Code == nil {
			return nil, 0, errUnconfirmed
		}
		switch *chunk.Code {
		case 0:
			audio, err = base64.StdEncoding.AppendDecode(audio, []byte(chunk.Data))
			if err != nil || len(audio) > maxAudioBytes {
				return nil, 0, errUnconfirmed
			}
		case 20000000:
			if chunk.Data != "" {
				return nil, 0, errUnconfirmed
			}
			finished = true
		default:
			return nil, 0, errUnconfirmed
		}
		if chunk.Usage.Words > 0 && !math.IsInf(chunk.Usage.Words, 0) {
			words = chunk.Usage.Words
		}
	}
	if !finished || len(audio) == 0 {
		return nil, 0, errUnconfirmed
	}
	return audio, words, nil
}
