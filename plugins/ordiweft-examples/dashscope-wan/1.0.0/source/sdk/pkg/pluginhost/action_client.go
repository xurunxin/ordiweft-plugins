package pluginhost

import (
	"encoding/json"
	"io"
)

// ActionSession owns one original private channel. Its reader must be bounded
// by the runtime's lifetime; a generic io.Reader cannot be interrupted by ctx.
// Missing, partial or invalid replies close this session without reconnecting.
type ActionSession struct {
	Start   ActionStart
	Host    *Client
	channel *privateClientChannel
}

func OpenAction(reader io.Reader, writer io.Writer) (*ActionSession, error) {
	channel, body, err := openPrivateClientChannel(reader, writer)
	if err != nil {
		return nil, err
	}
	start, err := DecodeActionStart(body)
	if err != nil {
		return nil, err
	}
	return &ActionSession{Start: start, Host: channel.client(), channel: channel}, nil
}

func (s *ActionSession) Complete(outputs []string, value any) error {
	if s == nil {
		return ErrContract
	}
	if outputs == nil {
		outputs = []string{}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return ErrContract
	}
	completion := ActionCompletion{HostProtocol: Version, Kind: "action-complete", Outputs: outputs, Value: body}
	wire, err := json.Marshal(completion)
	if err != nil {
		return ErrContract
	}
	if _, err = DecodeActionCompletion(wire); err != nil {
		return err
	}
	return s.channel.complete(completion)
}
