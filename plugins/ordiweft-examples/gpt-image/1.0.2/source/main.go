// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

//go:embed release.template.json
var release []byte

type imageResponse struct {
	Data []struct {
		Base64 string `json:"b64_json"`
	} `json:"data"`
	Usage json.RawMessage `json:"usage"`
}

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("gpt_image_original_result_unconfirmed\n")
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
	if json.Unmarshal(release, &descriptor) != nil || descriptor.Generation == nil {
		return errors.New("invalid_candidate_catalog")
	}
	g := task.Start.Generation
	var model aigc.Model
	for _, candidate := range descriptor.Generation.Models {
		if candidate.ID == g.Model {
			model = candidate
			break
		}
	}
	if g.Provider != "gpt-image" || g.Capability != "image.generate" || model.ID == "" {
		return errors.New("unsupported_original_model")
	}
	g.Parameters, err = aigc.NormalizeParameters(model, g.Parameters)
	if err != nil {
		return err
	}
	request := aigc.Request{}
	request.Payload, _ = json.Marshal(g)
	for _, input := range task.Start.Inputs {
		request.Inputs = append(request.Inputs, aigc.Input{Kind: input.Kind, Role: input.Role})
	}
	if aigc.ValidateGeneration(*descriptor.Generation, request) != nil {
		return errors.New("invalid_original_contract")
	}
	inputs := append([]pluginhost.TaskInput(nil), task.Start.Inputs...)
	sort.SliceStable(inputs, func(i, j int) bool { return inputs[i].Role < inputs[j].Role })
	if err = validateInputs(ctx, task.Host, inputs, g.Parameters); err != nil {
		return err
	}
	streaming, _ := g.Parameters["stream"].(bool)
	expected := 1
	if !streaming {
		n, ok := g.Parameters["n"].(float64)
		if !ok || math.Trunc(n) != n || n < 1 || n > 10 {
			return errors.New("invalid_original_count")
		}
		expected = int(n)
	}
	operation, endpoint := "generate-json", "/v1/images/generations"
	if len(inputs) > 0 {
		operation, endpoint = "edit-multipart", "/v1/images/edits"
	}
	if streaming {
		operation += "-sse"
	}
	if g.Parameters["output_format"] == "jpeg" {
		operation += "-jpeg"
	}
	metadata := map[string]any{"model": g.Model, "endpoint": endpoint, "state": "submitting", "outputFormat": g.Parameters["output_format"], "transport": "host-services"}
	if err = task.Host.Call(ctx, "original-phase", "jobs", "checkpoint", map[string]any{"metadata": metadata}, nil); err != nil {
		return err
	}
	var out imageResponse
	if streaming {
		reader, err := task.Host.OpenStream(ctx, "original-submit", operation)
		if err != nil {
			return err
		}
		defer reader.Close()
		out, err = readSSE(reader, func(count int) error {
			metadata["state"], metadata["partialImagesReceived"] = "receiving", count
			return task.Host.Call(ctx, fmt.Sprintf("original-partial-%d", count), "jobs", "progress", map[string]any{"metadata": metadata}, nil)
		})
		if err != nil || reader.Status() < 200 || reader.Status() >= 300 {
			return errors.New("original_stream_unconfirmed")
		}
	} else {
		var response struct {
			Status int                    `json:"status"`
			Body   pluginhost.AssetHandle `json:"body"`
		}
		if err = task.Host.Call(ctx, "original-submit", "connections", "request", map[string]string{"operation": operation}, &response); err != nil || response.Status < 200 || response.Status >= 300 {
			return errors.New("original_response_unconfirmed")
		}
		body, err := pluginhost.ReadAsset(ctx, task.Host, "original-response", response.Body, 64<<20)
		if err != nil || json.Unmarshal(body, &out) != nil {
			return errors.New("original_response_invalid")
		}
		if err = task.Host.Call(ctx, "release-original-response", "assets", "release", map[string]string{"id": response.Body.ID}, nil); err != nil {
			return err
		}
	}
	if len(out.Data) != expected {
		return errors.New("original_output_count_mismatch")
	}
	outputs := make([]string, 0, expected)
	for index, item := range out.Data {
		if len(item.Base64) > 70<<20 {
			return errors.New("original_output_too_large")
		}
		data, err := base64.StdEncoding.DecodeString(item.Base64)
		if err != nil || len(data) == 0 || len(data) > 50<<20 {
			return errors.New("original_output_invalid")
		}
		candidate, err := pluginhost.StageOutput(ctx, task.Host, fmt.Sprintf("original-output-%d", index), data, 50<<20)
		if err != nil {
			return err
		}
		outputs = append(outputs, candidate.ID)
	}
	metadata["state"] = "received"
	if len(out.Usage) > 0 && string(out.Usage) != "null" {
		var usage any
		if json.Unmarshal(out.Usage, &usage) != nil {
			return errors.New("original_usage_invalid")
		}
		metadata["usage"] = usage
	}
	return task.Complete(outputs, metadata)
}

