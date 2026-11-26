// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type plugin struct {
	manifest aigc.Manifest
	models   map[string]modelSpec
}

func (p *plugin) Manifest() aigc.Manifest { return p.manifest }

func (p *plugin) Validate(_ context.Context, request aigc.Request) error {
	_, _, _, err := p.prepare(request)
	return err
}

type attachment struct {
	field string
	mime  string
	data  []byte
}

func (p *plugin) prepare(request aigc.Request) (modelSpec, map[string]any, []attachment, error) {
	var empty modelSpec
	parts := strings.Split(request.Secrets[credentialName], ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(request.Secrets[credentialName], " \t\r\n") {
		return empty, nil, nil, errors.New("invalid Higgsfield credentials")
	}
	if err := aigc.ValidateGeneration(p.manifest, request); err != nil {
		return empty, nil, nil, err
	}
	var generation aigc.Generation
	if err := json.Unmarshal(request.Payload, &generation); err != nil {
		return empty, nil, nil, err
	}
	spec := p.models[generation.Model]
	params, err := aigc.NormalizeParameters(spec.model, generation.Parameters)
	if err != nil {
		return empty, nil, nil, err
	}
	for _, key := range spec.encodedFields {
		value, exists := params[key]
		if !exists {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(value.(string)), &decoded); err != nil {
			return empty, nil, nil, fmt.Errorf("invalid JSON parameter %s", key)
		}
		params[key] = decoded
	}
	if spec.promptKey != "" {
		prompt := generation.Prompt
		if strings.TrimSpace(prompt) == "" {
			prompt = spec.promptDefault
		}
		if prompt != "" {
			params[spec.promptKey] = prompt
		}
	}
	for _, key := range []string{"link_url", "file_url"} {
		if raw, ok := params[key].(string); ok {
			if _, valid := httpsURL(raw); !valid {
				return empty, nil, nil, fmt.Errorf("invalid HTTPS parameter %s", key)
			}
		}
	}
	inputs, err := aigc.ResolveInputRoles(spec.model, generation.Parameters, request.Inputs)
	if err != nil {
		return empty, nil, nil, err
	}
	grouped := map[string][]string{}
	attachments := make([]attachment, 0, len(inputs))
	for _, input := range inputs {
		name := spec.roleFields[input.Role]
		if len(spec.roleFields) == 0 {
			for _, field := range spec.media {
				if field.kind == input.Kind {
					name = field.name
					break
				}
			}
		}
		if name == "" {
			return empty, nil, nil, errors.New("invalid Higgsfield reference role")
		}
		item := attachment{field: name, data: input.Data}
		if len(input.Data) != 0 {
			item.mime, err = uploadType(input, strings.HasPrefix(spec.model.ID, magiPrefix))
			if err != nil {
				return empty, nil, nil, err
			}
		}
		attachments = append(attachments, item)
		grouped[name] = append(grouped[name], "https://input.example.invalid/reference")
	}
	if spec.model.ID == magiPrefix+"reference-to-video" && len(inputs) == 0 {
		return empty, nil, nil, errors.New("MAGI-2 reference-to-video requires a media reference")
	}
	for _, field := range spec.media {
		if len(grouped[field.name]) > field.max {
			return empty, nil, nil, fmt.Errorf("too many %s references", field.name)
		}
	}
	spec.setMedia(params, grouped)
	if err := spec.schema.Validate(params); err != nil {
		return empty, nil, nil, errors.New("Higgsfield parameters or reference roles do not match the model schema")
	}
	return spec, params, attachments, nil
}

func (spec modelSpec) setMedia(params map[string]any, grouped map[string][]string) {
	for _, field := range spec.media {
		delete(params, field.name)
		urls := grouped[field.name]
		if len(urls) == 0 {
			continue
		}
		if field.array {
			params[field.name] = urls
		} else {
			params[field.name] = urls[0]
		}
	}
}

func uploadType(input aigc.Input, magi bool) (string, error) {
	mime := http.DetectContentType(input.Data)
	var kind string
	var limit int
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		kind, limit = "image", 29_999_999
	case "video/mp4":
		kind, limit = "video", 200_000_000
	case "audio/wave":
		kind, mime, limit = "audio", "audio/wav", 15_000_000
	}
	if kind == "" || kind != input.Kind || magi && len(input.Data) > limit {
		return "", errors.New("unsupported Higgsfield reference format or size")
	}
	return mime, nil
}

