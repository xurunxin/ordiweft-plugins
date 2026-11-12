package pluginhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
)

// The runtime owns reader shutdown. A generic io.Reader cannot be interrupted
// by context; uncertainty permanently closes this original session.
type privateClientChannel struct {
	mu      sync.Mutex
	scanner *bufio.Scanner
	writer  io.Writer
	closed  bool
}

func openPrivateClientChannel(reader io.Reader, writer io.Writer) (*privateClientChannel, []byte, error) {
	if reader == nil || writer == nil {
		return nil, nil, ErrContract
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), MaxControlBytes+1)
	if !scanner.Scan() {
		return nil, nil, ErrContract
	}
	return &privateClientChannel{scanner: scanner, writer: writer}, bytes.Clone(scanner.Bytes()), nil
}

func (c *privateClientChannel) client() *Client {
	return NewClient(func(ctx context.Context, call Call) ([]byte, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return nil, ErrContract
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.closed = true
		if err := c.write(call); err != nil {
			return nil, err
		}
		if !c.scanner.Scan() {
			return nil, ErrContract
		}
		reply := bytes.Clone(c.scanner.Bytes())
		if _, err := DecodeReply(reply, call.ID); err != nil {
			return nil, err
		}
		c.closed = false
		return reply, nil
	})
}

func (c *privateClientChannel) write(value any) error {
	body, err := json.Marshal(value)
	if err != nil || len(body) > MaxControlBytes {
		return ErrContract
	}
	body = append(body, '\n')
	written, err := c.writer.Write(body)
	if err != nil {
		return err
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	return nil
}

func (c *privateClientChannel) complete(value any) error {
	if c == nil {
		return ErrContract
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrContract
	}
	c.closed = true
	return c.write(value)
}