func validateInputs(ctx context.Context, host *pluginhost.Client, inputs []pluginhost.TaskInput, parameters map[string]any) error {
	if len(inputs) > 17 {
		return errors.New("too_many_original_inputs")
	}
	var first pluginhost.MediaInfo
	var mask *pluginhost.TaskInput
	references := 0
	for index, input := range inputs {
		if input.Kind != "image" || input.Asset.Bytes < 1 || input.Asset.Bytes >= 50<<20 {
			return errors.New("invalid_original_image")
		}
		var info pluginhost.MediaInfo
		if err := host.Call(ctx, fmt.Sprintf("inspect-original-%d", index), "media", "inspect", map[string]string{"id": input.Asset.ID}, &info); err != nil {
			return err
		}
		if info.SHA256 != input.Asset.SHA256 || info.Bytes != input.Asset.Bytes || info.Mime != input.Asset.Mime || info.Width <= 0 || info.Height <= 0 || int64(info.Width)*int64(info.Height) > 100000000 || info.Mime != "image/png" && info.Mime != "image/jpeg" && info.Mime != "image/webp" {
			return errors.New("invalid_original_image_facts")
		}
		if input.Role == "mask" {
			if mask != nil {
				return errors.New("ambiguous_original_mask")
			}
			copy := input
			mask = &copy
			continue
		}
		if !strings.HasPrefix(input.Role, "image") {
			return errors.New("invalid_original_role")
		}
		references++
		if references == 1 {
			first = info
			if info.Orientation >= 5 {
				first.Width, first.Height = first.Height, first.Width
			}
		}
	}
	if references > 16 || len(inputs) > 0 && references == 0 || (parameters["edit_overlay"] == "mask") != (mask != nil) {
		return errors.New("invalid_original_edit_inputs")
	}
	if mask == nil {
		return nil
	}
	if mask.Asset.Mime != "image/png" || mask.Asset.Bytes >= 4<<20 {
		return errors.New("invalid_original_mask")
	}
	data, err := pluginhost.ReadAsset(ctx, host, "original-mask", mask.Asset, 4<<20)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != first.Width || img.Bounds().Dy() != first.Height {
		return errors.New("original_mask_dimensions_mismatch")
	}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha < 65535 {
				return nil
			}
		}
	}
	return errors.New("original_mask_has_no_transparency")
}

func readSSE(reader io.Reader, partial func(int) error) (imageResponse, error) {
	var out imageResponse
	scanner := bufio.NewScanner(io.LimitReader(reader, 256<<20))
	scanner.Buffer(make([]byte, 64<<10), 72<<20)
	var data strings.Builder
	completed, count := false, 0
	consume := func() error {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		if raw == "" || raw == "[DONE]" {
			return nil
		}
		var event struct {
			Type   string          `json:"type"`
			Base64 string          `json:"b64_json"`
			Usage  json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(raw), &event) != nil {
			return errors.New("invalid_original_event")
		}
		switch event.Type {
		case "image_generation.partial_image", "image_edit.partial_image":
			count++
			return partial(count)
		case "image_generation.completed", "image_edit.completed":
			if completed {
				return errors.New("duplicate_original_completion")
			}
			completed = true
			out.Data = append(out.Data, struct {
				Base64 string `json:"b64_json"`
			}{event.Base64})
			out.Usage = event.Usage
		case "error":
			return errors.New("original_stream_error")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consume(); err != nil {
				return out, err
			}
		} else if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(line, "data:"))
			data.WriteByte('\n')
		}
	}
	if err := consume(); err != nil {
		return out, err
	}
	if scanner.Err() != nil || !completed {
		return out, errors.New("original_stream_incomplete")
	}
	return out, nil
}
