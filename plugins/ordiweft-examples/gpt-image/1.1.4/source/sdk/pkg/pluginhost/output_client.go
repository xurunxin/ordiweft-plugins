package pluginhost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// StageOutput freezes bytes and stages a private candidate using stable block
// IDs. It grants no canvas write, fee submission, retry or replacement output.
func StageOutput(ctx context.Context, host *Client, prefix string, data []byte, maxBytes int64) (AssetHandle, error) {
	var empty AssetHandle
	if host == nil || !opaqueID.MatchString(prefix) || len(prefix) > 90 || maxBytes < 1 || maxBytes > MaxAssetDecodeBytes || len(data) == 0 || int64(len(data)) > maxBytes {
		return empty, ErrContract
	}
	frozen := bytes.Clone(data)
	sum := sha256.Sum256(frozen)
	return StageOutputReader(ctx, host, prefix, bytes.NewReader(frozen), int64(len(frozen)), hex.EncodeToString(sum[:]), maxBytes)
}
