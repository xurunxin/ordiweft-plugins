package aigc

import "fmt"

type VideoDraftOutput struct {
	Parameter  string `json:"parameter"`
	FinalModel string `json:"finalModel"`
}
type VideoDraftSource struct {
	Parameter string   `json:"parameter"`
	Models    []string `json:"models"`
}

func validateVideoDraftModels(models []Model) error {
	byID := map[string]Model{}
	for _, m := range models {
		byID[m.ID] = m
	}
	for _, m := range models {
		if m.DraftOutput != nil {
			d := m.DraftOutput
			f, ok := m.Fields[d.Parameter]
			final, exists := byID[d.FinalModel]
			if m.Capability != "video.generate" || m.FromDraft != nil || !ok || f.Type != "boolean" || !exists || final.FromDraft == nil {
				return fmt.Errorf("invalid video draft output")
			}
			found := false
			for _, id := range final.FromDraft.Models {
				found = found || id == m.ID
			}
			if !found {
				return fmt.Errorf("draft final model does not accept source")
			}
		}
		if m.FromDraft != nil {
			d := m.FromDraft
			f, ok := m.Fields[d.Parameter]
			if m.Capability != "video.generate" || m.MaxInputs != 0 || len(m.InputKinds) != 0 || m.Prompt != nil && m.Prompt.Required || !ok || f.Type != "string" || !f.Required || len(d.Models) == 0 || len(d.Models) > 16 {
				return fmt.Errorf("invalid video draft source")
			}
			seen := map[string]bool{}
			for _, id := range d.Models {
				src, exists := byID[id]
				if seen[id] || !exists || src.DraftOutput == nil || src.DraftOutput.FinalModel != m.ID {
					return fmt.Errorf("invalid draft source model")
				}
				seen[id] = true
			}
		}
	}
	return nil
}
