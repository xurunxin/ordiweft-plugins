package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const MaxVerificationFixtureBytes = 8 << 20

type VerificationExchange struct {
	CallID              string `json:"callId"`
	Operation           string `json:"operation"`
	Status              int    `json:"status"`
	Body                []byte `json:"body"`
	WaitAttempt         *int   `json:"waitAttempt,omitempty"`
	ChannelProbe        bool   `json:"channelProbe,omitempty"`
	SubmissionPreflight bool   `json:"submissionPreflight,omitempty"`
}

type VerificationDownload struct {
	CallID       string `json:"callId"`
	Name         string `json:"name"`
	SourceCallID string `json:"sourceCallId,omitempty"`
	SourceWaitID string `json:"sourceWaitId,omitempty"`
	Status       int    `json:"status"`
	Body         []byte `json:"body"`
}

type VerificationOutput struct {
	SHA256 string      `json:"sha256"`
	Voice  *aigc.Voice `json:"voice,omitempty"`
}

// VerificationFixture is data consumed by the host-owned verifier, never a
// script or a grant. It exercises one media or normalized ASR result; the host determines
// which declared generation capabilities and byte validators it implements.
type VerificationFixture struct {
	Version           int                    `json:"version"`
	Generation        aigc.Generation        `json:"generation"`
	Inputs            []aigc.Input           `json:"inputs"`
	Exchanges         []VerificationExchange `json:"exchanges"`
	OutputSHA256      string                 `json:"outputSha256"`
	Outputs           []VerificationOutput   `json:"outputs,omitempty"`
	CheckpointVoiceID string                 `json:"checkpointVoiceId,omitempty"`
	Downloads         []VerificationDownload `json:"downloads,omitempty"`
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
	if d.Decode(&fixture) != nil || d.Decode(new(any)) != io.EOF || fixture.Version != Version || len(fixture.Inputs) > MaxTaskInputs || len(fixture.Exchanges) < 1 || len(fixture.Exchanges) > 32 {
		return fixture, ErrContract
	}
	digest := fixture.OutputSHA256
	if fixture.Outputs == nil {
		if !ValidDigest(digest) || fixture.CheckpointVoiceID != "" {
			return fixture, ErrContract
		}
	} else {
		if digest != "" || len(fixture.Outputs) == 0 || len(fixture.Outputs) > MaxTaskOutputs || fixture.Generation.Capability == "speech.transcribe" {
			return fixture, ErrContract
		}
		digest = fixture.Outputs[0].SHA256
		matchingCheckpoint := fixture.CheckpointVoiceID == ""
		for _, output := range fixture.Outputs {
			if !ValidDigest(output.SHA256) {
				return fixture, ErrContract
			}
			if voice := output.Voice; voice != nil {
				if fixture.Generation.Capability != "voice.design" || voice.Provider != fixture.Generation.Provider || voice.ID == "" ||
					len(voice.ID) > 512 || len(voice.CompatibleModels) == 0 {
					return fixture, ErrContract
				}
				matchingCheckpoint = matchingCheckpoint || voice.ID == fixture.CheckpointVoiceID
			}
		}
		if !matchingCheckpoint {
			return fixture, ErrContract
		}
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
		if exchange.SubmissionPreflight {
			if exchange.WaitAttempt != nil {
				return fixture, ErrContract
			}
			key += "#submission-preflight"
			if !exchange.ChannelProbe {
				if _, err := BindVerificationSubmissionSnapshot(exchange.Body, "fixture", strings.Repeat("a", 64), 1); err != nil {
					return fixture, ErrContract
				}
			}
		}
		if exchange.WaitAttempt != nil {
			attempt := *exchange.WaitAttempt
			if attempt < 0 || attempt >= 32 || attempt != waits[exchange.CallID] || sources[exchange.CallID] {
				return fixture, ErrContract
			}
			if !exchange.ChannelProbe {
				waits[exchange.CallID] = attempt + 1
			}
			key += "#" + strconv.Itoa(attempt)
		} else if waits[exchange.CallID] > 0 {
			return fixture, ErrContract
		}
		if exchange.ChannelProbe {
			key += "#channel-probe"
		}
		if !opaqueID.MatchString(exchange.CallID) || seen[key] || !aigc.IDPattern.MatchString(exchange.Operation) || exchange.Status < 200 || exchange.Status > 599 || len(exchange.Body) > 1<<20 {
			return fixture, ErrContract
		}
		seen[key] = true
		if exchange.WaitAttempt == nil && !exchange.ChannelProbe && !exchange.SubmissionPreflight {
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
		start.Inputs = append(start.Inputs, TaskInput{Kind: input.Kind, Role: input.Role, Asset: AssetHandle{ID: "fixture", SHA256: digest, Bytes: int64(len(input.Data)), Mime: "application/octet-stream"}})
	}
	if start.Validate() != nil {
		return fixture, ErrContract
	}
	return fixture, nil
}

// BindVerificationSubmissionSnapshot binds only the host-owned identity and
// clock in a synthetic upstream template. All service claims remain fixture data.
func BindVerificationSubmissionSnapshot(body []byte, job, digest string, now int64) ([]byte, error) {
	var snapshot SubmissionSnapshot
	if len(body) > 64<<10 || !opaqueID.MatchString(job) || !ValidDigest(digest) || now <= 0 || now > 1<<63-1-60000 {
		return nil, ErrContract
	}
	if _, err := strictJSON(body); err != nil || !canonicalSubmissionKeys(body) {
		return nil, ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&snapshot) != nil || d.Decode(new(any)) != io.EOF || snapshot.Version != 1 || snapshot.JobID != "" || snapshot.RequestDigest != "" || snapshot.ObservedAt != 0 || snapshot.ExpiresAt < 1 || snapshot.ExpiresAt > 60000 {
		return nil, ErrContract
	}
	snapshot.JobID, snapshot.RequestDigest = job, digest
	snapshot.ObservedAt, snapshot.ExpiresAt = now, now+snapshot.ExpiresAt
	return json.Marshal(snapshot)
}
