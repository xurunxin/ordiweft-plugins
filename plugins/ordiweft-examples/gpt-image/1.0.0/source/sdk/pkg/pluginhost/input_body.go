package pluginhost

import "bytes"

// InputBodySource selects exactly one original input for a host-bound upload.
// It is installed data, never a control-frame asset ID, body or path.
type InputBodySource struct {
	Index int    `json:"index"`
	Role  string `json:"role,omitempty"`
}

func (s InputBodySource) valid() bool {
	if s.Role != "" {
		return s.Index == 0 && policyName(s.Role, 128)
	}
	return s.Index >= 0 && s.Index < 256
}

func (s InputBodySource) input(inputs []RequestInput) (RequestInput, error) {
	if !s.valid() || len(inputs) > 256 {
		return RequestInput{}, ErrContract
	}
	if s.Role == "" {
		if s.Index >= len(inputs) {
			return RequestInput{}, ErrContract
		}
		return inputs[s.Index], nil
	}
	var selected RequestInput
	matches := 0
	for _, input := range inputs {
		if input.Role == s.Role {
			selected = input
			matches++
		}
	}
	if matches != 1 {
		return RequestInput{}, ErrContract
	}
	return selected, nil
}

func (s InputBodySource) build(inputs []RequestInput) ([]byte, error) {
	input, err := s.input(inputs)
	if err != nil || validateRequestInputBytes(input) != nil {
		return nil, ErrContract
	}
	return bytes.Clone(input.Data), nil
}

func (s InputBodySource) validate(inputs []RequestInput, body []byte) error {
	input, err := s.input(inputs)
	if err != nil {
		return ErrContract
	}
	input.Data = body
	return validateRequestInputBytes(input)
}
