package tapnowweb

import (
	"context"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentDuplicateProducesOnlyOneTask(t *testing.T) {
	c, count, _ := fixture(t, "2", false)
	_, v, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	r := Request{Model: "nano-banana-flash", Mode: v.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(v), MaxCost: 3}
	var wg sync.WaitGroup
	var succeeded atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, e := c.Produce(context.Background(), "concurrent-fixture", "alice", "fixture-local", r, nil, nil); e == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 || succeeded.Load() != 1 {
		t.Fatal("multiple charges", count.Load(), succeeded.Load())
	}
}

func TestManualEstimateAndLegacyCostLimit(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	r := Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 1}
	if _, _, err := c.Produce(context.Background(), "legacy-budget-fixture", "alice", "fixture-local", r, nil, nil); err == nil || paid.Load() != 0 {
		t.Fatal("legacy budget must block the request", err)
	}
	r.MaxCost, r.UseEstimatedCost = 0, true
	if _, _, err := c.Produce(context.Background(), "manual-estimate-fixture", "alice", "fixture-local", r, nil, nil); err != nil || paid.Load() != 1 {
		t.Fatal("manual request should use the verified upstream estimate", err, paid.Load())
	}
}

func TestProducesShareCanvasButKeepIndividualTickets(t *testing.T) {
	c, paid, creates := fixture(t, "2", false)
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	r := Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 3}
	_, first, err := c.Produce(context.Background(), "fixture-job-first", "alice", "local-canvas", r, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := c.Produce(context.Background(), "fixture-job-second", "bob", "local-canvas", r, nil, nil)
	if err != nil || first.CanvasID != second.CanvasID || first.NodeID == second.NodeID || len(first.TaskIDs) != 1 || len(second.TaskIDs) != 1 || paid.Load() != 2 || creates.Load() != 1 {
		t.Fatalf("first=%+v second=%+v error=%v paid=%d creates=%d", first, second, err, paid.Load(), creates.Load())
	}
}
func TestVideoReferenceDurationAndProduction(t *testing.T) {
	c, count, _ := fixture(t, "2", false)
	_, v, _ := c.catalog.Find("seedance-2.5", "video", "reference_to_video")
	r := Request{Model: "seedance-2.5", Mode: v.ID, Kind: "video", Prompt: "Use {{Video 1}}", Parameters: defaults(v), References: []tapnow.Reference{{Kind: "video", Role: "video", URL: "/files/source-video", Order: 0}}, VideoConsent: true, UploadConsent: true, MaxCost: 3}
	resolve := func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureVideo(35), Mime: "video/mp4", Duration: 1}, nil
	}
	if _, _, e := c.Produce(context.Background(), "invalid-video-fixture", "alice", "fixture-local", r, resolve, nil); e == nil || count.Load() != 0 {
		t.Fatal("duration exceeded but charged")
	}
	resolve = func(context.Context, string) (Material, error) {
		return Material{Bytes: fixtureVideo(5), Mime: "video/mp4", Duration: 35}, nil
	}
	media, ticket, e := c.Produce(context.Background(), "valid-video-fixture", "alice", "fixture-local", r, resolve, nil)
	if e != nil || count.Load() != 1 || len(media) != 1 || media[0].Kind != "video" || len(ticket.TaskIDs) != 1 {
		t.Fatal(media, ticket, e)
	}
}
func TestResolutionDependentOptionsRejectInvalidPairs(t *testing.T) {
	c := Baseline()
	_, v, _ := c.Find("veo3.1-fast", "video", "text_to_video")
	p := defaults(v)
	p["duration"] = float64(4)
	p["resolution"] = "720p"
	r := Request{Model: "veo3.1-fast", Mode: v.ID, Kind: "video", Prompt: "fixture", Parameters: p, VideoConsent: true, MaxCost: 3}
	if _, _, e := c.Prepare(r); e != nil {
		t.Fatal(e)
	}
	p["resolution"] = "4k"
	if _, _, e := c.Prepare(r); e == nil {
		t.Fatal("invalid resolution-duration pair")
	}
}

func TestOmniReferenceHonorsPublishedZeroMinimum(t *testing.T) {
	c := Baseline()
	_, v, _ := c.Find("seedance-2.5", "video", "reference_to_video")
	if v.Images.Min != 0 || v.Videos.Min != 0 {
		t.Fatal("fixture range changed")
	}
	r := Request{Model: "seedance-2.5", Mode: v.ID, Kind: "video", Prompt: "Text-only in omni mode", Parameters: defaults(v), VideoConsent: true, MaxCost: 3}
	r.Parameters["times"] = float64(2)
	b, _, e := c.Prepare(r)
	if e != nil || b["images"] != nil || b["videos"] != nil || b["times"] != float64(2) {
		t.Fatal(b, e)
	}
}
