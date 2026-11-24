package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// VerifyDirectory checks an unpacked bundle without executing it. The caller
// still owns trust verification, immutable installation and sandbox admission.
func VerifyDirectory(root string, d Descriptor) error {
	if err := d.Validate(); err != nil {
		return err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return ErrContract
	}
	real, err := filepath.EvalSymlinks(abs)
	matchingRoot := filepath.Clean(abs) == filepath.Clean(real)
	if runtime.GOOS == "windows" {
		matchingRoot = strings.EqualFold(filepath.Clean(abs), filepath.Clean(real))
	}
	if err != nil || !matchingRoot {
		return fmt.Errorf("%w: linked bundle root", ErrContract)
	}
	expected := make(map[string]File, len(d.Files))
	for _, f := range d.Files {
		expected[f.Path] = f
	}
	seen := 0
	definitionNames := map[string]bool{}
	for _, name := range d.Definitions {
		definitionNames[name] = true
	}
	definitions := map[string][]byte{}
	err = filepath.WalkDir(abs, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: linked bundle member", ErrContract)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: non-regular bundle member", ErrContract)
		}
		rel, e := filepath.Rel(abs, name)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == "release.json" {
			return nil
		}
		f, ok := expected[rel]
		if !ok {
			return fmt.Errorf("%w: unlisted bundle member", ErrContract)
		}
		file, e := os.Open(name)
		if e != nil {
			return e
		}
		defer file.Close()
		stat, e := file.Stat()
		if e != nil || !stat.Mode().IsRegular() || stat.Size() != f.Bytes {
			return fmt.Errorf("%w: file size", ErrContract)
		}
		h := sha256.New()
		reader := io.LimitReader(file, f.Bytes+1)
		if definitionNames[rel] {
			data, err := io.ReadAll(reader)
			if err != nil {
				return err
			}
			definitions[rel] = data
			reader = bytes.NewReader(data)
		}
		count, e := io.Copy(h, reader)
		if e != nil || count != f.Bytes || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			return fmt.Errorf("%w: file digest", ErrContract)
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(expected) {
		return fmt.Errorf("%w: missing bundle member", ErrContract)
	}
	_, err = DecodeDefinitions(d, definitions)
	return err
}
