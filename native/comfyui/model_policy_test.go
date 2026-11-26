package comfyui

import (
	"context"
	"encoding/json"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"testing"
)

func TestH3PromptUsesBoundSlotsRatherThanConnectionOrder(t *testing.T) {
	d := Definition{Graph: Graph{}, Roles: map[string]Binding{}}
	for _, id := range []string{"a", "b", "c"} {
		d.Graph[id] = Node{Class: "LoadImage", Inputs: map[string]any{"image": id + ".png"}}
		d.Roles[id] = Binding{Node: id, Input: "image", Kind: "image"}
	}
	d.Graph["h3"] = Node{Class: "MiniMaxH3ReferenceToVideo", Inputs: map[string]any{"ref_images.ref_image_0": []any{"a", float64(0)}, "ref_images.ref_image_1": []any{"b", float64(0)}, "ref_images.ref_image_2": []any{"c", float64(0)}}}
	text, e := projectH3Prompt(d, "{{Image 1}} meets {{Image 2}}", []aigc.Input{{Kind: "image", Role: "c"}, {Kind: "image", Role: "a"}})
	if e != nil || text != "<Picture 3> meets <Picture 1>" {
		t.Fatal(text, e)
	}
	if _, e = projectH3Prompt(d, "{{Image 3}}", []aigc.Input{{Kind: "image", Role: "a"}}); e == nil {
		t.Fatal("dangling SDK reference accepted")
	}
	if _, e = projectH3Prompt(d, "<Picture 4>", nil); e == nil {
		t.Fatal("dangling native reference accepted")
	}
}

func TestEasyMixedMediaAndExportedPlaceholders(t *testing.T) {
	d := Definition{Graph: Graph{
		"i":  {Class: "LoadImage", Inputs: map[string]any{"image": "a.png"}},
		"v":  {Class: "LoadVideo", Inputs: map[string]any{"video": "b.mp4"}},
		"a":  {Class: "LoadAudio", Inputs: map[string]any{"audio": "c.wav"}},
		"h3": {Class: "MiniMaxH3Easy", Inputs: map[string]any{"media_1": []any{"v", float64(0)}, "media_type_1": "video", "media_2": []any{"i", float64(0)}, "media_type_2": "image", "media_3": []any{"a", float64(0)}, "media_type_3": "audio"}},
	}, Roles: map[string]Binding{"i": {Node: "i"}, "v": {Node: "v"}, "a": {Node: "a"}}}
	text, e := projectH3Prompt(d, "__MINIMAX_H3_REF_1__ {{Image 1}} {{Audio 1}}", []aigc.Input{{Kind: "audio", Role: "a"}, {Kind: "image", Role: "i"}})
	if e != nil || text != "__MINIMAX_H3_REF_1__ __MINIMAX_H3_REF_2__ __MINIMAX_H3_REF_3__" {
		t.Fatal(text, e)
	}
}

func TestNativeH3AudioOrdinalsIncludeVideoSoundtracks(t *testing.T) {
	d := Definition{Graph: Graph{
		"v": {Class: "LoadVideo", Inputs: map[string]any{}}, "sound": {Class: "LoadAudio", Inputs: map[string]any{}}, "voice": {Class: "LoadAudio", Inputs: map[string]any{}},
		"h3": {Class: "MiniMaxH3ReferenceToVideo", Inputs: map[string]any{"ref_videos.ref_video_2": []any{"v", float64(0)}, "ref_video_audios.ref_video_audio_2": []any{"sound", float64(0)}, "ref_audios.ref_audio_5": []any{"voice", float64(0)}}},
	}, Roles: map[string]Binding{"v": {Node: "v", Kind: "video"}, "sound": {Node: "sound", Kind: "audio"}, "voice": {Node: "voice", Kind: "audio"}}}
	text, e := projectH3Prompt(d, "{{Video 1}} {{Audio 1}} {{Audio 2}}", []aigc.Input{{Kind: "video", Role: "v"}, {Kind: "audio", Role: "voice"}, {Kind: "audio", Role: "sound"}})
	if e != nil || text != "<Video 1> <Audio 2> <Audio 1>" {
		t.Fatal(text, e)
	}
}

func TestKnownModelCapsRejectBeforeProviderCalls(t *testing.T) {
	for _, tc := range []struct {
		class, input string
		value, max   float64
	}{{"MiniMaxH3Easy", "seconds", 16, 15}, {"MiniMaxMusic3TextEncode", "max_duration", 301, 300}} {
		d := Definition{Graph: Graph{"m": {Class: tc.class, Inputs: map[string]any{tc.input: 5.0}}}, Fields: map[string]Binding{"p": {Node: "m", Input: tc.input}}, Manifest: aigc.Manifest{ID: "comfy-policy", Models: []aigc.Model{{ID: "workflow", Capability: "video.generate", Fields: map[string]aigc.Field{"p": {Type: "number", Default: 5.0}}}}}}
		p := New(d)
		f := p.Manifest().Models[0].Fields["p"]
		if f.Maximum == nil || *f.Maximum != tc.max {
			t.Fatal(f)
		}
		raw, _ := json.Marshal(aigc.Generation{Provider: "comfy-policy", Model: "workflow", Capability: "video.generate", Parameters: map[string]any{"p": tc.value}})
		if e := p.Validate(context.Background(), aigc.Request{Payload: raw}); e == nil {
			t.Fatal("model cap bypassed")
		}
	}
}

func TestSeedControlKeepsPersistedManifestValid(t *testing.T) {
	info := fixtureInfo()
	info["LoadImage"].Input["required"]["seed"] = []any{"INT", map[string]any{"default": 1.0, "min": 0.0, "max": 1000.0}}
	var graph Graph
	_ = json.Unmarshal([]byte(fixtureGraph), &graph)
	graph["load"].Inputs["seed"] = 1.0
	raw, _ := json.Marshal(graph)
	d, err := Compile("seed", "http://localhost:8000", raw, info)
	if err != nil {
		t.Fatal(err)
	}
	manifest := New(d).Manifest()
	if err = manifest.Validate(); err != nil {
		t.Fatal("persisted workflow will prevent startup", err)
	}
	found := false
	for _, f := range manifest.Models[0].Fields {
		found = found || f.Control == "seed"
		if f.Control == "seed" && f.Section != "advanced" {
			t.Fatal("seed should remain grouped in advanced settings", f)
		}
	}
	if !found {
		t.Fatal("seed metadata missing")
	}
}
