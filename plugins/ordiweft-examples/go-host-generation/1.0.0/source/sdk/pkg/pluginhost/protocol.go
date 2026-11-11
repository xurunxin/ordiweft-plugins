package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"strings"
)

const MaxControlBytes = 128 << 10
const MaxChunkBytes = 64 << 10

var opaqueID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var decimalQuantity = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
var diagnosticCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var mimeValue = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)

// Call is the untrusted control envelope. Identity, release, grants, endpoint,
// budgets and stop epochs are deliberately absent: the receiver binds them.
type Call struct {
	Version   int             `json:"version"`
	ID        string          `json:"id"`
	Service   string          `json:"service"`
	Operation string          `json:"operation"`
	Input     json.RawMessage `json:"input"`
}

func DecodeCall(body []byte) (Call, error) {
	var call Call
	if len(body) == 0 || len(body) > MaxControlBytes {
		return call, ErrContract
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&call) != nil || decoder.Decode(new(any)) != io.EOF {
		return call, ErrContract
	}
	if call.Version != Version || !opaqueID.MatchString(call.ID) || !slices.Contains(operations[call.Service], call.Operation) || len(call.Input) == 0 || !json.Valid(call.Input) || call.Input[0] != '{' {
		return call, ErrContract
	}
	return call, nil
}

// AssetHandle describes fixed bytes, not permission to read them. Host lookups
// recheck current task ownership/ACL and never accept paths in place of handles.
type AssetHandle struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Mime   string `json:"mime"`
}

func (h AssetHandle) Validate() error {
	if !opaqueID.MatchString(h.ID) || !ValidDigest(h.SHA256) || h.Bytes < 0 || h.Bytes > 512<<20 || len(h.Mime) > 128 || !mimeValue.MatchString(h.Mime) {
		return ErrContract
	}
	return nil
}

// Frame is an observation on an already established stream. Sequence and
// checkpoint IDs identify original data; reconnect never grants a new submit.
type Frame struct {
	Version      int    `json:"version"`
	StreamID     string `json:"streamId"`
	Sequence     int64  `json:"sequence"`
	Kind         string `json:"kind"` // progress, chunk, checkpoint, end
	Data         []byte `json:"data,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
}

func (f Frame) Validate() error {
	if f.Version != Version || !opaqueID.MatchString(f.StreamID) || f.Sequence < 0 || f.Sequence > 9007199254740991 || !slices.Contains([]string{"progress", "chunk", "checkpoint", "end"}, f.Kind) || len(f.Data) > MaxChunkBytes {
		return ErrContract
	}
	if (f.Kind == "checkpoint") != (f.CheckpointID != "") || f.CheckpointID != "" && !opaqueID.MatchString(f.CheckpointID) || f.Kind == "end" && len(f.Data) != 0 {
		return ErrContract
	}
	return nil
}

func (f Failure) Validate() error {
	if !diagnosticCode.MatchString(f.Code) || !slices.Contains([]string{"prepare", "submit", "observe", "output", "adopt"}, f.Stage) || !slices.Contains([]string{"not_sent", "sent", "unknown"}, f.Submission) || len(f.RequestID) > 256 || strings.ContainsAny(f.RequestID, "\x00\r\n") {
		return ErrContract
	}
	return nil
}

func (m Meter) Validate() error {
	if !diagnosticCode.MatchString(m.Name) || !diagnosticCode.MatchString(m.Unit) || len(m.Quantity) > 80 || !decimalQuantity.MatchString(m.Quantity) || !slices.Contains([]string{"provider_reported", "estimated", "confirmed"}, m.Source) {
		return ErrContract
	}
	return nil
}

// ValidateReport rejects a plugin's claim of host confirmation. A valid report
// is still evidence from the plugin, never a price, refund or settlement fact.
func (m Meter) ValidateReport() error {
	if m.Validate() != nil || m.Source == "confirmed" {
		return ErrContract
	}
	return nil
}
