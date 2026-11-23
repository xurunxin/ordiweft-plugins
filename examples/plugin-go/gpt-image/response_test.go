// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

type fragmentedReader struct{ source io.Reader }

func (r fragmentedReader) Read(p []byte) (int, error) { return r.source.Read(p[:min(len(p), 1)]) }
func TestResponseConfirmationAndCleanup(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", os.Getenv("TMPDIR"))
	for _, tc := range []struct {
		name, body  string
		stream, bad bool
		want        int
	}{
		{name: "sync", body: `{"data":[{"b64_json":"YWJj"},{"b64_json":"ZA=="}],"usage":{"n":2}}`, want: 2},
		{name: "escaped", body: `{"DATA":[{"B64_JSON":"Y\u0057Jj\r\n"}]}`, want: 1},
		{name: "bad-base64", body: `{"data":[{"b64_json":"a!"}]}`, bad: true, want: 1},
		{name: "non-ascii", body: `{"data":[{"b64_json":"\u0100"}]}`, bad: true, want: 1},
		{name: "missing", body: `{"data":[{}]}`, bad: true, want: 1},
		{name: "duplicate", body: `{"data":[{"b64_json":"YQ==","B64_JSON":"Yg=="}]}`, bad: true, want: 1},
		{name: "count", body: `{"data":[{"b64_json":"YQ=="}]}`, bad: true, want: 2},
		{name: "trailing", body: `{"data":[{"b64_json":"YQ=="}]}{}`, bad: true, want: 1},
		{name: "truncated", body: `{"data":[{"b64_json":"YQ=="}]`, bad: true, want: 1},
		{name: "sse", body: `event: ignored
data: {
data: "b64_json":"YWJj","type":"image_edit.completed","usage":{"n":1}}

data: [DONE]

`, stream: true, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			stage := func(_ int, f *decodedImage) error {
				data, err := io.ReadAll(f.file)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(data)
				if int64(len(data)) != f.size || hex.EncodeToString(sum[:]) != f.digest {
					t.Fatal("decoded file changed")
				}
				count++
				return nil
			}
			var err error
			if tc.stream {
				_, err = readImageSSE(fragmentedReader{strings.NewReader(tc.body)}, func(int) error { return nil }, stage)
			} else {
				_, err = readImages(fragmentedReader{strings.NewReader(tc.body)}, tc.want, stage)
			}
			if (err != nil) != tc.bad {
				t.Fatalf("bad=%v error=%v", tc.bad, err)
			}
			if !tc.bad && count != tc.want {
				t.Fatal("output count", count)
			}
			files, _ := os.ReadDir(os.TempDir())
			if len(files) != 0 {
				t.Fatal("temporary files retained", len(files))
			}
		})
	}
}
func TestSSEWaitsForOriginalEOF(t *testing.T) {
	complete := `data: {"b64_json":"YWJj","type":"image_generation.completed"}` + "\n\n"
	partial := `data: {"type":"image_generation.partial_image","b64_json":"not base64"}` + "\n\n"
	for _, tc := range []struct {
		name, body string
		bad        bool
		partials   int
	}{
		{"partial-before", partial + complete, false, 1}, {"partial-after", complete + partial, false, 1},
		{"duplicate", complete + complete, true, 0}, {"error-after", complete + `data: {"type":"error"}` + "\n\n", true, 0},
		{"truncated", complete + `data: {"type":`, true, 0}, {"no-completion", partial, true, 1},
		{"field-order-partial", `data: {"b64_json":"not base64","type":"image_edit.partial_image"}` + "\n\n" + complete, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			staged, partials := 0, 0
			_, err := readImageSSE(fragmentedReader{strings.NewReader(tc.body)}, func(n int) error { partials = n; return nil }, func(int, *decodedImage) error { staged++; return nil })
			if (err != nil) != tc.bad || partials != tc.partials || tc.bad && staged != 0 || !tc.bad && staged != 1 {
				t.Fatalf("error=%v staged=%d partials=%d", err, staged, partials)
			}
		})
	}
}
func TestResponseLimitsAndUnknownValues(t *testing.T) {
	for _, body := range []string{`{"data":[{"b64_json":"YQ=="}],"usage":"` + strings.Repeat("x", 65<<10) + `"}`, `{"` + strings.Repeat("x", 128<<10) + `":0,"data":[{"b64_json":"YQ=="}]}`} {
		if _, err := readImages(strings.NewReader(body), 1, func(int, *decodedImage) error { return nil }); err == nil {
			t.Fatal("unbounded field accepted")
		}
	}
	var got []byte
	body := `{"ignored":{"nested":[null,true,{"x":"y"}]},"data":[{"b64_json":"YWJj","ignored":false}],"usage":null}`
	_, err := readImages(strings.NewReader(body), 1, func(_ int, f *decodedImage) error { got, _ = io.ReadAll(f.file); return nil })
	if err != nil || string(got) != "abc" {
		t.Fatal(err, string(got))
	}
}

type repeatedBytes struct{}

