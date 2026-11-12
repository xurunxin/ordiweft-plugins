package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	host "gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

func run() error {
	task, err := host.OpenAction(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	if task.Start.PortID != "document-count" || len(task.Start.Inputs) != 1 {
		return host.ErrContract
	}
	input := task.Start.Inputs[0]
	if input.Kind != "document" || input.Role != "source" || input.Asset.Mime != "text/plain" {
		return host.ErrContract
	}
	data, err := host.ReadAsset(context.Background(), task.Host, "source", input.Asset, 1<<20)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) {
		return host.ErrContract
	}
	return task.Complete(nil, struct {
		Characters   int    `json:"characters"`
		Words        int    `json:"words"`
		SourceSHA256 string `json:"sourceSha256"`
	}{utf8.RuneCount(data), len(strings.Fields(string(data))), input.Asset.SHA256})
}

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "processor failed")
		os.Exit(1)
	}
}
