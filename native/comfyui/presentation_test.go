package comfyui

import (
	"encoding/json"
	"reflect"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func TestPresentationUpgradesSavedWorkflowWithoutChangingExecution(t *testing.T) {
	graph, err := Parse(json.RawMessage(`{
 "1":{"class_type":"ModelLoader","inputs":{"width":768}},
 "2":{"class_type":"Generate","_meta":{"title":"生成视频"},"inputs":{"prompt":"hello","width":512,"seconds":["3",0],"noise_seed":42,"steps":8}},
 "3":{"class_type":"PrimitiveFloat","_meta":{"title":"Float (Duration)"},"inputs":{"value":3}},
 "4":{"class_type":"TextToLowercase","_meta":{"title":"原始提示词备份"},"inputs":{"text":"backup"}}
 }`))
	if err != nil {
		t.Fatal(err)
	}
	d := Definition{Graph: graph, Prompt: &Binding{Node: "2", Input: "prompt"}, Fields: map[string]Binding{}, Manifest: aigc.Manifest{Models: []aigc.Model{{Fields: map[string]aigc.Field{}}}}}
	for _, id := range keys(graph) {
		for _, input := range keys(graph[id].Inputs) {
			value := graph[id].Inputs[input]
			if _, _, ok := link(value, graph); ok {
				continue
			}
			key := id + "-" + input
			f, ok := field(value, nil)
			if !ok {
				t.Fatal(key)
			}
			d.Fields[key] = Binding{Node: id, Input: input}
			d.Manifest.Models[0].Fields[key] = f
		}
	}
	before, _ := json.Marshal(d.Graph)
	oldParameters := defaults(d.Manifest.Models[0])
	p := New(d)
	fields := p.Manifest().Models[0].Fields
	for key, section := range map[string]string{"1-width": "advanced", "2-prompt": "common", "2-width": "common", "2-noise_seed": "advanced", "2-steps": "advanced", "3-value": "common", "4-text": "advanced"} {
		if fields[key].Section != section || fields[key].Group == nil || fields[key].Group.ID != d.Fields[key].Node {
			t.Fatalf("%s: %#v", key, fields[key])
		}
	}
	if fields["3-value"].Label != "时长（秒） · value" || fields["2-prompt"].Control != "textarea" {
		t.Fatal("primitive or prompt presentation lost")
	}
	after, _ := json.Marshal(p.Definition.Graph)
	if string(before) != string(after) || !reflect.DeepEqual(oldParameters, defaults(p.Manifest().Models[0])) || !reflect.DeepEqual(d.Fields, p.Definition.Fields) {
		t.Fatal("presentation changed execution")
	}
	if d.Manifest.Models[0].Fields["2-width"].Group != nil {
		t.Fatal("mutated stored definition")
	}
	if !reflect.DeepEqual(p.Definition, present(p.Definition)) {
		t.Fatal("presentation is not idempotent")
	}
	normalized, e := aigc.NormalizeParameters(p.Manifest().Models[0], oldParameters)
	if e != nil || !reflect.DeepEqual(normalized, oldParameters) {
		t.Fatal("advanced fields dropped from payload", normalized, e)
	}
}
