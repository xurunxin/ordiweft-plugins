package tapnowweb

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/canvas"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestSeedanceDraftFinalUsesOriginalFileAndConfirmedQuote(t *testing.T) {
	c, paid, canvases := fixture(t, "2.5", false)
	ctx := context.Background()
	const local = "draft-local"
	const draftJob = "original-seedance-draft"
	const sourceURL = "https://files.tapnow.media/api/conversation/storage/uploads/a9226ed1-d0cd-4317-b600-96328d73f582"
	sourceID := DraftFileID(sourceURL)
	if sourceID != "a9226ed1-d0cd-4317-b600-96328d73f582" || DraftFileID("https://elsewhere.example/?src="+sourceURL) != "" {
		t.Fatal("file identity extraction mismatch")
	}
	_, mode, err := c.catalog.Find(canvas.TapNowDraftModel, "video", "video_edit")
	if err != nil {
		t.Fatal(err)
	}
	draft := Request{Kind: "video", Model: canvas.TapNowDraftModel, Mode: mode.ID, Parameters: defaults(mode), Prompt: "approved draft",
		References:   []tapnow.Reference{{Kind: "video", Role: "video", URL: "/files/original-video"}},
		VideoConsent: true, UploadConsent: true, MaxCost: 3}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureVideo(5), Mime: "video/mp4"}, nil
	}
	_, ticket, err := c.Produce(ctx, draftJob, "alice", local, draft, resolve, nil)
	if err != nil || paid.Load() != 1 {
		t.Fatal(ticket, err)
	}
	old, _ := c.Receipt(draftJob)
	if old.DraftPricing == nil || old.DraftPricing.Duration != -1 ||
		!reflect.DeepEqual(old.DraftPricing.Videos, []string{"https://files.tapnow.media/api/conversation/storage/uploads/fixture-file-mp4"}) {
		t.Fatal("original pricing was not retained", old.DraftPricing)
	}
	// Reopen a legacy receipt and recover its pricing from the original file.
	old.DraftPricing = nil
	if err = c.receipt(draftJob, old); err != nil {
		t.Fatal(err)
	}
	const videoRef = "https://files.tapnow.ai/files/a19c554d-6b12-480d-a3f5-5c5f0f5010fd.mp4"
	transport := c.http.Transport
	var estimates, submissions []map[string]any
	c.http.Transport = observeTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/conversation/v1/generations/tasks" && paid.Load() == 1 {
			if req.Method != "GET" || req.URL.Query().Get("ids") != ticket.TaskIDs[0] {
				t.Error("read did not use the original task")
			}
			outputs := []map[string]string{{"video_url": videoRef}, {"video_url": sourceURL}}
			raw, _ := json.Marshal(map[string]any{"code": 0, "data": []map[string]any{{"id": ticket.TaskIDs[0], "status": "completed", "generated_videos": outputs}}})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), Request: req}, nil
		}
		if req.URL.Path == "/api/conversation/v1/me/generation-history" {
			if req.Method != "GET" || req.URL.Query().Get("canvas_id") != ticket.CanvasID {
				t.Error("history was not scoped to the original canvas")
			}
			record := map[string]any{"file_url": sourceURL, "additional_parameters": map[string]any{"resolution": "480p", "duration": 5, "videos": []string{videoRef}}}
			raw, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"items": []any{record}, "total": 1}})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw)), Request: req}, nil
		}
		if req.URL.Path == "/api/billing/v2/estimate" || req.URL.Path == "/api/conversation/v1/generations/video" {
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(raw))
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if req.URL.Path == "/api/billing/v2/estimate" {
				estimates = append(estimates, body)
			} else {
				submissions = append(submissions, body)
			}
		}
		return transport.RoundTrip(req)
	})
	source := DraftSourceRecord{JobID: draftJob, CanvasID: local, Owner: "alice", TaskID: ticket.TaskIDs[0], TaskIndex: 1, TaskOutputs: 2, Ticket: ticket}
	foreign := source
	foreign.Owner = "bob"
	if _, err = c.ResolveDraft(ctx, foreign); err == nil {
		t.Fatal("foreign draft accepted")
	}
	foreign = source
	foreign.CanvasID = "another-canvas"
	if _, err = c.ResolveDraft(ctx, foreign); err == nil {
		t.Fatal("foreign canvas accepted")
	}
	changed := source
	changed.TaskOutputs = 1
	if _, err = c.ResolveDraft(ctx, changed); err == nil {
		t.Fatal("changed output list accepted")
	}
	bound, err := c.ResolveDraft(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	final := Request{Kind: "video", Model: canvas.TapNowFinalModel, Mode: canvas.TapNowFinalMode, VideoConsent: true,
		Draft: bound, MaxCost: 2.49, Parameters: map[string]any{"sourceJob": draftJob, "resolution": "4k"}}
	quote, err := c.PreviewQuote(ctx, final, nil, false)
	if err != nil || quote.EstimatedCost != 2.5 || len(estimates) != 1 || paid.Load() != 1 {
		t.Fatal(quote, err, len(estimates), paid.Load())
	}
	estimate := obj(estimates[0]["videoParams"])
	if estimate["draft_video_id"] != sourceID || estimate["resolution"] != "4k" || estimate["duration"] != float64(5) || !reflect.DeepEqual(urlList(estimate["videos"]), []string{videoRef}) {
		t.Fatal("quote lost original duration, media or file", estimate)
	}
	if _, _, err = c.Produce(ctx, "draft-too-expensive", "alice", local, final, nil, nil); err == nil || paid.Load() != 1 {
		t.Fatal("confirmation amount was bypassed", err)
	}
	final.MaxCost = 2.5
	if _, _, err = c.Produce(ctx, "draft-final-confirmed", "alice", local, final, nil, nil); err != nil || paid.Load() != 2 || canvases.Load() != 1 || len(submissions) != 1 {
		t.Fatal("confirmed continuation should submit once in the original canvas", err, paid.Load(), canvases.Load())
	}
	body := submissions[0]
	delete(body, "context")
	delete(body, "metadata")
	want := map[string]any{"provider": "tapnow", "model": "agent-video-generation-02", "draft_video_id": sourceID, "resolution": "4k", "scene": "generation", "times": float64(1)}
	if !reflect.DeepEqual(body, want) {
		t.Fatal("unexpected paid body", body)
	}
	if _, _, err = c.Produce(ctx, "draft-final-confirmed", "alice", local, final, nil, nil); err == nil || paid.Load() != 2 {
		t.Fatal("repeat paid submission accepted")
	}
	final.Draft = nil
	final.Parameters["draft_video_id"] = sourceID
	if _, _, err = c.Prepare(final); err == nil {
		t.Fatal("client-controlled native draft ID accepted")
	}
}
