// SPDX-License-Identifier: Apache-2.0
// npm-pack reproduces the archive of a reviewed portable release. npm-release.mjs
// then requires its exact pinned ZIP length and SHA-256 before staging.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
)

func run(source, destination string) (err error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	manifest, err := root.ReadFile("release.json")
	if err != nil {
		return err
	}
	var descriptor struct {
		Files []struct {
			Path   string
			Bytes  int
			SHA256 string
		}
	}
	if err = json.Unmarshal(manifest, &descriptor); err != nil {
		return err
	}
	if len(descriptor.Files) == 0 {
		return errors.New("empty release manifest")
	}
	sort.Slice(descriptor.Files, func(i, j int) bool { return descriptor.Files[i].Path < descriptor.Files[j].Path })
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, out.Close())
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	archive := zip.NewWriter(out)
	write := func(path string, data []byte) error {
		entry, err := archive.Create(path)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	}
	if err = write("release.json", manifest); err != nil {
		return err
	}
	previous := "release.json"
	for _, file := range descriptor.Files {
		if file.Path == "release.json" || !fs.ValidPath(file.Path) || file.Path == "." || file.Path == previous {
			return fmt.Errorf("invalid entry %q", file.Path)
		}
		previous = file.Path
		info, err := root.Lstat(file.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != int64(file.Bytes) {
			return fmt.Errorf("invalid file %q", file.Path)
		}
		body, err := root.ReadFile(file.Path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(body)
		if hex.EncodeToString(hash[:]) != file.SHA256 {
			return fmt.Errorf("digest differs for %q", file.Path)
		}
		if err = write(file.Path, body); err != nil {
			return err
		}
	}
	return archive.Close()
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: go run scripts/npm-pack.go <reviewed bundle> <new ZIP>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
