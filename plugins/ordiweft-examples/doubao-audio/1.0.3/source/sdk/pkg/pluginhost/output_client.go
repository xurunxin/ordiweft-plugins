package pluginhost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// StageOutput freezes bytes and stages a private candidate using stable block
// IDs. It grants no canvas write, fee submission, retry or replacement output.
func StageOutput(ctx context.Context, host *Client, prefix string, data []byte, maxBytes int64) (AssetHandle, error) {
	var empty AssetHandle
	if host == nil || !opaqueID.MatchString(prefix) || len(prefix) > 90 || maxBytes < 1 || maxBytes > MaxAssetDecodeBytes || len(data) == 0 || int64(len(data)) > maxBytes {
		return empty, ErrContract
	}
	frozen := bytes.Clone(data)
	h := sha256.Sum256(frozen)
	hash := hex.EncodeToString(h[:])
	var buffer struct {
		ID         string `json:"id"`
		Bytes      int64  `json:"bytes"`
		SHA256     string `json:"sha256"`
		NextOffset int64  `json:"nextOffset"`
	}
	if err := host.Call(ctx, prefix+"-begin", "outputs", "begin", map[string]any{"bytes": len(frozen), "sha256": hash}, &buffer); err != nil {
		return empty, err
	}
	if !opaqueID.MatchString(buffer.ID) || buffer.Bytes != int64(len(frozen)) || buffer.SHA256 != hash || buffer.NextOffset < 0 || buffer.NextOffset > int64(len(frozen)) {
		return empty, ErrContract
	}
	for offset := buffer.NextOffset; offset < int64(len(frozen)); {
		end := min(int64(len(frozen)), offset+MaxChunkBytes)
		var result struct {
			NextOffset int64 `json:"nextOffset"`
			EOF        *bool `json:"eof"`
		}
		if err := host.Call(ctx, prefix+"-write-"+strconv.FormatInt(offset, 10), "outputs", "write", map[string]any{"id": buffer.ID, "offset": offset, "data": frozen[offset:end]}, &result); err != nil {
			return empty, err
		}
		if result.NextOffset != end || result.EOF == nil || *result.EOF != (end == int64(len(frozen))) {
			return empty, ErrContract
		}
		offset = end
	}
	var staged struct {
		Output AssetHandle `json:"output"`
	}
	if err := host.Call(ctx, prefix+"-stage", "outputs", "stage", map[string]string{"id": buffer.ID}, &staged); err != nil {
		return empty, err
	}
	if staged.Output.Validate() != nil || staged.Output.Bytes != int64(len(frozen)) || staged.Output.SHA256 != hash {
		return empty, ErrContract
	}
	return staged.Output, nil
}
