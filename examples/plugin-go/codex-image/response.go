// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

type responseError struct{ code string }

func (e *responseError) Error() string  { return e.code }
func responseFailure(code string) error { return &responseError{code} }

// Only fixed local classifications or the validated host code are reported.
// JSON parser messages and OS paths are never included in a business receipt.
func resultFailureCode(err error) string {
	var response *responseError
	if errors.As(err, &response) {
		return response.code
	}
	var host *pluginhost.HostError
	if errors.As(err, &host) {
		return "host_" + host.Failure.Code
	}
	if errors.Is(err, pluginhost.ErrContract) {
		return "private_contract_invalid"
	}
	return "original_result_unconfirmed"
}

const maxImageBytes = 50 << 20
const maxJSONValue = 70 << 20

// Keep JSON syntax validation in the standard decoder. A budget is renewed for
// each token/value, not for each Read, so an oversized scalar cannot grow to the
// entire response limit. Decoder read-ahead is at most one 32KiB source chunk.
type responseBudget struct {
	source           io.Reader
	remaining, total int64
}

func (r *responseBudget) Read(p []byte) (int, error) {
	if r.remaining <= 0 || r.total <= 0 {
		return 0, responseFailure("original_json_limit")
	}
	p = p[:min(int64(len(p)), r.remaining, r.total, 32<<10)]
	n, err := r.source.Read(p)
	r.remaining -= int64(n)
	r.total -= int64(n)
	return n, err
}

type responseJSON struct {
	source   *responseBudget
	decoder  *jsontext.Decoder
	payloads *payloadFilter
}

func newResponseJSON(r io.Reader) *responseJSON {
	filter := newPayloadFilter(r)
	b := &responseBudget{source: filter, total: (256 << 20) + 1}
	return &responseJSON{source: b, decoder: jsontext.NewDecoder(b), payloads: filter}
}
func (r *responseJSON) token() (jsontext.Token, error) {
	r.source.remaining = 64 << 10
	return r.decoder.ReadToken()
}
func (r *responseJSON) value(limit int64) (jsontext.Value, error) {
	r.source.remaining = limit + (32 << 10)
	v, err := r.decoder.ReadValue()
	if err == nil && int64(len(v)) > limit {
		err = responseFailure("original_json_value_limit")
	}
	return v, err
}
func (r *responseJSON) object(field func(string) error) error {
	t, err := r.token()
	if err != nil || t.Kind() != '{' {
		return responseFailure("original_object_invalid")
	}
	seen := map[string]bool{}
	for {
		t, err = r.token()
		if err != nil {
			return err
		}
		if t.Kind() == '}' {
			return nil
		}
		if t.Kind() != '"' {
			return responseFailure("original_field_invalid")
		}
		key := strings.ToLower(t.String())
		if seen[key] {
			return responseFailure("original_field_duplicate")
		}
		seen[key] = true
		if err = field(key); err != nil {
			return err
		}
	}
}
func (r *responseJSON) end() error {
	_, err := r.token()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return responseFailure("original_json_trailing")
}
func (r *responseJSON) skip() error { _, err := r.value(64 << 10); r.payloads.close(); return err }

func readImages(reader io.Reader, expected int, stage func(int, *decodedImage) error, metadata map[string]any) (json.RawMessage, error) {
	r := newResponseJSON(reader)
	defer r.payloads.close()
	count := 0
	found := false
	var usage json.RawMessage
	err := r.object(func(key string) error {
		switch key {
		case "data":
			found = true
			t, err := r.token()
			if err != nil || t.Kind() != '[' {
				return responseFailure("original_data_invalid")
			}
			for r.decoder.PeekKind() != ']' {
				if count >= expected {
					return responseFailure("original_output_count_mismatch")
				}
				var image *decodedImage
				err = r.object(func(key string) error {
					if key == "generation_id" {
						value, err := r.value(4096)
						var id string
						if err != nil || json.Unmarshal(value, &id) != nil {
							return responseFailure("original_metadata_invalid")
						}
						metadata["generationId"] = id
						return nil
					}
					if key != "b64_json" {
						return r.skip()
					}
					var semantic, syntax error
					image, semantic, syntax = r.image(true)
					if syntax != nil {
						return syntax
					}
					return semantic
				})
				if err == nil && image == nil {
					err = responseFailure("original_output_missing")
				}
				if err == nil {
					err = stage(count, image)
				}
				image.close()
				if err != nil {
					return err
				}
				count++
			}
			t, err = r.token()
			if err != nil || t.Kind() != ']' {
				return responseFailure("original_data_invalid")
			}
			return nil
		case "model", "background", "size", "quality", "output_format":
			value, err := r.value(4096)
			var text string
			if err != nil || json.Unmarshal(value, &text) != nil {
				return responseFailure("original_metadata_invalid")
			}
			metadata[key] = text
			return nil
		case "usage":
			value, err := r.value(64 << 10)
			if err == nil {
				usage = append(json.RawMessage(nil), value...)
			}
			return err
		default:
			return r.skip()
		}
	})
	if err != nil {
		return nil, err
	}
	if !found || count != expected {
		return nil, responseFailure("original_output_count_mismatch")
	}
	return usage, r.end()
}
