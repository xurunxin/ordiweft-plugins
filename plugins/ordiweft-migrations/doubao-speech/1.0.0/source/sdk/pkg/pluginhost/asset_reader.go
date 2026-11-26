package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"hash"
	"io"
	"strconv"
)

const MaxAssetReadBytes = 256 << 20

// AssetReader keeps at most one original RPC chunk. Bytes remain unconfirmed
// until EOF validates the whole handle; a terminal error never repeats a read.
type AssetReader struct {
	host             *Client
	ctx              context.Context
	prefix           string
	asset            AssetHandle
	offset           int64
	buffer           []byte
	hash             hash.Hash
	err              error
	verified, closed bool
}

func OpenAsset(ctx context.Context, host *Client, prefix string, asset AssetHandle, maxBytes int64) (*AssetReader, error) {
	if host == nil || !opaqueID.MatchString(prefix) || len(prefix) > 90 || asset.Validate() != nil || maxBytes < 1 || maxBytes > MaxAssetReadBytes || asset.Bytes > maxBytes {
		return nil, ErrContract
	}
	return &AssetReader{host: host, ctx: ctx, prefix: prefix, asset: asset, hash: sha256.New()}, nil
}

func (r *AssetReader) Verified() bool { return r.verified }

// Close is local. The caller retains authority over the original handle.
func (r *AssetReader) Close() error {
	r.closed = true
	r.buffer = nil
	return nil
}

func (r *AssetReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if r.err != nil {
		return 0, r.err
	}
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return 0, err
	}
	if len(r.buffer) == 0 && r.offset < r.asset.Bytes {
		limit := min(int64(MaxChunkBytes), r.asset.Bytes-r.offset)
		var reply struct {
			Data       string `json:"data"`
			NextOffset int64  `json:"nextOffset"`
			EOF        *bool  `json:"eof"`
		}
		err := r.host.Call(r.ctx, r.prefix+"-read-"+strconv.FormatInt(r.offset, 10), "assets", "read", struct {
			ID     string `json:"id"`
			Offset int64  `json:"offset"`
			Limit  int64  `json:"limit"`
		}{r.asset.ID, r.offset, limit}, &reply)
		if err != nil {
			r.err = err
			return 0, err
		}
		if len(reply.Data) > base64.StdEncoding.EncodedLen(int(MaxChunkBytes)) {
			r.err = ErrContract
			return 0, r.err
		}
		chunk, err := base64.StdEncoding.Strict().DecodeString(reply.Data)
		if err != nil || len(chunk) == 0 || int64(len(chunk)) > limit || base64.StdEncoding.EncodeToString(chunk) != reply.Data || reply.NextOffset != r.offset+int64(len(chunk)) || reply.NextOffset > r.asset.Bytes || reply.EOF == nil || *reply.EOF != (reply.NextOffset == r.asset.Bytes) {
			r.err = ErrContract
			return 0, r.err
		}
		r.offset = reply.NextOffset
		r.buffer = chunk
		_, _ = r.hash.Write(chunk)
	}
	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	if len(r.buffer) == 0 && r.offset == r.asset.Bytes {
		if hex.EncodeToString(r.hash.Sum(nil)) != r.asset.SHA256 {
			r.err = ErrContract
			return n, r.err
		}
		r.verified = true
		r.err = io.EOF
		return n, io.EOF
	}
	return n, nil
}
