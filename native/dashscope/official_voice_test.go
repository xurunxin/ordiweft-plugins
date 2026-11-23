package dashscope

import (
	"gitea.nkit.top/xurx/Ordiweft/internal/voices"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"testing"
)

func TestEveryOfficialVoiceCanBindOnlyItsModel(t *testing.T) {
	for _, v := range voices.List("dashscope", "") {
		model := v.CompatibleModels[0]
		r := aigc.Request{Voice: &aigc.Voice{Provider: "dashscope", ID: v.ID, Kind: "official", CompatibleModels: v.CompatibleModels}}
		g := aigc.Generation{Capability: "speech.generate", Model: model}
		if e := validateVoice(g, nil, r); e != nil {
			t.Fatalf("%s: %v", v.ID, e)
		}
		g.Model = "qwen3-tts-vd-2026-01-26"
		if validateVoice(g, nil, r) == nil {
			t.Fatal("official voice crossed models")
		}
	}
}
