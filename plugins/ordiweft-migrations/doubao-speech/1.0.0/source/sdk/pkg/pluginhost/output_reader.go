package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// StageOutputReader stages only after size, EOF and SHA confirm the entire
// source. It retains one chunk; a failed private buffer is never retried/reset.
func StageOutputReader(ctx context.Context, host *Client, prefix string, source io.Reader, size int64, digest string, maxBytes int64) (AssetHandle, error) {
	return stageOutputReader(ctx, host, prefix, source, size, digest, maxBytes, "stage")
}

// StageOutputReaderAndReleaseBuffer requires outputs.stage-buffer. It moves
// only the completed private buffer, never the reader or an original input.
func StageOutputReaderAndReleaseBuffer(ctx context.Context, host *Client, prefix string, source io.Reader, size int64, digest string, maxBytes int64) (AssetHandle, error) {
	return stageOutputReader(ctx, host, prefix, source, size, digest, maxBytes, "stage-buffer")
}

func stageOutputReader(ctx context.Context, host *Client, prefix string, source io.Reader, size int64, digest string, maxBytes int64, operation string) (AssetHandle, error) {
	var empty AssetHandle
	if host == nil || source == nil || !opaqueID.MatchString(prefix) || len(prefix) > 90 || !ValidDigest(digest) || maxBytes < 1 || maxBytes > MaxAssetDecodeBytes || size < 1 || size > maxBytes {
		return empty, ErrContract
	}
	var buffer struct {
		ID         string `json:"id"`
		Bytes      int64  `json:"bytes"`
		SHA256     string `json:"sha256"`
		NextOffset int64  `json:"nextOffset"`
	}
	if err := host.Call(ctx, prefix+"-begin", "outputs", "begin", map[string]any{"bytes": size, "sha256": digest}, &buffer); err != nil {
		return empty, err
	}
	if !opaqueID.MatchString(buffer.ID) || buffer.Bytes != size || buffer.SHA256 != digest || buffer.NextOffset < 0 || buffer.NextOffset > size {
		return empty, ErrContract
	}
	hash := sha256.New()
	chunk := make([]byte, MaxChunkBytes)
	sourceEOF := false
	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		end := min(size, offset+MaxChunkBytes)
		part := chunk[:end-offset]
		var err error
		sourceEOF, err = readOutputChunk(ctx, source, part)
		if err != nil {
			return empty, err
		}
		if sourceEOF && end != size {
			return empty, io.ErrUnexpectedEOF
		}
		_, _ = hash.Write(part)
		if end > buffer.NextOffset {
			start := max(offset, buffer.NextOffset)
			var result struct {
				NextOffset int64 `json:"nextOffset"`
				EOF        *bool `json:"eof"`
			}
			if err := host.Call(ctx, prefix+"-write-"+strconv.FormatInt(start, 10), "outputs", "write", map[string]any{"id": buffer.ID, "offset": start, "data": part[start-offset:]}, &result); err != nil {
				return empty, err
			}
			if result.NextOffset != end || result.EOF == nil || *result.EOF != (end == size) {
				return empty, ErrContract
			}
		}
		offset = end
	}
	var extra [1]byte
	if !sourceEOF {
		if n, err := io.ReadFull(source, extra[:]); n != 0 || err != io.EOF {
			return empty, ErrContract
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return empty, ErrContract
	}
	var staged struct {
		Output AssetHandle `json:"output"`
	}
	if err := host.Call(ctx, prefix+"-stage", "outputs", operation, map[string]string{"id": buffer.ID}, &staged); err != nil {
		return empty, err
	}
	if staged.Output.Validate() != nil || staged.Output.Bytes != size || staged.Output.SHA256 != digest {
		return empty, ErrContract
	}
	return staged.Output, nil
}

// io.ReadFull discards an error accompanying the final requested bytes. Keep
// that error: a matching byte count alone cannot confirm a failed source read.
func readOutputChunk(ctx context.Context, source io.Reader, part []byte) (bool, error) {
	filled, emptyReads := 0, 0
	for filled < len(part) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, err := source.Read(part[filled:])
		if n < 0 || n > len(part)-filled {
			return false, ErrContract
		}
		filled += n
		if err != nil {
			if err == io.EOF && filled == len(part) {
				return true, nil
			}
			if err == io.EOF {
				return false, io.ErrUnexpectedEOF
			}
			return false, err
		}
		if n == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return false, io.ErrNoProgress
			}
		} else {
			emptyReads = 0
		}
	}
	return false, nil
}
