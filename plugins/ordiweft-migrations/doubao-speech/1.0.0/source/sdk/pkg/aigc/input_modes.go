package aigc

import (
	"encoding/json"
	"fmt"
)

func validateInputModes(field Field) error {
	if len(field.InputModes) == 0 {
		return nil
	}
	if field.Type != "string" || len(field.InputModes) > 32 {
		return fmt.Errorf("invalid input mode control")
	}
	for value, mode := range field.InputModes {
		found := false
		for _, choice := range field.Enum {
			if text, ok := choice.(string); ok && text == value {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("input mode must belong to field enum")
		}
		switch mode.Intent {
		case "text", "first_frame", "first_last_frame", "reference", "edit", "extend":
		default:
			return fmt.Errorf("invalid input mode intent")
		}
		for _, limit := range []InputRange{mode.Images, mode.Videos, mode.Audios} {
			if limit.Min < 0 || limit.Max < limit.Min || limit.Max > 80 {
				return fmt.Errorf("invalid input mode range")
			}
		}
		if len(mode.Parameters) > 32 {
			return fmt.Errorf("too many mode parameters")
		}
	}
	return nil
}

func validateModeInputs(model Model, parameters map[string]any, inputs []Input) error {
	counts := map[string]int{}
	for _, input := range inputs {
		counts[input.Kind]++
	}
	for key, field := range model.Fields {
		if len(field.InputModes) == 0 || !enabled(field, parameters) {
			continue
		}
		value, _ := parameters[key].(string)
		mode, ok := field.InputModes[value]
		if !ok {
			return fmt.Errorf("input mode unavailable")
		}
		for kind, limit := range map[string]InputRange{"image": mode.Images, "video": mode.Videos, "audio": mode.Audios} {
			if counts[kind] < limit.Min || counts[kind] > limit.Max {
				return fmt.Errorf("%s references require %d–%d inputs", kind, limit.Min, limit.Max)
			}
		}
		for key, want := range mode.Parameters {
			actual, _ := json.Marshal(parameters[key])
			expected, _ := json.Marshal(want)
			if string(actual) != string(expected) {
				return fmt.Errorf("input mode requires %s=%v", key, want)
			}
		}
	}
	return nil
}

func validateModelInputModes(model Model) error {
	controls := 0
	for control, field := range model.Fields {
		if len(field.InputModes) > 0 {
			controls++
			if controls > 1 {
				return fmt.Errorf("model must have one input mode control")
			}
		}
		for _, mode := range field.InputModes {
			minimum := 0
			for kind, limit := range map[string]InputRange{"image": mode.Images, "video": mode.Videos, "audio": mode.Audios} {
				minimum += limit.Min
				if limit.Max > model.MaxInputs {
					return fmt.Errorf("input mode exceeds model limit")
				}
				if limit.Max > 0 {
					found := false
					for _, allowed := range model.InputKinds {
						found = found || allowed == kind
					}
					if !found {
						return fmt.Errorf("input mode kind unsupported")
					}
				}
			}
			if minimum > model.MaxInputs {
				return fmt.Errorf("input mode minimum exceeds model limit")
			}
			for key, value := range mode.Parameters {
				if key == control {
					return fmt.Errorf("mode cannot override its control")
				}
				definition, ok := model.Fields[key]
				if !ok {
					return fmt.Errorf("unknown mode parameter")
				}
				if _, err := normalizeValue(definition, value, mode.Parameters, key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
