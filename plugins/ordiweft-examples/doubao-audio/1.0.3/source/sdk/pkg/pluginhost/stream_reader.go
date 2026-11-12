package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
)

// StreamReader exposes the original bounded connection as an io.Reader. A
// lost/malformed reply is terminal; Read never reconnects or repeats a submit.
type StreamReader struct {
	client   *Client
	ctx      context.Context
	id       string
	sequence int64
	buffer   []byte
	status   int
	err      error
	closed   bool
}

func (c *Client) OpenStream(ctx context.Context, id, operation string) (*StreamReader, error) {
	var opened struct {
		StreamID string `json:"streamId"`
	}
	if err := c.Call(ctx, id, "streams", "open", map[string]string{"operation": operation}, &opened); err != nil {
		return nil, err
	}
	if opened.StreamID != id {
		return nil, ErrContract
	}
	return &StreamReader{client: c, ctx: ctx, id: id}, nil
}

func (r *StreamReader) Status() int { return r.status }

func (r *StreamReader) callID(operation string) string {
	h := sha256.Sum256([]byte(r.id + ":" + operation))
	return "stream_" + hex.EncodeToString(h[:])
}

func (r *StreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(r.buffer) == 0 && r.err == nil {
		var reply StreamReply
		if err := r.client.Call(r.ctx, r.callID("next:"+strconv.FormatInt(r.sequence, 10)), "streams", "next", StreamRead{StreamID: r.id, Sequence: r.sequence}, &reply); err != nil {
			r.err = err
		} else if reply.Frame.Validate() != nil || reply.Frame.StreamID != r.id || reply.Frame.Sequence != r.sequence || reply.Status < 100 || reply.Status > 599 || r.status != 0 && reply.Status != r.status || reply.Frame.Kind != "chunk" && reply.Frame.Kind != "end" || reply.Frame.Kind == "chunk" && len(reply.Frame.Data) == 0 {
			r.err = ErrContract
		} else {
			r.status = reply.Status
			r.sequence++
			r.buffer = reply.Frame.Data
			if reply.Frame.Kind == "end" {
				r.err = io.EOF
			}
		}
	}
	if len(r.buffer) != 0 {
		n := copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		return n, nil
	}
	return 0, r.err
}

func (r *StreamReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	r.buffer = nil
	if r.err != nil && r.err != io.EOF {
		return r.err
	}
	return r.client.Call(r.ctx, r.callID("close"), "streams", "close", map[string]string{"streamId": r.id}, nil)
}
