package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const MaxWorkerResponseBytes = 1 << 20
const MaxWorkerOutputBytes = 512 << 10

// WorkerRequest is a host-created invocation, not an endpoint or permission.
// The request digest is the SHA-256 of the exact transmitted JSON bytes.
type WorkerRequest struct {
	Version          int             `json:"version"`
	TaskID           string          `json:"taskId"`
	ReleaseID        string          `json:"releaseId"`
	PortID           string          `json:"portId"`
	DefinitionDigest string          `json:"definitionDigest"`
	Implementation   string          `json:"implementation"`
	Arguments        json.RawMessage `json:"arguments"`
}

type WorkerOutput struct {
	Kind   string `json:"kind"`
	Mime   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Data   []byte `json:"data"`
}

// WorkerObservation is private original evidence. A released claim is made by
// the explicitly trusted external worker, not by a package or Docker inspector.
type WorkerObservation struct {
	Version        int             `json:"version"`
	TaskID         string          `json:"taskId"`
	ReleaseID      string          `json:"releaseId"`
	RequestDigest  string          `json:"requestDigest"`
	Implementation string          `json:"implementation"`
	State          string          `json:"state"`
	Released       bool            `json:"released"`
	Value          json.RawMessage `json:"value,omitempty"`
	Outputs        []WorkerOutput  `json:"outputs,omitempty"`
}

func ValidExternalWorkerDescriptor(d Descriptor) bool {
	return d.Runtime == "worker" && d.Generation == nil && len(d.Requirements) == 0 && len(d.Contributions) == 1 && d.Contributions[0].Kind == "worker" && d.Definitions[d.Contributions[0].ID] != ""
}

func decodeWorkerJSON(body []byte, limit int, target any) error {
	if len(body) == 0 || len(body) > limit {
		return ErrContract
	}
	value, err := strictJSON(body)
	if err != nil {
		return err
	}
	root, object := value.(map[string]any)
	if !object {
		return ErrContract
	}
	switch target.(type) {
	case *WorkerRequest:
		if !workerMembers(root, []string{"version", "taskId", "releaseId", "portId", "definitionDigest", "implementation", "arguments"}) {
			return ErrContract
		}
	case *WorkerObservation:
		if !workerMembers(root, []string{"version", "taskId", "releaseId", "requestDigest", "implementation", "state", "released"}, "value", "outputs") {
			return ErrContract
		}
		if _, ok := root["released"].(bool); !ok {
			return ErrContract
		}
		if raw, exists := root["outputs"]; exists {
			files, ok := raw.([]any)
			if !ok {
				return ErrContract
			}
			for _, file := range files {
				members, ok := file.(map[string]any)
				if !ok || !workerMembers(members, []string{"kind", "mime", "sha256", "bytes", "data"}) {
					return ErrContract
				}
			}
		}
	case *WorkerPolicy:
		if !workerMembers(root, []string{"version", "origin", "allowHTTP", "implementation", "timeoutMs"}, "authorizationEnv") {
			return ErrContract
		}
		if _, ok := root["allowHTTP"].(bool); !ok {
			return ErrContract
		}
		if value, exists := root["authorizationEnv"]; exists {
			if _, ok := value.(string); !ok {
				return ErrContract
			}
		}
	default:
		return ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return ErrContract
	}
	return nil
}

func workerMembers(root map[string]any, required []string, optional ...string) bool {
	for _, name := range required {
		if _, ok := root[name]; !ok {
			return false
		}
	}
	for name := range root {
		if !slices.Contains(required, name) && !slices.Contains(optional, name) {
			return false
		}
	}
	return true
}

func workerObject(body []byte) bool {
	if len(body) == 0 || len(body) > 64<<10 {
		return false
	}
	v, err := strictJSON(body)
	_, object := v.(map[string]any)
	return err == nil && object
}

func DecodeWorkerRequest(body []byte) (WorkerRequest, error) {
	var out WorkerRequest
	if decodeWorkerJSON(body, MaxControlBytes, &out) != nil || out.Version != Version || !ValidDigest(out.TaskID) || !ValidDigest(out.ReleaseID) || !ValidDigest(out.DefinitionDigest) || !ValidDigest(out.Implementation) || !aigc.IDPattern.MatchString(out.PortID) || !workerObject(out.Arguments) {
		return WorkerRequest{}, ErrContract
	}
	return out, nil
}

func DecodeWorkerObservation(body []byte) (WorkerObservation, error) {
	var out WorkerObservation
	if decodeWorkerJSON(body, MaxWorkerResponseBytes, &out) != nil || out.Version != Version || !ValidDigest(out.TaskID) || !ValidDigest(out.ReleaseID) || !ValidDigest(out.RequestDigest) || !ValidDigest(out.Implementation) || !slices.Contains([]string{"running", "done", "failed", "canceled"}, out.State) || len(out.Outputs) > 4 {
		return WorkerObservation{}, ErrContract
	}
	if out.State != "done" {
		if out.Value != nil || len(out.Outputs) != 0 || out.State == "running" && out.Released {
			return WorkerObservation{}, ErrContract
		}
		return out, nil
	}
	if !workerObject(out.Value) {
		return WorkerObservation{}, ErrContract
	}
	var total int64
	for _, file := range out.Outputs {
		total += int64(len(file.Data))
		sum := sha256.Sum256(file.Data)
		if !validActionInputKind(file.Kind) || len(file.Mime) == 0 || len(file.Mime) > 128 || strings.ContainsAny(file.Mime, "\x00\r\n") || file.Bytes < 1 || file.Bytes != int64(len(file.Data)) || total > MaxWorkerOutputBytes || file.SHA256 != hex.EncodeToString(sum[:]) {
			return WorkerObservation{}, ErrContract
		}
	}
	return out, nil
}

// Matches verifies the original byte-level request identity without granting
// result adoption, current authority or remote resource-release trust.
func (o WorkerObservation) Matches(body []byte) bool {
	r, err := DecodeWorkerRequest(body)
	sum := sha256.Sum256(body)
	return err == nil && o.TaskID == r.TaskID && o.ReleaseID == r.ReleaseID && o.Implementation == r.Implementation && o.RequestDigest == hex.EncodeToString(sum[:])
}

// SameResult permits a later resource-release witness, never a changed original
// outcome. Object member order is irrelevant; numeric lexemes retain precision.
func (o WorkerObservation) SameResult(other WorkerObservation) bool {
	if o.Version != other.Version || o.TaskID != other.TaskID || o.ReleaseID != other.ReleaseID || o.RequestDigest != other.RequestDigest || o.Implementation != other.Implementation || o.State != other.State || len(o.Outputs) != len(other.Outputs) {
		return false
	}
	if len(o.Value) != 0 || len(other.Value) != 0 {
		left, leftErr := strictJSON(o.Value)
		right, rightErr := strictJSON(other.Value)
		if leftErr != nil || rightErr != nil {
			return false
		}
		leftBody, _ := json.Marshal(left)
		rightBody, _ := json.Marshal(right)
		if !bytes.Equal(leftBody, rightBody) {
			return false
		}
	}
	for index, left := range o.Outputs {
		right := other.Outputs[index]
		if left.Kind != right.Kind || left.Mime != right.Mime || left.SHA256 != right.SHA256 || left.Bytes != right.Bytes || !bytes.Equal(left.Data, right.Data) {
			return false
		}
	}
	return true
}
