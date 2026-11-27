// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const (
	provider = "apimart"
	version  = "1.2.0"
	secret   = "API_KEY"
	api      = "https://api.apimart.ai"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

//go:embed catalog.json
var catalogJSON []byte

type reference struct {
	Field    string   `json:"field"`
	Kind     string   `json:"kind"`
	Role     string   `json:"role,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max"`
	MaxBytes int      `json:"maxBytes,omitempty"`
	Array    bool     `json:"array,omitempty"`
	Inline   bool     `json:"inline,omitempty"`
	ItemRole string   `json:"itemRole,omitempty"`
}

type modelSpec struct {
	Model      aigc.Model     `json:"model"`
	Upstream   string         `json:"upstream,omitempty"`
	Endpoint   string         `json:"endpoint"`
	PromptKey  string         `json:"promptKey,omitempty"`
	Fixed      map[string]any `json:"fixed,omitempty"`
	References []reference    `json:"references,omitempty"`
	MusicTask  bool           `json:"musicTask,omitempty"`
}

type plugin struct {
	manifest aigc.Manifest
	models   map[string]modelSpec
	base     string
	client   *http.Client
	stateDir string
}

func loadPlugin() (*plugin, error) {
	var catalog []modelSpec
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		return nil, err
	}
	p := &plugin{
		models: make(map[string]modelSpec, len(catalog)), base: api,
		stateDir: os.Getenv("ORDIWEFT_PLUGIN_STATE_DIR"),
		client: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		manifest: aigc.Manifest{
			APIVersion: aigc.APIVersion, ID: provider, Name: "APIMart", Version: version, Runtime: "process",
			Auth: "global", Secrets: []string{secret},
			Scheduling: aigc.Scheduling{Independent: true, Concurrency: aigc.RemoteHostConcurrency, MaxQueued: 200},
			Nodes: []aigc.Node{
				{ID: "image", Label: "APIMart 图片", Capability: "image.generate", Model: "gemini-nano-banana-2.1"},
				{ID: "video", Label: "APIMart 视频", Capability: "video.generate", Model: "magi-2"},
				{ID: "music", Label: "APIMart 音乐", Capability: "music.generate", Model: "suno"},
				{ID: "speech", Label: "APIMart 配音", Capability: "speech.generate", Model: "gpt-4o-mini-tts"},
			},
		},
	}
	for _, spec := range catalog {
		if spec.Upstream == "" {
			spec.Upstream = spec.Model.ID
		}
		p.manifest.Models = append(p.manifest.Models, spec.Model)
		p.models[spec.Model.ID] = spec
	}
	if err := p.manifest.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *plugin) Manifest() aigc.Manifest { return p.manifest }

type attachment struct {
	ref   reference
	input aigc.Input
}

