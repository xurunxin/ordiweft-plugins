// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

//go:embed release.template.json
var release []byte

//go:embed policies/generate-json.json
var reviewedSizing []byte

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("gpt_image_original_result_unconfirmed\n")
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
	defer func() {
		if returned != nil {
			_ = task.Host.ReportDiagnostic(ctx, "original-failure", pluginhost.Failure{Code: resultFailureCode(returned), Stage: "output", Submission: "unknown"})
		}
	}()
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
	if err = validateParameters(g); err != nil {
		return err
	}
	inputs := append([]pluginhost.TaskInput(nil), task.Start.Inputs...)
	sort.SliceStable(inputs, func(i, j int) bool { return inputs[i].Role < inputs[j].Role })
	if err = validateInputs(ctx, task.Host, inputs, g.Parameters); err != nil {
		return err
	}
	streaming, _ := g.Parameters["stream"].(bool)
	expected := 1
	if !streaming && g.Parameters["output_format"] != "svg" {
		n, ok := g.Parameters["n"].(float64)
		if !ok || math.Trunc(n) != n || n < 1 || n > 10 {
			return errors.New("invalid_original_count")
		}
		expected = int(n)
	}
	operation, endpoint := "generate-json", "/v1/images/generations"
	if g.Parameters["output_format"] == "gif" || g.Parameters["output_format"] == "svg" {
		operation = "generate-formats"
	}
	if len(inputs) > 0 {
		operation, endpoint = "edit-multipart", "/v1/images/edits"
		if g.Parameters["edit_overlay"] == "markup" && len(inputs) == 2 {
			operation = "edit-markup"
		}
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
	outputs := make([]string, 0, expected)
	stage := func(index int, image *decodedImage) error {
		metadata["state"], metadata["outputIndex"] = "staging", index
		if err := task.Host.Call(ctx, fmt.Sprintf("original-stage-progress-%d", index), "jobs", "progress", map[string]any{"metadata": metadata}, nil); err != nil {
			return err
		}
		candidate, err := pluginhost.StageOutputReaderAndReleaseBuffer(ctx, task.Host, fmt.Sprintf("original-output-%d", index), image.file, image.size, image.digest, maxImageBytes)
		if err != nil {
			return err
		}
		format, _ := g.Parameters["output_format"].(string)
		if format == "svg" && candidate.Mime != "image/svg+xml" || format == "gif" && candidate.Mime != "image/gif" || format != "svg" && candidate.Mime == "image/svg+xml" {
			return errors.New("original_output_format_mismatch")
		}
		if format == "gif" {
			var info pluginhost.MediaInfo
			if err = task.Host.Call(ctx, fmt.Sprintf("inspect-output-%d", index), "media", "inspect", map[string]string{"id": candidate.ID}, &info); err != nil || info.Frames < 2 {
				return errors.New("original_animation_incomplete")
			}
		}
		outputs = append(outputs, candidate.ID)
		return nil
	}
	releaseInputs := func() error {
		for index, input := range inputs {
			if err := task.Host.Call(ctx, fmt.Sprintf("release-original-input-%d", index), "assets", "release", map[string]string{"id": input.Asset.ID}, nil); err != nil {
				return err
			}
		}
		return nil
	}
	var usage json.RawMessage
	if streaming {
		reader, err := task.Host.OpenStream(ctx, "original-submit", operation)
		if err != nil {
			return err
		}
		defer reader.Close()
		if err = releaseInputs(); err != nil {
			return err
		}
		usage, err = readImageSSE(reader, func(count int) error {
			metadata["state"], metadata["partialImagesReceived"] = "receiving", count
			return task.Host.Call(ctx, fmt.Sprintf("original-partial-%d", count), "jobs", "progress", map[string]any{"metadata": metadata}, nil)
		}, stage)
		if err != nil || reader.Status() < 200 || reader.Status() >= 300 {
			if err != nil {
				return fmt.Errorf("original_stream_unconfirmed: %w", err)
			}
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
		if err = releaseInputs(); err != nil {
			return err
		}
		body, err := pluginhost.OpenAsset(ctx, task.Host, "original-response", response.Body, 256<<20)
		if err != nil {
			return err
		}
		defer body.Close()
		usage, err = readImages(body, expected, stage)
		if err != nil || !body.Verified() {
			if err != nil {
				return fmt.Errorf("original_response_invalid: %w", err)
			}
			return errors.New("original_response_invalid")
		}
		if err = task.Host.Call(ctx, "release-original-response", "assets", "release", map[string]string{"id": response.Body.ID}, nil); err != nil {
			return err
		}
	}
	metadata["state"] = "received"
	if len(usage) > 0 && string(usage) != "null" {
		var value any
		if json.Unmarshal(usage, &value) != nil {
			return errors.New("original_usage_invalid")
		}
		metadata["usage"] = value
	}
	return task.Complete(outputs, metadata)
}

func validateParameters(g aigc.Generation) error {
	if !strings.HasPrefix(g.Model, "gpt-image-2") {
		return nil
	}
	size, _ := g.Parameters["size"].(string)
	if g.Parameters["custom_resolution"] == false {
		// Use the candidate's reviewed data table, also enforced independently by
		// the host's original-request encoder and final outbound validation.
		var policy struct {
			Body struct {
				Size struct {
					Cases []struct {
						From []string
						To   string
					}
				}
				Prompt struct{ Suffixes []struct{ Text string } }
			}
		}
		if json.Unmarshal(reviewedSizing, &policy) != nil {
			return errors.New("invalid_reviewed_sizing")
		}
		for _, entry := range policy.Body.Size.Cases {
			if len(entry.From) == 2 && entry.From[0] == g.Parameters["resolution_tier"] && entry.From[1] == g.Parameters["aspect_ratio"] {
				size = entry.To
				break
			}
		}
		if g.Parameters["aspect_ratio"] == "360" {
			if len(policy.Body.Prompt.Suffixes) != 1 || utf8.RuneCountInString(g.Prompt+policy.Body.Prompt.Suffixes[0].Text) > 32000 {
				return errors.New("original_panorama_prompt_too_long")
			}
		}
	}
	if size == "auto" {
		return nil
	}
	var width, height int
	if n, _ := fmt.Sscanf(size, "%dx%d", &width, &height); n != 2 || fmt.Sprintf("%dx%d", width, height) != size || width < 1 || height < 1 || width > 3840 || height > 3840 || width%16 != 0 || height%16 != 0 || width > height*3 || height > width*3 || width*height > 8294400 || g.Parameters["output_format"] != "gif" && width*height < 655360 {
		return errors.New("invalid_original_dimensions")
	}
	if g.Parameters["output_format"] == "gif" {
		frames, framesOK := g.Parameters["frame_count"].(float64)
		count, countOK := g.Parameters["n"].(float64)
		if !framesOK || !countOK || float64(width*height)*frames*count > 64*1024*1024 {
			return errors.New("original_animation_pixel_limit")
		}
	}
	return nil
}

func validateInputs(ctx context.Context, host *pluginhost.Client, inputs []pluginhost.TaskInput, parameters map[string]any) error {
	if len(inputs) > 18 || len(inputs) > 0 && (parameters["output_format"] == "svg" || parameters["output_format"] == "gif") {
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
		if input.Role == "mask" || input.Role == "markup" {
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
	overlay := parameters["edit_overlay"] == "mask" || parameters["edit_overlay"] == "markup"
	if references > 16 || len(inputs) > 0 && references == 0 || overlay != (mask != nil) || mask != nil && mask.Role != parameters["edit_overlay"] {
		return errors.New("invalid_original_edit_inputs")
	}
	if mask == nil {
		return nil
	}
	if mask.Asset.Mime != "image/png" || mask.Role == "mask" && mask.Asset.Bytes >= 4<<20 || mask.Role == "markup" && int64(first.Width)*int64(first.Height) > 16777216 {
		return errors.New("invalid_original_mask")
	}
	limit := int64(50 << 20)
	if mask.Role == "mask" {
		limit = 4 << 20
	}
	data, err := pluginhost.ReadAsset(ctx, host, "original-overlay", mask.Asset, limit)
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
