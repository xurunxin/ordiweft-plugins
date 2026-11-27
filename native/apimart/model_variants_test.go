// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestVisibleExtRoutesKeepLabelsAndNotes(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range p.models {
		variant, annotated := modelVariants[spec.Upstream]
		if spec.Model.Hidden || !annotated {
			continue
		}
		label, note := spec.Model.Label, spec.Model.UsageNote
		if mode := spec.Model.GenerationMode; mode != nil {
			label, note = mode.FamilyLabel, mode.FamilyNote
		}
		if label != variant.label || note != variant.note {
			t.Errorf("%s lost its route presentation: %q, %q", spec.Model.ID, label, note)
		}
	}
}

func TestVeoRemixRequiresConfirmedExtSource(t *testing.T) {
	p, err := loadPlugin()
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"veo3.1-fast", "veo3.1-quality"} {
		t.Run(model, func(t *testing.T) {
			for _, source := range []struct {
				id       string
				fallback bool
				want     bool
			}{
				{model, false, true},
				{model, true, false},
				{model + "-official", false, false},
			} {
				var params map[string]any
				if !strings.HasSuffix(source.id, "-official") {
					params = map[string]any{"official_fallback": source.fallback}
				}
				request := generation(t, source.id, "video.generate", "A short scene", params, nil)
				if got := p.canProduceTaskSource(p.models[source.id], request); got != source.want {
					t.Errorf("%s with fallback=%t eligible=%t, want %t", source.id, source.fallback, got, source.want)
				}
			}
			if p.models[model+"-remix"].Model.TaskContinuation.Accepts(model, nil) {
				t.Fatal("a historical receipt without route proof became eligible")
			}
		})
	}
}
