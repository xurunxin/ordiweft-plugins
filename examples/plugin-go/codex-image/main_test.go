package main

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestOriginalResponsePreservesReportedMetadataAndRejectsExtraOutput(t *testing.T) {
	metadata := map[string]any{}
	var actual string
	usage, err := readImages(strings.NewReader(`{"model":"reported-model","size":"1254x1254","quality":"medium","background":"opaque","data":[{"b64_json":"b3JpZ2luYWw="}],"usage":{"images":1}}`), 1, func(_ int, image *decodedImage) error {
		data, e := io.ReadAll(image.file)
		actual = string(data)
		return e
	}, metadata)
	if err != nil || actual != "original" || metadata["model"] != "reported-model" || metadata["size"] != "1254x1254" || metadata["quality"] != "medium" {
		t.Fatal("original metadata/bytes lost", err, metadata)
	}
	var values map[string]int
	if json.Unmarshal(usage, &values) != nil || values["images"] != 1 {
		t.Fatal("usage lost")
	}
	_, err = readImages(strings.NewReader(`{"data":[{"b64_json":"YQ=="},{"b64_json":"Yg=="}]}`), 1, func(_ int, _ *decodedImage) error { return nil }, map[string]any{})
	if err == nil {
		t.Fatal("unexpected extra output accepted")
	}
}
