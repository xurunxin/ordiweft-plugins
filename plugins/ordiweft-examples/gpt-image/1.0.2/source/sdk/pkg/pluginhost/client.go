package pluginhost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
)

// Reply is a response on the private original-task channel. It carries no new
// identity or grants. An invalid/missing reply never licenses a repeat call.
type Reply struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Failure        `json:"error,omitempty"`
}

func DecodeReply(body []byte, id string) (Reply, error) {
	var r Reply
	if len(body) == 0 || len(body) > MaxControlBytes {
		return r, ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Version != Version || !opaqueID.MatchString(r.ID) || r.ID != id {
		return r, ErrContract
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return r, ErrContract
	}
	_, hasError := fields["error"]
	_, hasResult := fields["result"]
	if hasError == hasResult || hasError && r.Error == nil {
		return r, ErrContract
	}
	if r.Error != nil {
		if r.Error.Validate() != nil || len(r.Result) != 0 {
			return r, ErrContract
		}
	} else if len(r.Result) == 0 || r.Result[0] != '{' || !json.Valid(r.Result) {
		return r, ErrContract
	}
	return r, nil
}

type HostError struct{ Failure Failure }

func (e *HostError) Error() string { return "plugin_host_" + e.Failure.Code }

// Exchange is supplied by the runtime's private task channel. The SDK has no
// URL, credentials, reconnect-submit policy, or implicit call-ID generator.
type Exchange func(context.Context, Call) ([]byte, error)
type Client struct{ exchange Exchange }

func NewClient(exchange Exchange) *Client { return &Client{exchange: exchange} }

func (c *Client) Call(ctx context.Context, id, service, operation string, input, output any) error {
	if c == nil || c.exchange == nil {
		return ErrContract
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return ErrContract
	}
	call := Call{Version: Version, ID: id, Service: service, Operation: operation, Input: data}
	wire, err := json.Marshal(call)
	if err != nil {
		return ErrContract
	}
	call, err = DecodeCall(wire)
	if err != nil {
		return err
	}
	body, err := c.exchange(ctx, call)
	if err != nil {
		return err
	}
	reply, err := DecodeReply(body, id)
	if err != nil {
		return err
	}
	if reply.Error != nil {
		return &HostError{Failure: *reply.Error}
	}
	if output != nil && json.Unmarshal(reply.Result, output) != nil {
		return ErrContract
	}
	return nil
}

func (c *Client) ReportUsage(ctx context.Context, id string, meters []Meter) error {
	if len(meters) == 0 || len(meters) > 64 {
		return ErrContract
	}
	for _, m := range meters {
		if m.ValidateReport() != nil {
			return ErrContract
		}
	}
	return c.Call(ctx, id, "usage", "report", struct {
		Meters []Meter `json:"meters"`
	}{meters}, nil)
}

func (c *Client) ReportDiagnostic(ctx context.Context, id string, f Failure) error {
	if f.Validate() != nil {
		return ErrContract
	}
	return c.Call(ctx, id, "diagnostics", "report", struct {
		Failure Failure `json:"failure"`
	}{f}, nil)
}
