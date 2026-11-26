package tapnowweb

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/internal/canvas"
)

const seedance25WireModel = "agent-video-generation-02"

var (
	errDraftUnavailable = errors.New("原样片或原 TapNow 账号不可用")
	draftFilePattern    = regexp.MustCompile(`^([a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})(?:\.[A-Za-z0-9]+)?$`)
)

func draftFinalModel() Model {
	return Model{ID: canvas.TapNowFinalModel, Model: "seedance-2.5", Kind: "video", Provider: "tapnow", Name: "Seedance 2.5 正式视频",
		Hidden: true, FromDraft: true, Modes: []Mode{{
			ID: canvas.TapNowFinalMode, PromptOptional: true, Defaults: map[string]any{"resolution": "1080p"},
			Fields: map[string]Field{
				"sourceJob":  {Type: "string", Fixed: true},
				"resolution": {Type: "string", Enum: []any{"1080p", "4k"}, Default: "1080p", OptionLabels: map[string]string{"4k": "4K"}},
			},
		}}}
}

func (c Catalog) withDraftFinal() Catalog {
	c.Models = append(slices.Clone(c.Models), draftFinalModel())
	return c
}

func isDraftFinal(r Request) bool {
	return r.Model == canvas.TapNowFinalModel || r.Mode == canvas.TapNowFinalMode || r.Draft != nil
}

func prepareDraftFinal(r Request) (map[string]any, Mode, error) {
	mode := draftFinalModel().Modes[0]
	job := str(r.Parameters, "sourceJob")
	if r.Model != canvas.TapNowFinalModel || r.Mode != canvas.TapNowFinalMode || r.Kind != "video" || !r.VideoConsent ||
		r.Prompt != "" || len(r.References) != 0 || r.CameraControl != nil || !safeID.MatchString(job) ||
		r.MaxCost < 0 || math.IsNaN(r.MaxCost) || math.IsInf(r.MaxCost, 0) {
		return nil, mode, errors.New("正式视频只能沿用已确认的 Seedance 2.5 样片")
	}
	p, err := validateFields(mode, r.Parameters)
	if err != nil {
		return nil, mode, err
	}
	body := map[string]any{"provider": "tapnow", "model": seedance25WireModel, "resolution": p["resolution"], "scene": "generation", "times": 1}
	if r.Draft != nil {
		if r.Draft.jobID != job {
			return nil, mode, errDraftUnavailable
		}
		body["draft_video_id"] = r.Draft.fileID
	}
	return body, mode, nil
}

type DraftContinuation struct {
	jobID, fileID, sourceCanvas string
	revision                    uint64
	pricing                     draftPricing
}

type DraftSourceRecord struct {
	JobID, Owner, CanvasID, TaskID, FileID string
	TaskIndex, TaskOutputs                 int
	Ticket                                 Ticket
}

type draftPricing struct {
	Duration        float64  `json:"duration"`
	Images          []string `json:"images,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty"`
	Videos          []string `json:"videos,omitempty"`
	Audios          []string `json:"audios,omitempty"`
}

func draftPricingFrom(body map[string]any) *draftPricing {
	p := obj(EstimateBody("video", body)["videoParams"])
	d := number(p["duration"])
	if n, ok := p["duration"].(int); ok {
		d = float64(n)
	}
	if d != -1 && (d < 1 || d > 30 || math.IsNaN(d)) {
		return nil
	}
	return &draftPricing{Duration: d, Images: urlList(p["images"]), ReferenceImages: urlList(p["reference_images"]),
		Videos: urlList(p["videos"]), Audios: urlList(p["audios"])}
}

func (p draftPricing) estimateBody(body map[string]any) map[string]any {
	quote := merge(body, map[string]any{"duration": p.Duration})
	for key, refs := range map[string][]string{"images": p.Images, "reference_images": p.ReferenceImages, "videos": p.Videos, "audios": p.Audios} {
		if len(refs) > 0 {
			quote[key] = refs
		}
	}
	return quote
}

// DraftFileID mirrors the official generated-video file reference. The caller
// passes a validated provider result URL, never a URL from a canvas document.
func DraftFileID(source string) string {
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "https" || u.User != nil ||
		!slices.Contains([]string{"files.tapnow.media", "files.tapnow.ai", "files.tapnow.tv"}, u.Host) {
		return ""
	}
	i := strings.LastIndex(u.Path, "/")
	if i < 0 {
		return ""
	}
	prefix, file := u.Path[:i], u.Path[i+1:]
	if prefix != "/api/conversation/storage/uploads" && prefix != "/uploads" && prefix != "/files" {
		return ""
	}
	m := draftFilePattern.FindStringSubmatch(file)
	if len(m) != 2 || prefix != "/files" && m[1] != file {
		return ""
	}
	return m[1]
}