func (p *plugin) prepare(request aigc.Request) (modelSpec, map[string]any, []attachment, error) {
	var spec modelSpec
	if err := aigc.ValidateGeneration(p.manifest, request); err != nil {
		return spec, nil, nil, err
	}
	var g aigc.Generation
	if err := json.Unmarshal(request.Payload, &g); err != nil {
		return spec, nil, nil, err
	}
	spec = p.models[g.Model]
	body, err := aigc.NormalizeParameters(spec.Model, g.Parameters)
	if err != nil {
		return spec, nil, nil, err
	}
	for key, value := range spec.Fixed {
		body[key] = value
	}
	body["model"] = spec.Upstream
	if spec.Model.TaskContinuation != nil && strings.TrimSpace(g.Prompt) != "" {
		return spec, nil, nil, errors.New("APIMart continuation uses the original prompt")
	}
	if request.SourceTask != nil {
		if spec.Model.TaskContinuation == nil {
			return spec, nil, nil, errors.New("APIMart model does not accept a source task")
		}
		if !request.SourceTask.Valid() || !taskIDPattern.MatchString(request.SourceTask.Ticket) {
			return spec, nil, nil, errors.New("invalid APIMart source task")
		}
		switch spec.Model.ID {
		case "MiniMax-H3-Regeneration":
			body["source_task_id"] = request.SourceTask.Ticket
		case "seedance-2.5-final-1080p":
			body["draft_task_id"] = request.SourceTask.Ticket
		default:
			return spec, nil, nil, errors.New("unknown APIMart task continuation")
		}
	}
	prompt := strings.TrimSpace(g.Prompt)
	if spec.Upstream == "flowmusic" {
		if lyrics, _ := body["lyrics"].(string); prompt == "" && strings.TrimSpace(lyrics) == "" {
			return spec, nil, nil, errors.New("Flow Music requires a description or lyrics")
		}
		if bpm, ok := body["bpm"].(float64); ok {
			body["bpm"] = fmt.Sprintf("%.0f", bpm)
		}
	}
	if body["model"] == "suno" && spec.Endpoint == "/v1/music/generations" {
		if err := prepareSuno(body, prompt); err != nil {
			return spec, nil, nil, err
		}
	}
	if key := spec.PromptKey; key != "" && prompt != "" {
		body[key] = prompt
	}
	if spec.Endpoint == "/v1/chat/completions" {
		body = map[string]any{
			"model": spec.Upstream, "messages": []map[string]string{{"role": "user", "content": prompt}},
			"modalities": []string{"text", "audio"}, "audio": map[string]any{"voice": body["voice"], "format": body["response_format"]},
		}
	}

	inputs, err := aigc.ResolveInputRoles(spec.Model, g.Parameters, request.Inputs)
	if err != nil {
		return spec, nil, nil, err
	}
	var files []attachment
	counts := make([]int, len(spec.References))
	for _, input := range inputs {
		found := false
		for i, ref := range spec.References {
			if input.Kind != ref.Kind || !matchesRole(ref, input.Role) {
				continue
			}
			counts[i]++
			if counts[i] > ref.Max {
				return spec, nil, nil, fmt.Errorf("too many references for %s", ref.Field)
			}
			files = append(files, attachment{ref: ref, input: input})
			found = true
			break
		}
		if !found {
			return spec, nil, nil, errors.New("unsupported reference role")
		}
	}
	for i, ref := range spec.References {
		if counts[i] < ref.Min {
			return spec, nil, nil, fmt.Errorf("reference required for %s", ref.Field)
		}
	}
	if spec.Model.ID == "magi-2-reference" && len(files) == 0 {
		return spec, nil, nil, errors.New("MAGI-2 reference mode needs at least one image, video or audio clip")
	}
	if spec.Model.ID == "kling-v4-reference" && len(files) > 15 {
		return spec, nil, nil, errors.New("Kling V4 accepts at most 15 combined reference files")
	}
	if spec.Upstream == "wan2.7-r2v" && (len(files) == 0 || len(files) > 5) {
		return spec, nil, nil, errors.New("Wan 2.7 reference mode needs one to five references in total")
	}
	if spec.Upstream == "Omni-Flash-Ext" && counts[0] == 2 {
		return spec, nil, nil, errors.New("Omni Flash Ext accepts one or three image references")
	}
	if spec.Upstream == "grok-imagine-2.0-ext" && len(files) > 0 {
		if count, ok := body["n"].(float64); ok && count != 1 {
			return spec, nil, nil, errors.New("Grok Imagine 2.0 Ext accepts one output when using references")
		}
	}
	if body["output_format"] == "gif" {
		for _, file := range files {
			if file.ref.Role == "mask" {
				return spec, nil, nil, errors.New("GIF output does not support a mask")
			}
		}
	}
	return spec, body, files, nil
}

func prepareSuno(body map[string]any, prompt string) error {
	custom := body["custom"] == "custom"
	body["custom"] = custom
	instrumental, _ := body["instrumental"].(bool)
	if !custom && (prompt == "" || len([]rune(prompt)) > 3000) || custom && !instrumental && prompt == "" {
		return errors.New("Suno requires an inspiration prompt up to 3000 characters or lyrics in custom vocal mode")
	}
	if body["version"] == "custom" {
		id, _ := body["custom_model_id"].(string)
		if !uuidPattern.MatchString(id) {
			return errors.New("Suno custom model requires its complete UUID")
		}
		if persona, _ := body["persona_id"].(string); persona != "" {
			return errors.New("Suno custom model and persona are mutually exclusive")
		}
		delete(body, "version")
	} else {
		delete(body, "custom_model_id")
	}
	return nil
}

func matchesRole(ref reference, role string) bool {
	if len(ref.Roles) == 0 {
		return ref.Role == "" || ref.Role == role
	}
	for _, candidate := range ref.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func (p *plugin) Validate(_ context.Context, request aigc.Request) error {
	_, _, _, err := p.prepare(request)
	return err
}

func run(args []string) error {
	p, err := loadPlugin()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return aigc.Serve(context.Background(), os.Stdin, os.Stdout, p)
	}
	switch args[0] {
	case "package":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"manifest": p.manifest, "command": []string{filepath.Base(exe)},
			"sha256": fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	case "manifest":
		return json.NewEncoder(os.Stdout).Encode(p.manifest)
	case "prices":
		return p.printPrices(context.Background(), args[1:])
	case "status":
		return p.printStatus(context.Background(), args[1:])
	default:
		return errors.New("expected package, manifest, prices or status")
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
