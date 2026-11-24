package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const MaxVerificationFixtureBytes = 8 << 20

type VerificationExchange struct {
	CallID      string `json:"callId"`
	Operation   string `json:"operation"`
	Status      int    `json:"status"`
	Body        []byte `json:"body"`
	WaitAttempt *int   `json:"waitAttempt,omitempty"`
}

type VerificationDownload struct {
	CallID       string `json:"callId"`
	Name         string `json:"name"`
	SourceCallID string `json:"sourceCallId,omitempty"`
	SourceWaitID string `json:"sourceWaitId,omitempty"`
	Status       int    `json:"status"`
	Body         []byte `json:"body"`
}

// VerificationFixture is data consumed by the host-owned verifier, never a
// script or a grant. It exercises one media result; the actual host determines
// which declared generation capabilities and byte validators it implements.
type VerificationFixture struct {
	Version      int                    `json:"version"`
	Generation   aigc.Generation        `json:"generation"`
	Inputs       []aigc.Input           `json:"inputs"`
	Exchanges    []VerificationExchange `json:"exchanges"`
	OutputSHA256 string                 `json:"outputSha256"`
	Downloads    []VerificationDownload `json:"downloads,omitempty"`
}

func DecodeVerificationFixture(body []byte, manifest aigc.Manifest) (VerificationFixture, error) {
	var fixture VerificationFixture
	if len(body) == 0 || len(body) > MaxVerificationFixtureBytes {
		return fixture, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return fixture, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&fixture) != nil || d.Decode(new(any)) != io.EOF || fixture.Version != Version || !ValidDigest(fixture.OutputSHA256) || len(fixture.Inputs) > MaxTaskInputs || len(fixture.Exchanges) < 1 || len(fixture.Exchanges) > 32 {
		return fixture, ErrContract
	}
	total := 0
	for _, input := range fixture.Inputs {
		if input.URL != "" || len(input.Data) > 1<<20 {
			return fixture, ErrContract
		}
		total += len(input.Data)
	}
	seen := map[string]bool{}
	sources := map[string]bool{}
	waits := map[string]int{}
	for _, exchange := range fixture.Exchanges {
		key := exchange.CallID
		if exchange.WaitAttempt != nil {
			attempt := *exchange.WaitAttempt
			if attempt < 0 || attempt >= 32 || attempt != waits[exchange.CallID] || sources[exchange.CallID] {
				return fixture, ErrContract
			}
			waits[exchange.CallID] = attempt + 1
			key += "#" + strconv.Itoa(attempt)
		} else if waits[exchange.CallID] > 0 {
			return fixture, ErrContract
		}
		if !opaqueID.MatchString(exchange.CallID) || seen[key] || !aigc.IDPattern.MatchString(exchange.Operation) || exchange.Status < 200 || exchange.Status > 599 || len(exchange.Body) > 1<<20 {
			return fixture, ErrContract
		}
		seen[key] = true
		if exchange.WaitAttempt == nil {
			sources[exchange.CallID] = true
		}
		total += len(exchange.Body)
	}
	if len(fixture.Downloads) > MaxTaskOutputs {
		return fixture, ErrContract
	}
	for _, download := range fixture.Downloads {
		validSource := download.SourceWaitID == "" && sources[download.SourceCallID] || download.SourceCallID == "" && waits[download.SourceWaitID] > 0
		if !opaqueID.MatchString(download.CallID) || seen[download.CallID] || waits[download.CallID] > 0 || !aigc.IDPattern.MatchString(download.Name) || !validSource || download.Status < 200 || download.Status > 599 || len(download.Body) > 1<<20 {
			return fixture, ErrContract
		}
		seen[download.CallID] = true
		total += len(download.Body)
	}
	if total > 4<<20 {
		return fixture, ErrContract
	}
	for _, model := range manifest.Models {
		if model.ID == fixture.Generation.Model {
			parameters, err := aigc.NormalizeParameters(model, fixture.Generation.Parameters)
			if err != nil {
				return fixture, ErrContract
			}
			fixture.Generation.Parameters = parameters
			break
		}
	}
	payload, err := json.Marshal(fixture.Generation)
	if err != nil || aigc.ValidateGeneration(manifest, aigc.Request{Payload: payload, Inputs: fixture.Inputs}) != nil {
		return fixture, ErrContract
	}
	// Validate the actual control-frame shape with fixed placeholder handles.
	// The verifier later replaces them with hashes of the real fixture bytes.
	start := TaskStart{HostProtocol: Version, APIVersion: manifest.APIVersion, Generation: fixture.Generation, Inputs: []TaskInput{}}
	for _, input := range fixture.Inputs {
		start.Inputs = append(start.Inputs, TaskInput{Kind: input.Kind, Role: input.Role, Asset: AssetHandle{ID: "fixture", SHA256: fixture.OutputSHA256, Bytes: int64(len(input.Data)), Mime: "application/octet-stream"}})
	}
	if start.Validate() != nil {
		return fixture, ErrContract
	}
	return fixture, nil
}
