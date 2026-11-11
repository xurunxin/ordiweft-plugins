package pluginhost

import "bytes"

// InputBodySource selects exactly one original input for a host-bound upload.
// It is installed data, never a control-frame asset ID, body or path.
type InputBodySource struct {
	Index int `json:"index"`
}

func (s InputBodySource) valid() bool { return s.Index >= 0 && s.Index < 256 }

func (s InputBodySource) build(inputs []RequestInput) ([]byte, error) {
	if !s.valid() || s.Index >= len(inputs) || len(inputs) > 256 || validateRequestInputBytes(inputs[s.Index]) != nil {
		return nil, ErrContract
	}
	return bytes.Clone(inputs[s.Index].Data), nil
}

func (s InputBodySource) validate(inputs []RequestInput, body []byte) error {
	if !s.valid() || s.Index >= len(inputs) || len(inputs) > 256 {
		return ErrContract
	}
	input := inputs[s.Index]
	input.Data = body
	return validateRequestInputBytes(input)
}
