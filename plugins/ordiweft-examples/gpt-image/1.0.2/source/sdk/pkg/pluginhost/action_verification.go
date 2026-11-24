package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

const MaxActionVerificationFixtureBytes = 6 << 20

type ActionVerificationInput struct {
	Kind string `json:"kind"`
	Role string `json:"role"`
	Data []byte `json:"data"`
}

type ActionVerificationOutput struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Mime   string `json:"mime"`
}

// ActionVerificationFixture is supplemental test data, never a script, grant,
// business asset reference or a candidate-provided test success certificate.
type ActionVerificationFixture struct {
	Version         int                        `json:"version"`
	PortID          string                     `json:"portId"`
	Arguments       json.RawMessage            `json:"arguments"`
	Inputs          []ActionVerificationInput  `json:"inputs"`
	ExpectedValue   json.RawMessage            `json:"expectedValue"`
	ExpectedOutputs []ActionVerificationOutput `json:"expectedOutputs,omitempty"`
}

func DecodeActionVerificationFixture(body []byte, port string) (ActionVerificationFixture, error) {
	var out ActionVerificationFixture
	if len(body) == 0 || len(body) > MaxActionVerificationFixtureBytes {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Version != 1 || out.PortID != port || !actionObject(out.Arguments) || !actionObject(out.ExpectedValue) || out.Inputs == nil || len(out.Inputs) > 4 {
		return out, ErrContract
	}
	total := 0
	for _, input := range out.Inputs {
		if input.Kind != "image" && input.Kind != "audio" && input.Kind != "document" || len(input.Role) > 128 || strings.ContainsAny(input.Role, "\x00\r\n") || len(input.Data) == 0 || len(input.Data) > 1<<20 {
			return out, ErrContract
		}
		total += len(input.Data)
	}
	if total > 4<<20 {
		return out, ErrContract
	}
	if len(out.ExpectedOutputs) > 4 {
		return out, ErrContract
	}
	var outputBytes int64
	for _, output := range out.ExpectedOutputs {
		if output.Kind != "image" && output.Kind != "audio" && output.Kind != "document" || !ValidDigest(output.SHA256) || output.Bytes < 1 || output.Bytes > 64<<20 || len(output.Mime) < 1 || len(output.Mime) > 128 || strings.ContainsAny(output.Mime, "\x00\r\n") {
			return out, ErrContract
		}
		outputBytes += output.Bytes
	}
	if outputBytes > 64<<20 {
		return out, ErrContract
	}
	return out, nil
}

// ActionValueDigest sorts object members and preserves exact JSON numbers.
// It does not round fixture expectations through float64.
func ActionValueDigest(body []byte) (string, error) {
	if !actionObject(body) {
		return "", ErrContract
	}
	v, err := strictJSON(body)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", ErrContract
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ActionVerificationContextDigest(release, image, fixture string) (string, error) {
	if !ValidDigest(release) || !strings.HasPrefix(image, "sha256:") || !ValidDigest(strings.TrimPrefix(image, "sha256:")) || !ValidDigest(fixture) {
		return "", ErrContract
	}
	body, _ := json.Marshal([]string{"offline-action-contract-verifier-v1", release, image, fixture})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
