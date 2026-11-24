// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// The filter replaces only a validated ASCII b64_json string value with an
// internal marker. jsontext validates the unchanged surrounding JSON grammar.
// One lexical token per Read prevents read-ahead from spooling the next image
// while the caller is still staging the current file.
type payloadFilter struct {
	input                               *bufio.Reader
	pending                             []byte
	lastString                          string
	expectPayload, skipPayload, enabled bool
	nextID                              int
	payloads                            map[string]preparedPayload
}
type preparedPayload struct {
	image *decodedImage
	err   error
}
type originalResponseLimit struct {
	source    io.Reader
	remaining int64
}

func (r *originalResponseLimit) Read(p []byte) (int, error) {
	if r.remaining < 0 {
		return 0, responseFailure("original_json_total_limit")
	}
	p = p[:min(int64(len(p)), r.remaining+1)]
	n, err := r.source.Read(p)
	r.remaining -= int64(n)
	if r.remaining < 0 {
		return 0, responseFailure("original_json_total_limit")
	}
	return n, err
}
func newPayloadFilter(source io.Reader) *payloadFilter {
	return &payloadFilter{input: bufio.NewReaderSize(&originalResponseLimit{source: source, remaining: 256 << 20}, 64<<10), payloads: map[string]preparedPayload{}, skipPayload: true}
}
func (f *payloadFilter) close() {
	for key, p := range f.payloads {
		p.image.close()
		delete(f.payloads, key)
	}
}
func (f *payloadFilter) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(f.pending) == 0 {
		c, err := f.input.ReadByte()
		if err != nil {
			return 0, err
		}
		if c == '"' {
			if f.expectPayload && f.enabled {
				f.expectPayload = false
				f.lastString = ""
				if len(f.payloads) >= 16 {
					return 0, responseFailure("original_output_count_mismatch")
				}
				input := &base64JSONString{reader: f.input, rawBytes: 1}
				var payload preparedPayload
				if f.skipPayload {
					_, err = io.CopyBuffer(io.Discard, input, make([]byte, 64<<10))
				} else {
					payload.image, payload.err = spoolImage(input)
				}
				// A base64 error is semantic (partial events may ignore it), but an invalid
				// JSON string or failed source is never hidden by the replacement marker.
				if err == nil {
					_, err = io.CopyBuffer(io.Discard, input, make([]byte, 64<<10))
				}
				if err != nil {
					payload.image.close()
					return 0, err
				}
				marker := fmt.Sprintf("_original_payload_%d", f.nextID)
				f.nextID++
				f.payloads[marker] = payload
				f.pending, _ = json.Marshal(marker)
			} else {
				token := []byte{'"'}
				escaped := false
				for {
					b, err := f.input.ReadByte()
					if err != nil {
						return 0, io.ErrUnexpectedEOF
					}
					token = append(token, b)
					if len(token) > 64<<10 {
						return 0, responseFailure("original_json_value_limit")
					}
					if !escaped && b == '"' {
						break
					}
					if !escaped && b == '\\' {
						escaped = true
					} else {
						escaped = false
					}
				}
				var text string
				if json.Unmarshal(token, &text) != nil {
					return 0, responseFailure("original_json_invalid")
				}
				f.lastString = strings.ToLower(text)
				f.pending = token
			}
		} else {
			if c == ':' {
				f.expectPayload = f.lastString == "b64_json"
				f.lastString = ""
			} else if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
				f.lastString = ""
				f.expectPayload = false
			}
			f.pending = []byte{c}
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

// JSON payloads must be ASCII base64 (ASCII JSON escapes and CR/LF supported).
// This reader validates string boundaries/escapes before the grammar decoder
// sees a marker; it never normalizes malformed controls into valid JSON.
type base64JSONString struct {
	reader   *bufio.Reader
	rawBytes int64
	ended    bool
	failure  error
}

func (r *base64JSONString) byte() (byte, error) {
	c, err := r.reader.ReadByte()
	if err != nil {
		if err == io.EOF {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	}
	r.rawBytes++
	if r.rawBytes > maxJSONValue {
		return 0, responseFailure("original_json_value_limit")
	}
	return c, nil
}
func (r *base64JSONString) Read(p []byte) (n int, returned error) {
	if r.failure != nil {
		return 0, r.failure
	}
	defer func() {
		if returned != nil && returned != io.EOF {
			r.failure = returned
		}
	}()
	if r.ended {
		return 0, io.EOF
	}
	for n < len(p) {
		c, err := r.byte()
		if err != nil {
			return n, err
		}
		if c == '"' {
			r.ended = true
			return n, io.EOF
		}
		if c < 32 || c >= 128 {
			return n, responseFailure("original_base64_json_invalid")
		}
		if c == '\\' {
			c, err = r.byte()
			if err != nil {
				return n, err
			}
			switch c {
			case '"', '\\', '/':
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			case 'u':
				value := 0
				for i := 0; i < 4; i++ {
					h, err := r.byte()
					if err != nil {
						return n, err
					}
					var digit int
					switch {
					case h >= '0' && h <= '9':
						digit = int(h - '0')
					case h >= 'a' && h <= 'f':
						digit = int(h-'a') + 10
					case h >= 'A' && h <= 'F':
						digit = int(h-'A') + 10
					default:
						return n, responseFailure("original_base64_json_invalid")
					}
					value = value*16 + digit
				}
				if value >= 128 {
					return n, responseFailure("original_base64_json_invalid")
				}
				c = byte(value)
			default:
				return n, responseFailure("original_base64_json_invalid")
			}
		}
		p[n] = c
		n++
	}
	return n, nil
}

type decodedImage struct {
	file   *os.File
	size   int64
	digest string
}

func (f *decodedImage) close() {
	if f != nil && f.file != nil {
		name := f.file.Name()
		_ = f.file.Close()
		_ = os.Remove(name)
		f.file = nil
	}
}
func spoolImage(source io.Reader) (*decodedImage, error) {
	file, err := os.CreateTemp("", "ordiweft-gpt-output-*")
	if err != nil {
		return nil, responseFailure("original_output_temp_unavailable")
	}
	image := &decodedImage{file: file}
	ok := false
	defer func() {
		if !ok {
			image.close()
		}
	}()
	hash := sha256.New()
	image.size, err = io.CopyBuffer(io.MultiWriter(file, hash), io.LimitReader(base64.NewDecoder(base64.StdEncoding, source), maxImageBytes+1), make([]byte, 64<<10))
	if err != nil || image.size < 1 || image.size > maxImageBytes {
		return nil, responseFailure("original_output_invalid")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, responseFailure("original_output_temp_unavailable")
	}
	image.digest = hex.EncodeToString(hash.Sum(nil))
	ok = true
	return image, nil
}
func (r *responseJSON) image(capture bool) (*decodedImage, error, error) {
	r.payloads.enabled = true
	r.payloads.skipPayload = !capture
	defer func() { r.payloads.skipPayload = true; r.payloads.enabled = false }()
	value, err := r.value(1024)
	if err != nil {
		return nil, nil, err
	}
	var marker string
	if json.Unmarshal(value, &marker) != nil {
		return nil, nil, responseFailure("original_output_invalid")
	}
	prepared, exists := r.payloads.payloads[marker]
	if !exists {
		return nil, nil, responseFailure("original_output_invalid")
	}
	delete(r.payloads.payloads, marker)
	return prepared.image, prepared.err, nil
}
