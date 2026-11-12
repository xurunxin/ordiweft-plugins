package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
)

// ReadAsset is an explicitly bounded in-memory helper. It reads the original
// handle with stable call IDs, validates contiguous canonical chunks and SHA,
// and never retries or selects another asset after a missing reply.
func ReadAsset(ctx context.Context, host *Client, prefix string, asset AssetHandle, maxBytes int64) ([]byte, error) {
	if host == nil || !opaqueID.MatchString(prefix) || len(prefix) > 90 || asset.Validate() != nil || maxBytes < 1 || maxBytes > MaxAssetSourceBytes || asset.Bytes > maxBytes {
		return nil, ErrContract
	}
	data := make([]byte, 0, int(asset.Bytes))
	for offset := int64(0); offset < asset.Bytes; {
		limit := min(int64(MaxChunkBytes), asset.Bytes-offset)
		var reply struct {
			Data       string `json:"data"`
			NextOffset int64  `json:"nextOffset"`
			EOF        *bool  `json:"eof"`
		}
		if err := host.Call(ctx, prefix+"-read-"+strconv.FormatInt(offset, 10), "assets", "read", struct {
			ID     string `json:"id"`
			Offset int64  `json:"offset"`
			Limit  int64  `json:"limit"`
		}{asset.ID, offset, limit}, &reply); err != nil {
			return nil, err
		}
		if len(reply.Data) > base64.StdEncoding.EncodedLen(int(MaxChunkBytes)) {
			return nil, ErrContract
		}
		chunk, err := base64.StdEncoding.Strict().DecodeString(reply.Data)
		if err != nil || len(chunk) == 0 || int64(len(chunk)) > limit || base64.StdEncoding.EncodeToString(chunk) != reply.Data || reply.NextOffset != offset+int64(len(chunk)) || reply.NextOffset > asset.Bytes || reply.EOF == nil || *reply.EOF != (reply.NextOffset == asset.Bytes) {
			return nil, ErrContract
		}
		data = append(data, chunk...)
		offset = reply.NextOffset
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != asset.SHA256 {
		return nil, ErrContract
	}
	return data, nil
}
