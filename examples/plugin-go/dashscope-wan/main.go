// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

type response struct {
	Status int                    `json:"status"`
	Body   pluginhost.AssetHandle `json:"body"`
}

type taskResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Choices    []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
					Type  string `json:"type"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
}

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString("dashscope_wan_result_unconfirmed\n")
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	g, inputs := task.Start.Generation, task.Start.Inputs
	if g.Provider != "dashscope" || g.Capability != "image.generate" || g.Model != "wan2.7-image" && g.Model != "wan2.7-image-pro" || len(inputs) > 9 {
		return errors.New("unsupported_original_request")
	}
	n, ok := g.Parameters["n"].(float64)
	sequential, sequentialOK := g.Parameters["enable_sequential"].(bool)
	size, sizeOK := g.Parameters["size"].(string)
	if !ok || n < 1 || n > 12 || float64(int(n)) != n || !sequentialOK || !sizeOK || !sequential && n > 4 || size == "4K" && (g.Model != "wan2.7-image-pro" || sequential || len(inputs) > 0) {
		return errors.New("invalid_original_parameters")
	}
	extensions := make([]string, len(inputs))
	for i, input := range inputs {
		if input.Kind != "image" || input.Asset.Bytes < 1 || input.Asset.Bytes > 20<<20 {
			return errors.New("invalid_original_input")
		}
		switch input.Asset.Mime {
		case "image/png":
			extensions[i] = "png"
		case "image/jpeg":
			extensions[i] = "jpg"
		case "image/webp":
			extensions[i] = "webp"
		default:
			return errors.New("unsupported_original_image")
		}
	}
	if len(inputs) > 0 {
		var policy struct {
			Status int `json:"status"`
		}
		if err = task.Host.Call(ctx, "original-policy", "connections", "request", map[string]string{"operation": "upload-policy"}, &policy); err != nil || policy.Status != 200 {
			return errors.New("upload_policy_unconfirmed")
		}
		for i := range inputs {
			var uploaded response
			if err = task.Host.Call(ctx, fmt.Sprintf("original-upload-%d", i), "transfers", "upload", map[string]string{"operation": fmt.Sprintf("upload-%d-%s", i, extensions[i])}, &uploaded); err != nil || uploaded.Status < 200 || uploaded.Status >= 300 {
				return errors.New("original_upload_unconfirmed")
			}
		}
	}
	operation := fmt.Sprintf("generate-%d", len(inputs))
	if len(inputs) == 0 && !sequential {
		operation = "generate-0-thinking"
	}
	var submitted response
	if err = task.Host.Call(ctx, "original-submit", "connections", "request", map[string]string{"operation": operation}, &submitted); err != nil {
		return err
	}
	var original taskResponse
	if err = readJSON(ctx, task, "submitted", submitted, &original); err != nil || !validTicket(original.Output.TaskID) {
		return errors.New("original_task_unconfirmed")
	}
	if err = task.Host.Call(ctx, "original-ticket", "jobs", "checkpoint", map[string]any{"metadata": map[string]any{"remoteTaskId": original.Output.TaskID}}, nil); err != nil {
		return err
	}
	var observed struct {
		Response response `json:"response"`
		Receipt  struct {
			CallID string `json:"callId"`
		} `json:"receipt"`
	}
	if err = task.Host.Call(ctx, "original-observation", "jobs", "wait", map[string]string{"name": "result"}, &observed); err != nil {
		return err
	}
	var result taskResponse
	if err = readJSON(ctx, task, "observed", observed.Response, &result); err != nil || result.Output.TaskStatus != "SUCCEEDED" || result.Output.TaskID != original.Output.TaskID || len(result.Output.Choices) != 1 {
		return errors.New("original_result_unconfirmed")
	}
	content := result.Output.Choices[0].Message.Content
	if len(content) < 1 || len(content) > int(n) || !sequential && len(content) != int(n) {
		return errors.New("original_output_count_mismatch")
	}
	outputs := make([]string, 0, len(content))
	for i, image := range content {
		if image.Image == "" || image.Type != "" && image.Type != "image" {
			return errors.New("unsupported_result_content")
		}
		var downloaded struct {
			Asset pluginhost.AssetHandle `json:"asset"`
		}
		if err = task.Host.Call(ctx, fmt.Sprintf("original-download-%d", i), "transfers", "download", map[string]string{"name": fmt.Sprintf("image-%d", i), "callId": observed.Receipt.CallID}, &downloaded); err != nil {
			return err
		}
		var staged struct {
			Output pluginhost.AssetHandle `json:"output"`
		}
		if err = task.Host.Call(ctx, fmt.Sprintf("original-stage-%d", i), "outputs", "stage", map[string]string{"id": downloaded.Asset.ID}, &staged); err != nil {
			return err
		}
		outputs = append(outputs, staged.Output.ID)
	}
	return task.Complete(outputs, map[string]any{"remoteTaskId": original.Output.TaskID, "model": g.Model, "endpoint": "/services/aigc/image-generation/generation", "transport": "host-services"})
}

func validTicket(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func readJSON(ctx context.Context, task *pluginhost.GenerationSession, prefix string, result response, value any) error {
	if result.Status < 200 || result.Status >= 300 {
		return errors.New("original_response_rejected")
	}
	var decoded struct {
		Asset pluginhost.AssetHandle `json:"asset"`
	}
	if err := task.Host.Call(ctx, prefix+"-decode", "assets", "decode", map[string]any{"id": result.Body.ID, "encoding": "json", "pointer": "", "maxBytes": 65536}, &decoded); err != nil {
		return err
	}
	body, err := pluginhost.ReadAsset(ctx, task.Host, prefix+"-read", decoded.Asset, 65536)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}