func (repeatedBytes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}
func TestLargeResponseInBoundedContainer(t *testing.T) {
	if os.Getenv("ORDIWEFT_GPT_PARSER_LARGE") != "1" {
		t.Skip("explicit bounded container gate")
	}
	debug.SetMemoryLimit(160 << 20)
	for _, stream := range []bool{false, true} {
		reader, writer := io.Pipe()
		producer := make(chan error, 1)
		expected := 3
		if stream {
			expected = 1
		}
		go func() {
			var err error
			defer func() { _ = writer.CloseWithError(err); producer <- err }()
			if stream {
				_, err = io.WriteString(writer, `data: {"type":"image_generation.completed","b64_json":"`)
			} else {
				_, err = io.WriteString(writer, `{"data":[`)
			}
			for i := 0; i < expected && err == nil; i++ {
				if !stream {
					if i > 0 {
						_, err = io.WriteString(writer, ",")
					}
					if err == nil {
						_, err = io.WriteString(writer, `{"b64_json":"`)
					}
				}
				encoder := base64.NewEncoder(base64.StdEncoding, writer)
				if err == nil {
					_, err = io.CopyN(encoder, repeatedBytes{}, maxImageBytes)
				}
				if err == nil {
					err = encoder.Close()
				}
				if err == nil {
					_, err = io.WriteString(writer, `"}`)
				}
			}
			if err == nil {
				if stream {
					_, err = io.WriteString(writer, "\n\n")
				} else {
					_, err = io.WriteString(writer, `]}`)
				}
			}
		}()
		staged := 0
		stage := func(_ int, f *decodedImage) error {
			hash := sha256.New()
			n, err := io.Copy(hash, f.file)
			if err != nil || n != maxImageBytes || f.digest != hex.EncodeToString(hash.Sum(nil)) {
				return fmt.Errorf("decoded size/digest mismatch")
			}
			staged++
			return nil
		}
		var err error
		if stream {
			_, err = readImageSSE(reader, func(int) error { return nil }, stage)
		} else {
			_, err = readImages(reader, expected, stage)
		}
		_ = reader.CloseWithError(err)
		producerErr := <-producer
		if err != nil || producerErr != nil || staged != expected {
			t.Fatalf("stream=%v error=%v producer=%v staged=%d", stream, err, producerErr, staged)
		}
	}
	peak, err := os.ReadFile("/sys/fs/cgroup/memory.peak")
	if err != nil {
		t.Fatal(err)
	}
	record, _ := json.Marshal(map[string]any{"memoryPeakBytes": strings.TrimSpace(string(peak)), "syncOutputs": 3, "sseOutputs": 1, "bytesEach": maxImageBytes, "stagePeer": "local decoded-file digest callback; not host or provider"})
	t.Log(string(record))
}

func TestFailureReceiptExcludesFreeFormMessages(t *testing.T) {
	for _, err := range []error{errors.New("https://private.invalid/secret-and-provider-text"), &os.PathError{Op: "open", Path: "/private/owner/source", Err: os.ErrPermission}} {
		if got := resultFailureCode(err); got != "original_result_unconfirmed" {
			t.Fatal("free-form diagnostic escaped", got)
		}
	}
	wrapped := fmt.Errorf("private detail: %w", responseFailure("original_json_limit"))
	if resultFailureCode(wrapped) != "original_json_limit" {
		t.Fatal("safe classification lost")
	}
	host := &pluginhost.HostError{Failure: pluginhost.Failure{Code: "unavailable", Stage: "output", Submission: "unknown"}}
	if resultFailureCode(host) != "host_unavailable" {
		t.Fatal("host classification lost")
	}
}

func TestPayloadStreamingPreservesBase64Semantics(t *testing.T) {
	for _, encoded := range []string{"YWJj", "AB==", "YQ==\r\n", "YQ==Yg==", "YWJj====", "YQ== ", "YQ==\t", "YQ==\x00", "YQ==\n!", ""} {
		original, expected := base64.StdEncoding.DecodeString(encoded)
		if len(original) == 0 && expected == nil {
			expected = io.ErrUnexpectedEOF
		}
		token, _ := json.Marshal(encoded)
		body := `{"data":[{"b64_json":` + string(token) + `}]}`
		var actual []byte
		_, err := readImages(fragmentedReader{strings.NewReader(body)}, 1, func(_ int, f *decodedImage) error { actual, _ = io.ReadAll(f.file); return nil })
		if (err != nil) != (expected != nil) || err == nil && string(actual) != string(original) {
			t.Fatalf("base64 parity changed for %q: %v / %v", encoded, err, expected)
		}
	}
}
func FuzzPayloadFilterCannotRepairMalformedJSON(f *testing.F) {
	for _, seed := range []string{`{"data":[{"b64_json":"YQ=="}]}`, `{"data":[{"b64_json":"YQ==\q"}]}`, `{"data":[{"b64_json":"YQ=="}],"ignored":{"b64_json":"invalid"}}`, `{"data":[{"b64_json":"YQ==\u00QQ"}]}`, `{"data":[{"b64_json":"YQ==\u000a"}]}`, "{\"data\":[{\"b64_json\":\"YQ==\n\"}]}"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 4096 {
			t.Skip()
		}
		_, err := readImages(strings.NewReader(string(body)), 1, func(int, *decodedImage) error { return nil })
		if err == nil && !json.Valid(body) {
			t.Fatal("payload replacement repaired malformed JSON")
		}
	})
}

func TestPayloadStreamingKeepsNonImageMetadataOriginal(t *testing.T) {
	body := `{"data":[{"b64_json":"YQ=="}],"usage":{"b64_json":"YWJj"},"svg_usage":{"model":"gpt-6.2-sol","input_tokens":100,"output_tokens":50,"reasoning_tokens":10,"total_tokens":150},"ignored":{"b64_json":"not base64"}}`
	usage, err := readImages(strings.NewReader(body), 1, func(int, *decodedImage) error { return nil })
	if err != nil || string(usage.image) != `{"b64_json":"YWJj"}` || string(usage.svg) != `{"model":"gpt-6.2-sol","input_tokens":100,"output_tokens":50,"reasoning_tokens":10,"total_tokens":150}` {
		t.Fatal("payload reader changed either usage receipt", string(usage.image), string(usage.svg), err)
	}
}
