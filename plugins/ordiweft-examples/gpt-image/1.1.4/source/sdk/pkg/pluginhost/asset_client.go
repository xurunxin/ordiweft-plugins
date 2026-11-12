package pluginhost

import (
	"bytes"
	"context"
	"io"
)

// ReadAsset is an explicitly bounded in-memory helper. It reads the original
// handle with stable call IDs, validates contiguous canonical chunks and SHA,
// and never retries or selects another asset after a missing reply.
func ReadAsset(ctx context.Context, host *Client, prefix string, asset AssetHandle, maxBytes int64) ([]byte, error) {
	if maxBytes < 1 || maxBytes > MaxAssetSourceBytes {
		return nil, ErrContract
	}
	reader, err := OpenAsset(ctx, host, prefix, asset, maxBytes)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var result bytes.Buffer
	result.Grow(int(asset.Bytes))
	if _, err = io.Copy(&result, reader); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
