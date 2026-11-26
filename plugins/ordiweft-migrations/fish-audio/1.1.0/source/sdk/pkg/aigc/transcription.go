package aigc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Transcription times are source-relative milliseconds, never timeline ticks
// or inferred phoneme/viseme boundaries. Speaker labels are local to a result.
type TranscriptionUnit struct {
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Text    string `json:"text"`
	Speaker string `json:"speaker,omitempty"`
}

type Transcription struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Text          string              `json:"text"`
	DurationMS    int64               `json:"durationMs"`
	Granularity   string              `json:"granularity"`
	Segments      []TranscriptionUnit `json:"segments"`
	Words         []TranscriptionUnit `json:"words"`
	RequestID     string              `json:"requestId,omitempty"`
}

type TranscriptionSpec struct {
	MaxDurationMS      int64    `json:"maxDurationMs"`
	MaxBytes           int64    `json:"maxBytes"`
	WordTimestamps     bool     `json:"wordTimestamps"`
	SpeakerDiarization bool     `json:"speakerDiarization"`
	Languages          []string `json:"languages"`
}

var ErrTranscription = errors.New("invalid normalized transcription")

func DecodeTranscription(body []byte, maxDurationMS int64) (Transcription, error) {
	var result Transcription
	if len(body) == 0 || len(body) > 4<<20 || !utf8.Valid(body) {
		return result, ErrTranscription
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Validate(maxDurationMS) != nil {
		return Transcription{}, ErrTranscription
	}
	return result, nil
}

func (t Transcription) Validate(maxDurationMS int64) error {
	if t.SchemaVersion != 1 || t.DurationMS <= 0 || t.DurationMS > maxDurationMS || maxDurationMS > 3*60*60*1000 || len(t.Text) > 200000 || !utf8.ValidString(t.Text) || !utf8.ValidString(t.RequestID) || len(t.RequestID) > 256 || strings.ContainsAny(t.RequestID, "\x00\r\n") || t.Granularity != "word" && t.Granularity != "character" && t.Granularity != "segment" || t.Segments == nil || t.Words == nil || len(t.Segments) > 10000 || len(t.Words) > 20000 {
		return ErrTranscription
	}
	for _, units := range [][]TranscriptionUnit{t.Segments, t.Words} {
		last := int64(-1)
		for _, u := range units {
			if u.StartMS < 0 || u.StartMS < last || u.EndMS <= u.StartMS || u.EndMS > t.DurationMS || strings.TrimSpace(u.Text) == "" || !utf8.ValidString(u.Text) || len(u.Text) > 4000 || !utf8.ValidString(u.Speaker) || len(u.Speaker) > 128 || strings.ContainsAny(u.Speaker, "\x00\r\n") {
				return ErrTranscription
			}
			last = u.StartMS
		}
	}
	if t.Granularity == "segment" && len(t.Words) > 0 {
		return ErrTranscription
	}
	var text strings.Builder
	for _, u := range t.Segments {
		text.WriteString(u.Text)
	}
	if strings.TrimSpace(text.String()) != strings.TrimSpace(t.Text) {
		return ErrTranscription
	}
	if len(t.Words) > 0 {
		text.Reset()
		for _, u := range t.Words {
			text.WriteString(u.Text)
		}
		// Providers can insert whitespace between English words. Preserve original
		// text while checking that timed units do not substitute recognized words.
		compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
		if compact(text.String()) != compact(t.Text) {
			return ErrTranscription
		}
	}
	return nil
}

func (s TranscriptionSpec) Validate() error {
	if s.MaxDurationMS < 1 || s.MaxDurationMS > 3*60*60*1000 || s.MaxBytes < 1 || s.MaxBytes > 64<<20 || len(s.Languages) > 100 {
		return ErrTranscription
	}
	seen := map[string]bool{}
	for _, l := range s.Languages {
		if l == "" || len(l) > 35 || seen[l] {
			return ErrTranscription
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return ErrTranscription
			}
		}
		seen[l] = true
	}
	return nil
}
