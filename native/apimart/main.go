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
	version  = "1.3.10"
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
	BindTo   string   `json:"bindTo,omitempty"`
}

type modelSpec struct {
	Presentation *modePresentation `json:"presentation,omitempty"`
	Model        aigc.Model        `json:"model"`
	Upstream     string            `json:"upstream,omitempty"`
	Endpoint     string            `json:"endpoint"`
	PromptKey    string            `json:"promptKey,omitempty"`
	Fixed        map[string]any    `json:"fixed,omitempty"`
	References   []reference       `json:"references,omitempty"`
	MusicTask    bool              `json:"musicTask,omitempty"`
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
			},
		},
	}
	for i := range catalog {
		if catalog[i].Upstream == "" {
			catalog[i].Upstream = catalog[i].Model.ID
		}
	}
	applyGenerationModes(catalog)
	for _, spec := range catalog {
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
	if g.Model == "grok-imagine-video-1.5-edit" {
		return spec, nil, nil, errors.New("Grok Imagine Video 1.5 官方版不支持视频编辑，请选择 Grok Imagine Video 官方版")
	}
	body, err := aigc.NormalizeParameters(spec.Model, g.Parameters)
	if err != nil {
		return spec, nil, nil, err
	}
	if spec.Upstream == "seedream-5-5" && body["layer_decomposition"] == true {
		// Translate the old draft value when the new nested field is unset.
		legacy, _ := g.Parameters["text_recognition"].(bool)
		if options, ok := body["layer_decomposition_options"].(map[string]any); ok && legacy {
			original, _ := g.Parameters["layer_decomposition_options"].(map[string]any)
			if _, explicit := original["text_recognition"]; !explicit {
				options["text_recognition"] = true
			}
		}
	}
	for key, value := range spec.Fixed {
		body[key] = value
	}
	if spec.Model.ID != "midjourney-follow-up" && spec.Model.ID != "midjourney-video" && spec.Model.ID != "midjourney-inpaint" {
		body["model"] = spec.Upstream
	}
	if spec.Upstream == "veo3.1-fast" || spec.Upstream == "veo3.1-quality" {
		switch body["generation_type"] {
		case "text-to-video":
			delete(body, "generation_type")
		case "image-to-video":
			body["generation_type"] = "frame"
		case "reference-to-video":
			body["generation_type"] = "reference"
		}
	}
	if spec.Model.TaskContinuation != nil && !spec.Model.Prompt.Required && !spec.Model.TaskContinuation.AllowOptionalPrompt && strings.TrimSpace(g.Prompt) != "" {
		return spec, nil, nil, errors.New("APIMart continuation uses the original prompt")
	}
	if request.SourceTask != nil {
		if spec.Model.TaskContinuation == nil {
			return spec, nil, nil, errors.New("APIMart model does not accept a source task")
		}
		if !request.SourceTask.Valid() {
			return spec, nil, nil, errors.New("invalid APIMart source task")
		}
		pattern := taskIDPattern
		if spec.Model.ID == "flowmusic-follow-up" {
			pattern = uuidPattern
		}
		if !pattern.MatchString(request.SourceTask.Ticket) {
			return spec, nil, nil, errors.New("invalid APIMart source task")
		}
		switch spec.Model.ID {
		case "grok-imagine-2.0-ext-region-edit", "midjourney-inpaint":
			// The source is resolved through the free segmentation stage at execution.
		case "midjourney-follow-up":
			if err := prepareMidjourneyFollowUp(&spec, body, *request.SourceTask); err != nil {
				return spec, nil, nil, err
			}
		case "suno-follow-up", "flowmusic-follow-up":
			// Music operation fields are normalized during both preflight and execution.
		case "MiniMax-H3-Regeneration":
			body["source_task_id"] = request.SourceTask.Ticket
		case "seedance-2.5-final-1080p", "apimart-zoe-final":
			body["draft_task_id"] = request.SourceTask.Ticket
		case "veo3.1-fast-remix", "veo3.1-quality-remix":
			spec.Endpoint = strings.Replace(spec.Endpoint, "{task_id}", request.SourceTask.Ticket, 1)
		case "flux-3-video-final":
			body["draft_from_task_id"] = request.SourceTask.Ticket
		case "pixverse-v6-extend", "gemini-omni-1.1-flash-task-extend", "gemini-omni-1.2-flash-task-extend", "gemini-omni-1.2-flash-ext-task-extend", "gemini-omni-flash-preview-task-extend":
			body["extend_from_task_id"] = request.SourceTask.Ticket
		default:
			return spec, nil, nil, errors.New("unknown APIMart task continuation")
		}
	}
	if spec.Model.ID == "suno-follow-up" || spec.Model.ID == "flowmusic-follow-up" {
		if err := prepareMusicFollowUp(&spec, body, request.SourceTask); err != nil {
			return spec, nil, nil, err
		}
	}
	prompt := spec.Model.Prompt.ProjectReferences(strings.TrimSpace(g.Prompt))
	if spec.Upstream == "flowmusic" && spec.Model.TaskContinuation == nil {
		if lyrics, _ := body["lyrics"].(string); prompt == "" && strings.TrimSpace(lyrics) == "" {
			return spec, nil, nil, errors.New("Flow Music requires a description or lyrics")
		}
		if bpm, ok := body["bpm"].(float64); ok {
			body["bpm"] = fmt.Sprintf("%.0f", bpm)
		}
	}
	if spec.Upstream == "suno" {
		prompt, err = prepareSuno(spec.Model.ID, body, prompt)
		if err != nil {
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
	if len(files) > 0 {
		switch spec.Model.ID {
		case "seedance-2.5", "seedance-2-5-flash", "seedance-2.5-draft", "apimart-zoe", "apimart-zoe-draft":
			body["omni_reference_task_type"] = "reference"
		case "wan3.0-video", "wan3.0-video-prime", "Omni-Flash-Ext":
			body["generation_type"] = "reference"
		}
	}
	if spec.Model.ID == "kling-v4-reference" && len(files) > 15 {
		return spec, nil, nil, errors.New("Kling V4 accepts at most 15 combined reference files")
	}
	if spec.Upstream == "gemini-omni-1.1-flash-ext" && counts[0] == 2 {
		return spec, nil, nil, errors.New("Omni Flash Ext accepts one or three image references")
	}
	if spec.Model.ID == "grok-imagine-2.0-ext" {
		if count, ok := body["n"].(float64); len(files) > 0 && ok && count != 1 {
			return spec, nil, nil, errors.New("Grok Imagine 2.0 Ext accepts one output when using references")
		}
		if body["size"] == "auto" {
			delete(body, "size")
		}
	}
	if spec.Model.ID == "grok-imagine-1.5-video-apimart-frames" && len(files) > 1 && body["resolution"] == "1080p" {
		return spec, nil, nil, errors.New("Grok Imagine 1.5 APIMart first and last frames support 480p or 720p")
	}
	if spec.Model.ID == "pixverse-v6" && len(files) == 1 {
		if files[0].ref.Role != "first_frame" {
			return spec, nil, nil, errors.New("PixVerse V6 transition requires both first and last frames")
		}
		files[0].ref.Field, files[0].ref.Array = "image_urls", true
	}
	if spec.Model.ID == "grok-imagine-2.0-ext-region-edit" {
		if err := normalizeGrokRegions(body); err != nil {
			return spec, nil, nil, err
		}
	}
	if err := normalizeModelOptions(spec, body, files); err != nil {
		return spec, nil, nil, err
	}
	return spec, body, files, nil
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
