// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSyntheticSpeechFixture(t *testing.T) {
	raw, err := os.ReadFile("fixtures/verification.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Exchanges []struct {
			Body []byte `json:"body"`
		} `json:"exchanges"`
		OutputSHA256 string `json:"outputSha256"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || len(fixture.Exchanges) != 1 {
		t.Fatal("invalid fixture", err)
	}
	audio, words, err := readSpeech(bytes.NewReader(fixture.Exchanges[0].Body))
	sum := sha256.Sum256(audio)
	if err != nil || words != 3 || hex.EncodeToString(sum[:]) != fixture.OutputSHA256 {
		t.Fatal("audio bytes or usage differ from fixture", words, err)
	}
}

func TestStreamRequiresOneCompleteResponse(t *testing.T) {
	for _, tc := range []struct{ name, suffix string }{
		{"EOF before confirmation", ""},
		{"truncated JSON", "{\"code\":"},
		{"supplier rejected", "{\"code\":45000000}"},
		{"confirmation carries audio", "{\"code\":20000000,\"data\":\"Yg==\"}"},
		{"bytes after confirmation", "{\"code\":20000000}\n{\"code\":0,\"data\":\"Yg==\"}"},
		{"invalid audio bytes", "{\"code\":0,\"data\":\"!\"}\n{\"code\":20000000}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "{\"code\":0,\"data\":\"YQ==\"}\n" + tc.suffix
			audio, words, err := readSpeech(strings.NewReader(text))
			if !errors.Is(err, errUnconfirmed) || len(audio) != 0 || words != 0 {
				t.Fatal("incomplete response yielded a result", err)
			}
		})
	}
	good := strings.NewReader("{\"code\":0,\"data\":\"YQ==\"}\n{\"code\":20000000}\n")
	source := io.MultiReader(good, failedSource{})
	if audio, _, err := readSpeech(source); err == nil || len(audio) != 0 {
		t.Fatal("final confirmation without successful stream EOF yielded audio")
	}
}

type failedSource struct{}

func (failedSource) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
