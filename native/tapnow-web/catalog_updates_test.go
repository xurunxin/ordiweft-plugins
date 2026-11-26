package tapnowweb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSavedCatalogKeepsReviewedLegacyTransforms(t *testing.T) {
	b, _ := snapshots.ReadFile("catalog.json")
	var raw rawCatalog
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, m := range raw.Models {
		if hashes := legacyTransforms[str(m, "id")]; len(hashes) > 0 {
			m["transformHash"] = hashes[0]
		}
	}
	b, _ = json.Marshal(raw)
	c, err := ParseCatalog(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst", "doubao-seedream-5.0-pro"} {
		if _, _, err := c.Find(id, "image", "text_to_image"); err != nil {
			t.Fatalf("saved reviewed model %s became unavailable: %v", id, err)
		}
	}
	if reviewedTransform("gpt-image-2.6-flare", "unreviewed") || reviewedTransform("unknown", "") {
		t.Fatal("unreviewed transform accepted")
	}
}

func TestDiscoveredImageAliasesRequireMatchingPublishedContract(t *testing.T) {
	b, _ := snapshots.ReadFile("catalog.json")
	var raw rawCatalog
	_ = json.Unmarshal(b, &raw)
	for _, tc := range []struct {
		name, provider, hash string
		allowed              bool
	}{{"gpt-image-2.6-new", "openai", compiledTransforms["gpt-image-2.6-flare"], true}, {"gpt-image-2.6-changed", "openai", "changed", false}, {"gpt-image-2.6-wrong-channel", "other", compiledTransforms["gpt-image-2.6-flare"], false}, {"gpt-image-9", "openai", compiledTransforms["gpt-image-2.6-flare"], false}} {
		r := map[string]any{"id": tc.name, "model": tc.name, "type": "image", "provider": tc.provider, "transformHash": tc.hash, "supportedModes": []string{"text_to_image"}, "options": map[string]any{}, "defaults": map[string]any{}}
		copy := raw
		copy.Models = []map[string]any{r}
		data, _ := json.Marshal(copy)
		c, e := ParseCatalog(data)
		if e != nil {
			t.Fatal(e)
		}
		m, v, e := c.Find(tc.name, "image", "text_to_image")
		if !tc.allowed {
			if e == nil {
				t.Fatal("unreviewed discovery enabled", tc.name)
			}
			continue
		}
		if e != nil {
			t.Fatal(e)
		}
		body, _, e := c.Prepare(Request{Model: m.ID, Kind: "image", Mode: v.ID, Prompt: "fixture", Parameters: v.Defaults})
		if e != nil || body["model"] != tc.name || body["size"] != "auto" {
			t.Fatal("alias did not reuse verified parameters", body, e)
		}
	}
}

func TestGPT26AdaptiveAnimation(t *testing.T) {
	c := Baseline()
	for _, id := range []string{"gpt-image-2.6-flare", "gpt-image-2.6-sunburst"} {
		_, edit, e := c.Find(id, "image", "image_to_image")
		if e != nil {
			t.Fatal(e)
		}
		for _, format := range edit.Fields["output_format"].Enum {
			if format == "gif" || format == "svg" {
				t.Fatal("edit mode advertised text-only output", format)
			}
		}
		_, v, e := c.Find(id, "image", "text_to_image")
		if e != nil {
			t.Fatal(e)
		}
		choices, ok := evalRule(v.Fields["aspectRatio"].Rule, map[string]any{"output_format": "gif"}).([]any)
		if !ok || len(choices) != 1 || choices[0] != "auto" {
			t.Fatal("GIF catalog must constrain the shared ratio control to adaptive dimensions")
		}
		p := merge(v.Defaults, map[string]any{"output_format": "gif", "frame_count": float64(12), "frame_duration_ms": float64(150)})
		b, _, e := c.Prepare(Request{Model: id, Kind: "image", Mode: v.ID, Prompt: "animate a calm sea", Parameters: p})
		if e != nil {
			t.Fatal(e)
		}
		if b["size"] != "auto" || b["output_format"] != "gif" || b["frame_count"] != float64(12) || b["frame_duration_ms"] != float64(150) || b["complexity"] != nil {
			t.Fatal("GPT 2.6 animation mapping", b)
		}
		p["background"] = "transparent"
		if _, _, e = c.Prepare(Request{Model: id, Kind: "image", Mode: v.ID, Prompt: "fixture", Parameters: p}); e == nil {
			t.Fatal("unsupported Web transparency accepted")
		}
	}
}

func TestPublishedCatalogVisibility(t *testing.T) {
	c := Baseline()
	if _, _, err := c.Find("doubao-seedream-5.5", "image", "text_to_image"); err != nil {
		t.Fatal("newly visible Seedream unavailable", err)
	}
	if _, _, err := c.Find("seedance-2.5-flash", "video", "text_to_video"); err == nil {
		t.Fatal("Flash requires a site gate unavailable on the integrated origins")
	}
}

func TestStartupUsesNewestCatalog(t *testing.T) {
	b, _ := snapshots.ReadFile("catalog.json")
	var saved rawCatalog
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	observed, err := time.Parse(time.RFC3339Nano, saved.ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	saved.Models = saved.Models[:1]
	dir := t.TempDir()
	for _, offset := range []time.Duration{-time.Hour, time.Hour} {
		saved.ObservedAt = observed.Add(offset).Format(time.RFC3339Nano)
		b, err = json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "catalog.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		c := New("", "", "", dir, "")
		_, _, err := c.catalog.Find("kling-4.0-preview", "video", "text_to_video")
		if (err == nil) != (offset < 0) {
			t.Fatalf("unexpected catalog selection for stored offset %v: %v", offset, err)
		}
	}
}
