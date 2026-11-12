package pluginhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
)

// ServeChannel owns a private, serial NDJSON task pipe. The trusted caller binds
// exchange to the original task's installed services; the pipe grants nothing.
// A channel terminates on uncertainty rather than reconnecting or retrying.
func ServeChannel(ctx context.Context, pipe io.ReadWriteCloser, exchange Exchange) error {
	if pipe == nil || exchange == nil {
		return ErrContract
	}
	defer pipe.Close()
	stop := context.AfterFunc(ctx, func() { _ = pipe.Close() })
	defer stop()
	reader := bufio.NewScanner(pipe)
	reader.Buffer(make([]byte, 4096), MaxControlBytes+2)
	for reader.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		call, err := DecodeCall(reader.Bytes())
		if err != nil {
			return err
		}
		body, err := exchange(ctx, call)
		if err != nil {
			// Arbitrary internal errors can contain URLs/keys. The exchange must
			// explicitly encode a safe Failure to deliver one over this channel.
			return err
		}
		if _, err = DecodeReply(body, call.ID); err != nil {
			return err
		}
		// Normalize any valid whitespace/newline in a supplied JSON reply to a
		// single control frame; pretty JSON cannot inject another pipe message.
		var compact bytes.Buffer
		if json.Compact(&compact, body) != nil {
			return ErrContract
		}
		body = compact.Bytes()
		written, err := pipe.Write(append(body, '\n'))
		if err != nil {
			return err
		}
		if written != len(body)+1 {
			return io.ErrShortWrite
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return reader.Err()
}
