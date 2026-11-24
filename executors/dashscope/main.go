package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/internal/dashscope"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

var adapterVersion = "dev"

type broker struct {
	host     *pluginhost.Client
	sequence int
}

func (b *broker) call(ctx context.Context, operation string, input, output any) error {
	b.sequence++
	return b.host.Call(ctx, fmt.Sprintf("bailian-%d", b.sequence), "bailian", operation, input, output)
}

func (b *broker) write(ctx context.Context, source io.Reader, limit int) error {
	block := make([]byte, pluginhost.BailianBlockBytes)
	offset := 0
	for {
		n, err := io.ReadFull(source, block)
		if n > 0 {
			if offset+n > limit {
				return pluginhost.ErrContract
			}
			if e := b.call(ctx, "write", pluginhost.BailianBlock{Offset: offset, Data: block[:n]}, nil); e != nil {
				return e
			}
			offset += n
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (b *broker) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx := request.Context()
	if request.Body != nil {
		defer request.Body.Close()
		if err := b.write(ctx, request.Body, pluginhost.BailianMaxRequestBytes); err != nil {
			return nil, err
		}
	}
	var response pluginhost.BailianResponse
	err := b.call(ctx, "request", pluginhost.BailianRequest{
		Host: request.URL.Hostname(), Path: request.URL.RequestURI(), Method: request.Method,
		Async:   request.Header.Get("X-DashScope-Async") == "enable",
		Resolve: request.Header.Get("X-DashScope-OssResourceResolve") == "enable",
	}, &response)
	if err != nil || response.Status < 100 || response.Status > 599 || response.Bytes < 0 || response.Bytes > pluginhost.BailianMaxResponseBytes {
		return nil, pluginhost.ErrContract
	}
	body := make([]byte, 0, response.Bytes)
	for len(body) < response.Bytes {
		var block pluginhost.BailianBlock
		if err := b.call(ctx, "read", map[string]int{"offset": len(body)}, &block); err != nil {
			return nil, err
		}
		if block.Offset != len(body) || len(block.Data) == 0 || len(block.Data) > pluginhost.BailianBlockBytes || len(body)+len(block.Data) > response.Bytes {
			return nil, pluginhost.ErrContract
		}
		body = append(body, block.Data...)
	}
	return &http.Response{StatusCode: response.Status, Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
}

func (b *broker) upload(ctx context.Context, model string, _ []byte) (string, error) {
	var result struct{ Object string }
	err := b.call(ctx, "upload-audio", map[string]string{"model": model}, &result)
	return result.Object, err
}

func (b *broker) lookup(ctx context.Context, model string) (*aigc.Voice, error) {
	var result struct{ Voice *aigc.Voice }
	err := b.call(ctx, "clone-get", map[string]string{"model": model}, &result)
	return result.Voice, err
}

func (b *broker) remember(ctx context.Context, model string, voice aigc.Voice) error {
	return b.call(ctx, "clone-put", map[string]any{"model": model, "voice": voice}, nil)
}

func run(ctx context.Context) error {
	task, err := pluginhost.OpenGeneration(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	b := &broker{host: task.Host}
	var env pluginhost.BailianExecutionContext
	if err := b.call(ctx, "context", struct{}{}, &env); err != nil {
		return err
	}
	p, err := dashscope.NewManagedRuntime(env.Region, dashscope.ManagedRuntime{
		Transport: b, UploadAudio: b.upload, LookupClone: b.lookup, RememberClone: b.remember,
	})
	if err != nil {
		return err
	}
	payload, err := json.Marshal(task.Start.Generation)
	if err != nil {
		return err
	}
	request := aigc.Request{Payload: payload, Voice: env.Voice,
		Secrets: map[string]string{"API_KEY": "provider-bridge", "WORKSPACE_ID": pluginhost.BailianBridgeWorkspace}}
	for i, input := range task.Start.Inputs {
		data, err := pluginhost.ReadAsset(ctx, task.Host, fmt.Sprintf("reference-%d", i), input.Asset, 20<<20)
		if err != nil {
			return err
		}
		request.Inputs = append(request.Inputs, aigc.Input{Kind: input.Kind, Role: input.Role, Data: data})
	}
	var checkpointError error
	result, err := p.Execute(ctx, request, func(part aigc.Result) {
		if checkpointError == nil {
			checkpointError = b.call(ctx, "checkpoint", map[string]json.RawMessage{"metadata": part.Metadata}, nil)
		}
	})
	if err != nil {
		return err
	}
	if checkpointError != nil {
		return checkpointError
	}
	outputs := make([]string, 0, len(result.Media))
	for _, media := range result.Media {
		if len(media.Data) > 0 {
			if err := b.write(ctx, bytes.NewReader(media.Data), pluginhost.BailianMaxResponseBytes); err != nil {
				return err
			}
		}
		var staged struct{ ID string }
		if err := b.call(ctx, "stage", map[string]any{"kind": media.Kind, "voice": media.Voice, "url": media.URL}, &staged); err != nil {
			return err
		}
		outputs = append(outputs, staged.ID)
	}
	var metadata map[string]any
	if err := json.Unmarshal(result.Metadata, &metadata); err != nil || metadata == nil {
		return pluginhost.ErrContract
	}
	metadata["adapterVersion"] = adapterVersion
	return task.Complete(outputs, metadata)
}

func emitManifest(version string) error {
	if !pluginhost.ValidVersion(version) {
		return pluginhost.ErrContract
	}
	m := dashscope.New().Manifest()
	m.Version = version
	for i := range m.Models {
		m.Models[i].Pricing = nil
		if q := m.Models[i].QueueLimit; q != nil {
			_, q.Bucket, _ = strings.Cut(q.Bucket, "/")
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m)
}

func main() {
	var err error
	switch {
	case len(os.Args) == 3 && os.Args[1] == "manifest":
		err = emitManifest(os.Args[2])
	case len(os.Args) == 1:
		err = run(context.Background())
	default:
		err = pluginhost.ErrContract
	}
	if err != nil {
		_, _ = os.Stderr.WriteString("dashscope-execution-unconfirmed\n")
		os.Exit(1)
	}
}
