// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func (p *plugin) transcribe(ctx context.Context, spec modelSpec, key string, body map[string]any, input aigc.Input) (aigc.Result, error) {
	var empty aigc.Result
	if len(input.Data) == 0 || len(input.Data) > 25_000_000 {
		return empty, errors.New("Whisper requires a local audio file of at most 25 MB")
	}
	_, extension, err := referenceType(input)
	if err != nil {
		return empty, err
	}
	fields := map[string]string{"model": spec.Upstream, "response_format": "verbose_json"}
	if language, _ := body["language"].(string); language != "" {
		fields["language"] = language
	}
	payload, contentType, err := multipartFile(input.Data, extension, fields)
	if err != nil {
		return empty, err
	}
	raw, err := p.request(ctx, http.MethodPost, spec.Endpoint, key, contentType, payload)
	if err != nil {
		return empty, err
	}
	var response struct {
		Text     string  `json:"text"`
		Duration float64 `json:"duration"`
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return empty, errors.New("invalid APIMart transcription response")
	}
	millis := func(seconds float64) int64 {
		return int64(math.Round(seconds * float64(time.Second/time.Millisecond)))
	}
	out := aigc.Transcription{
		SchemaVersion: 1, Text: response.Text, DurationMS: millis(response.Duration),
		Granularity: "segment", Segments: []aigc.TranscriptionUnit{}, Words: []aigc.TranscriptionUnit{},
	}
	for _, segment := range response.Segments {
		if strings.TrimSpace(segment.Text) == "" {
			continue
		}
		out.Segments = append(out.Segments, aigc.TranscriptionUnit{
			StartMS: millis(segment.Start), EndMS: min(out.DurationMS, millis(segment.End)), Text: segment.Text,
		})
	}
	if err := out.Validate(spec.Model.Transcription.MaxDurationMS); err != nil {
		return empty, err
	}
	return aigc.Result{Transcription: &out}, nil
}