func (r Request) checkDraftAccount(revision uint64) error {
	if isDraftFinal(r) && (r.Draft == nil || r.Draft.revision != revision) {
		return errDraftUnavailable
	}
	return nil
}

func (r Request) remoteParameters(parameters map[string]any) map[string]any {
	if r.Draft != nil {
		resolution := str(r.Parameters, "resolution")
		if resolution == "" {
			resolution = "1080p"
		}
		return map[string]any{"model": "seedance-2.5", "resolution": resolution, "draftVideoId": r.Draft.fileID, "times": 1}
	}
	return merge(parameters, map[string]any{"model": r.Model, "modelType": r.Mode})
}

// ResolveDraft uses the private original generation receipt and the existing
// account-scoped canvas mapping. There is no new canvas or generation here.
func (c *Client) ResolveDraft(ctx context.Context, source DraftSourceRecord) (*DraftContinuation, error) {
	ticket := source.Ticket
	receipt, ok := c.Receipt(source.JobID)
	if !ok || receipt.Owner != source.Owner || receipt.Phase != "done" || !safeFileID(ticket.CanvasID) ||
		!safeFileID(ticket.NodeID) || !safeFileID(source.TaskID) || !slices.Contains(ticket.TaskIDs, source.TaskID) ||
		!slices.Equal(receipt.Ticket.TaskIDs, ticket.TaskIDs) || receipt.Ticket.CanvasID != ticket.CanvasID || receipt.Ticket.NodeID != ticket.NodeID {
		return nil, errDraftUnavailable
	}
	c.mu.Lock()
	revision := c.accountRevision
	c.mu.Unlock()
	identity, err := c.Identity(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(c.dir, c.canvasMappingName(identity, source.CanvasID)))
	var mapping canvasMapping
	if err != nil || json.Unmarshal(raw, &mapping) != nil || mapping.LocalID != source.CanvasID || mapping.RemoteID != ticket.CanvasID {
		return nil, errDraftUnavailable
	}
	fileID := source.FileID
	if fileID == "" {
		// Receipts written before this adapter stored the original task and
		// ordered result. Re-read that task instead of using an asset-level
		// preview, which may have been overwritten by deduplicated media.
		fileID, err = c.readDraftFile(ctx, revision, source)
		if err != nil {
			return nil, err
		}
	}
	if !draftFilePattern.MatchString(fileID) || len(fileID) != 36 {
		return nil, errDraftUnavailable
	}
	pricing := receipt.DraftPricing
	if pricing == nil {
		pricing, err = c.readDraftPricing(ctx, revision, ticket.CanvasID, fileID)
		if err != nil {
			return nil, err
		}
	}
	return &DraftContinuation{jobID: source.JobID, fileID: fileID, sourceCanvas: ticket.CanvasID, revision: revision, pricing: *pricing}, nil
}

func (c *Client) readDraftFile(ctx context.Context, revision uint64, source DraftSourceRecord) (string, error) {
	var tasks []map[string]any
	path := "/api/conversation/v1/generations/tasks?ids=" + url.QueryEscape(source.TaskID)
	if err := c.requestForAccount(ctx, revision, "GET", path, nil, &tasks); err != nil {
		return "", err
	}
	if len(tasks) != 1 || str(tasks[0], "id") != source.TaskID || str(tasks[0], "status") != "completed" {
		return "", errDraftUnavailable
	}
	urls := completedTaskURLs(tasks[0], "video")
	if len(urls) != source.TaskOutputs || source.TaskIndex < 0 || source.TaskIndex >= len(urls) {
		return "", errDraftUnavailable
	}
	return DraftFileID(urls[source.TaskIndex]), nil
}

func (c *Client) readDraftPricing(ctx context.Context, revision uint64, canvasID, fileID string) (*draftPricing, error) {
	const pageSize = 100
	for offset := 0; offset < 1000; offset += pageSize {
		query := url.Values{"canvas_id": {canvasID}, "generation_type": {"video"}, "offset": {strconv.Itoa(offset)}, "limit": {"100"}}
		var result struct {
			Items []struct {
				URL    string         `json:"file_url"`
				Params map[string]any `json:"additional_parameters"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := c.requestForAccount(ctx, revision, "GET", "/api/conversation/v1/me/generation-history?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			if !strings.EqualFold(DraftFileID(item.URL), fileID) {
				continue
			}
			if strings.EqualFold(str(item.Params, "resolution"), "480p") {
				if pricing := draftPricingFrom(item.Params); pricing != nil {
					return pricing, nil
				}
			}
			return nil, errDraftUnavailable
		}
		if len(result.Items) == 0 || offset+len(result.Items) >= result.Total {
			break
		}
	}
	return nil, errDraftUnavailable
}
