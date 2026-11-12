// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
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

func readImages(reader io.Reader, expected int, stage func(int, *decodedImage) error) (json.RawMessage, error) {
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

// One event exposes only data lines. Long lines are forwarded as ReadSlice
// fragments rather than copied into a Scanner token and a second event buffer.
type sseEvents struct {
	reader *bufio.Reader
	total  int64
}
type sseEvent struct {
	owner                        *sseEvents
	pending                      []byte
	continuation, dataLine, done bool
	lineBytes                    int
}

func (e *sseEvent) Read(p []byte) (int, error) {
	for len(e.pending) == 0 {
		if e.done {
			return 0, io.EOF
		}
		line, err := e.owner.reader.ReadSlice('\n')
		e.owner.total += int64(len(line))
		e.lineBytes += len(line)
		if e.owner.total > 256<<20 || e.lineBytes > 72<<20 {
			return 0, responseFailure("original_stream_limit")
		}
		final := err != bufio.ErrBufferFull
		if err != nil && err != bufio.ErrBufferFull && err != io.EOF {
			return 0, err
		}
		if !e.continuation {
			e.dataLine = bytes.HasPrefix(line, []byte("data:"))
			if e.dataLine {
				line = line[5:]
			}
		}
		if final {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if !e.continuation && len(line) == 0 && !e.dataLine {
				e.done = true
				return 0, io.EOF
			}
			e.lineBytes = 0
		}
		if e.dataLine {
			e.pending = line
			if final {
				e.pending = append(e.pending, '\n')
			}
		}
		e.continuation = !final
		if err == io.EOF {
			e.done = true
		}
	}
	n := copy(p, e.pending)
	e.pending = e.pending[n:]
	return n, nil
}
func readImageSSE(reader io.Reader, partial func(int) error, stage func(int, *decodedImage) error) (json.RawMessage, error) {
	events := &sseEvents{reader: bufio.NewReaderSize(reader, 32<<10)}
	var completed *decodedImage
	defer func() { completed.close() }()
	var usage json.RawMessage
	count := 0
	for {
		if _, err := events.reader.Peek(1); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		event := &sseEvent{owner: events}
		data := bufio.NewReaderSize(event, 4096)
		// Empty and DONE events have no JSON payload; leading whitespace is legal.
		for {
			b, err := data.Peek(1)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if !strings.ContainsRune(" \t\r\n", rune(b[0])) {
				break
			}
			_, _ = data.ReadByte()
		}
		first, err := data.Peek(1)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return nil, err
		}
		if first[0] == '[' {
			raw, err := io.ReadAll(io.LimitReader(data, 32))
			if err != nil || strings.TrimSpace(string(raw)) != "[DONE]" {
				return nil, responseFailure("invalid_original_event")
			}
			if _, err = data.ReadByte(); err != io.EOF {
				return nil, responseFailure("invalid_original_event")
			}
			continue
		}
		r := newResponseJSON(data)
		var kind string
		var image *decodedImage
		var decodeErr error
		var eventUsage json.RawMessage
		err = r.object(func(key string) error {
			switch key {
			case "type":
				value, err := r.value(1024)
				if err == nil {
					err = json.Unmarshal(value, &kind)
				}
				return err
			case "b64_json":
				var syntax error
				capture := kind != "image_generation.partial_image" && kind != "image_edit.partial_image"
				image, decodeErr, syntax = r.image(capture)
				return syntax
			case "usage":
				value, err := r.value(64 << 10)
				if err == nil {
					eventUsage = append(json.RawMessage(nil), value...)
				}
				return err
			default:
				return r.skip()
			}
		})
		if err == nil {
			err = r.end()
		}
		r.payloads.close()
		if err != nil {
			image.close()
			return nil, err
		}
		switch kind {
		case "image_generation.partial_image", "image_edit.partial_image":
			image.close()
			count++
			if err = partial(count); err != nil {
				return nil, err
			}
		case "image_generation.completed", "image_edit.completed":
			if completed != nil {
				image.close()
				return nil, errors.New("duplicate_original_completion")
			}
			if decodeErr != nil || image == nil {
				return nil, responseFailure("original_output_invalid")
			}
			completed = image
			usage = eventUsage
		case "error":
			image.close()
			return nil, responseFailure("original_stream_error")
		default:
			image.close()
		}
	}
	if completed == nil {
		return nil, responseFailure("original_stream_incomplete")
	}
	if err := stage(0, completed); err != nil {
		return nil, fmt.Errorf("original_stage: %w", err)
	}
	return usage, nil
}