func (p *plugin) Execute(ctx context.Context, request aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	spec, body, attachments, err := p.prepare(request)
	if err != nil {
		return aigc.Result{}, err
	}
	if request.JobID == "" {
		return aigc.Result{}, errors.New("Higgsfield requires an original job ID")
	}
	for _, item := range attachments {
		if len(item.data) == 0 {
			return aigc.Result{}, errors.New("Higgsfield reference bytes are missing")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 14*time.Minute)
	defer cancel()
	client := newClient(request.Secrets[credentialName])
	grouped := map[string][]string{}
	for _, item := range attachments {
		inputURL, err := client.upload(ctx, item.data, item.mime)
		if err != nil {
			return aigc.Result{}, err
		}
		grouped[item.field] = append(grouped[item.field], inputURL)
	}
	spec.setMedia(body, grouped)
	if err := spec.schema.Validate(body); err != nil {
		return aigc.Result{}, errors.New("uploaded Higgsfield inputs do not match the model schema")
	}
	receipt, err := client.submit(ctx, spec.endpoint, body, request.JobID)
	if err != nil {
		return aigc.Result{}, err
	}
	metadata := resultMetadata(spec, receipt, body)
	checkpoint(aigc.Result{Metadata: metadata})
	observed, err := client.wait(ctx, receipt)
	if err != nil {
		return aigc.Result{}, err
	}
	outputs := observed.Images
	if spec.output == "video" {
		if observed.Video == nil {
			return aigc.Result{}, errors.New("Higgsfield returned no video")
		}
		outputs = []remoteMedia{*observed.Video}
	}
	if len(outputs) == 0 {
		return aigc.Result{}, errors.New("Higgsfield returned no images")
	}
	result := aigc.Result{Metadata: resultMetadata(spec, observed, body)}
	for _, output := range outputs {
		if _, ok := httpsURL(output.URL); !ok {
			return aigc.Result{}, errors.New("invalid Higgsfield output URL")
		}
		result.Media = append(result.Media, aigc.Media{Kind: spec.output, URL: output.URL})
	}
	return result, nil
}

func resultMetadata(spec modelSpec, remote remoteRequest, body map[string]any) json.RawMessage {
	metadata := map[string]any{
		"taskId": remote.ID, "requestId": remote.ID, "status": remote.Status,
		"model": spec.model.ID, "endpoint": "/" + spec.endpoint,
	}
	if body["draft"] == true {
		metadata["draftId"] = remote.ID
	}
	result, _ := json.Marshal(metadata)
	return result
}

type localProcess struct{ *plugin }

func (p localProcess) Execute(ctx context.Context, request aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	result, err := p.plugin.Execute(ctx, request, checkpoint)
	if err != nil {
		var upstream *apiError
		if errors.As(err, &upstream) {
			fmt.Fprintln(os.Stderr, upstream.Error())
		}
	}
	return result, err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	p, err := loadPlugin()
	if err != nil {
		return err
	}
	if len(os.Args) == 2 && os.Args[1] == "package" {
		binary, err := os.Executable()
		if err != nil {
			return err
		}
		f, err := os.Open(binary)
		if err != nil {
			return err
		}
		defer f.Close()
		digest := sha256.New()
		if _, err := io.Copy(digest, f); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Manifest aigc.Manifest `json:"manifest"`
			Command  []string      `json:"command"`
			SHA256   string        `json:"sha256"`
		}{p.manifest, []string{filepath.Base(binary)}, hex.EncodeToString(digest.Sum(nil))})
	}
	if len(os.Args) != 1 {
		return errors.New("expected a standard Ordiweft process request or the package command")
	}
	return aigc.Serve(context.Background(), os.Stdin, os.Stdout, localProcess{p})
}
