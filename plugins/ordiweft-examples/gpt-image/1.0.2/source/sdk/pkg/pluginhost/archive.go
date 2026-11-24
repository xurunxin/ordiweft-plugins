package pluginhost

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

const MaxArchiveBytes = 32 << 20

type Archive struct {
	Descriptor Descriptor
	Metadata   []byte
	Files      map[string][]byte
}

// DecodeArchive verifies the complete ZIP inventory without extracting files,
// running code, granting permissions, or declaring a source trusted.
func DecodeArchive(body []byte) (Archive, error) {
	var out Archive
	if len(body) == 0 || len(body) > MaxArchiveBytes {
		return out, ErrContract
	}
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil || len(z.File) > 4097 {
		return out, ErrContract
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if !f.Mode().IsRegular() || f.Name != "release.json" && !ValidPath(f.Name) || entries[strings.ToLower(f.Name)] != nil || f.UncompressedSize64 > MaxArchiveBytes {
			return out, ErrContract
		}
		total += f.UncompressedSize64
		if total > MaxArchiveBytes {
			return out, ErrContract
		}
		entries[strings.ToLower(f.Name)] = f
	}
	meta := entries["release.json"]
	if meta == nil || meta.Name != "release.json" {
		return out, ErrContract
	}
	out.Metadata, err = archiveBytes(meta, MaxDescriptorBytes)
	if err != nil {
		return out, err
	}
	out.Descriptor, err = Decode(out.Metadata)
	if err != nil || len(entries) != len(out.Descriptor.Files)+1 {
		return out, ErrContract
	}
	out.Files = map[string][]byte{}
	for _, f := range out.Descriptor.Files {
		entry := entries[strings.ToLower(f.Path)]
		if entry == nil || entry.Name != f.Path || entry.UncompressedSize64 != uint64(f.Bytes) {
			return Archive{}, ErrContract
		}
		data, err := archiveBytes(entry, f.Bytes)
		if err != nil {
			return Archive{}, err
		}
		h := sha256.Sum256(data)
		if int64(len(data)) != f.Bytes || hex.EncodeToString(h[:]) != f.SHA256 {
			return Archive{}, ErrContract
		}
		out.Files[f.Path] = data
	}
	if _, err := DecodeDefinitions(out.Descriptor, out.Files); err != nil {
		return Archive{}, err
	}
	return out, nil
}

func archiveBytes(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, ErrContract
	}
	r, err := f.Open()
	if err != nil {
		return nil, ErrContract
	}
	defer r.Close()
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil || int64(len(body)) > max {
		return nil, ErrContract
	}
	return body, nil
}
