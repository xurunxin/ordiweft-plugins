package pluginhost

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxWorkspaceFiles = 64
const MaxWorkspaceFileBytes = 64_000
const MaxWorkspaceBytes = 1 << 20
const MaxWorkspaceEncodedBytes = 7 << 20 // JSON escaping plus bounded file names.

// WorkspaceSource is text, never an instruction to read a host path or run a
// build. The original release and all file bytes form the candidate identity.
type WorkspaceSource struct {
	BaseDigest string            `json:"baseDigest"`
	Files      map[string]string `json:"files"`
}

func WorkspacePath(name string) bool {
	if name != "release.json" && !ValidPath(name) {
		return false
	}
	if name != "LICENSE" && name != "NOTICE" && !slices.Contains([]string{".mjs", ".ts", ".json", ".md", ".txt"}, path.Ext(name)) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || slices.Contains([]string{"node_modules", "deploy", "data", "accounts", "credentials", "secrets"}, strings.ToLower(part)) {
			return false
		}
	}
	return true
}

func WorkspaceFileDigest(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

func (s WorkspaceSource) Validate() error {
	if !ValidDigest(s.BaseDigest) || len(s.Files) == 0 || len(s.Files) > MaxWorkspaceFiles {
		return ErrContract
	}
	if _, exists := s.Files["release.json"]; !exists {
		return ErrContract
	}
	seen, total := map[string]bool{}, 0
	for name, content := range s.Files {
		key := strings.ToLower(name)
		if !WorkspacePath(name) || seen[key] || !utf8.ValidString(content) || strings.ContainsRune(content, 0) || len(content) > MaxWorkspaceFileBytes {
			return ErrContract
		}
		seen[key] = true
		total += len(content)
	}
	if total > MaxWorkspaceBytes {
		return ErrContract
	}
	for name := range s.Files {
		parts := strings.Split(strings.ToLower(name), "/")
		for end := 1; end < len(parts); end++ {
			if seen[strings.Join(parts[:end], "/")] {
				return ErrContract
			}
		}
	}
	return nil
}

func (s WorkspaceSource) Digest() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	body, err := json.Marshal(s)
	if err != nil {
		return "", ErrContract
	}
	return WorkspaceFileDigest(string(body)), nil
}

// Archive recalculates the closure from frozen bytes. Candidate code cannot
// select another package/target, hide a file, inject a build hook or run here.
func (s WorkspaceSource) Archive(base Descriptor) ([]byte, error) {
	if _, exists := s.Files["build.json"]; exists && base.Runtime == "node" {
		return nil, ErrContract
	}
	return s.archive(base)
}

func (s WorkspaceSource) archive(base Descriptor) ([]byte, error) {
	baseDigest, err := base.Digest()
	if err != nil || s.Validate() != nil || s.BaseDigest != baseDigest {
		return nil, ErrContract
	}
	raw := []byte(s.Files["release.json"])
	if _, err = strictJSON(raw); err != nil {
		return nil, ErrContract
	}
	d, err := Decode(raw)
	if err != nil || d.PackageID != base.PackageID || d.Runtime != base.Runtime || !slices.Contains([]string{"data", "node"}, d.Runtime) {
		return nil, ErrContract
	}
	a, _ := json.Marshal(d.Contributions)
	b, _ := json.Marshal(base.Contributions)
	if !bytes.Equal(a, b) || (d.Generation == nil) != (base.Generation == nil) || d.Generation != nil && d.Generation.ID != base.Generation.ID {
		return nil, ErrContract
	}
	names := make([]string, 0, len(s.Files)-1)
	for name := range s.Files {
		if name != "release.json" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	d.Files = make([]File, 0, len(names))
	for _, name := range names {
		content := s.Files[name]
		d.Files = append(d.Files, File{Path: name, Bytes: int64(len(content)), SHA256: WorkspaceFileDigest(content)})
	}
	if err = d.Validate(); err != nil {
		return nil, err
	}
	manifest, err := json.Marshal(d)
	if err != nil {
		return nil, ErrContract
	}
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	for _, name := range append([]string{"release.json"}, names...) {
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		content := []byte(s.Files[name])
		if name == "release.json" {
			content = manifest
		}
		if _, err = f.Write(content); err != nil {
			return nil, err
		}
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
