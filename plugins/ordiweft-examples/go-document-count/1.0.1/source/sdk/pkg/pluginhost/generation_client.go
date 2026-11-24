package pluginhost

import (
	"encoding/json"
	"io"
)

// GenerationSession owns one original host-services generation channel.
// The host remains responsible for task authority and reader lifetime.
type GenerationSession struct {
	Start   TaskStart
	Host    *Client
	channel *privateClientChannel
}

func OpenGeneration(reader io.Reader, writer io.Writer) (*GenerationSession, error) {
	channel, body, err := openPrivateClientChannel(reader, writer)
	if err != nil {
		return nil, err
	}
	start, err := DecodeTaskStart(body)
	if err != nil {
		return nil, err
	}
	return &GenerationSession{Start: start, Host: channel.client(), channel: channel}, nil
}

func (s *GenerationSession) Complete(outputs []string, metadata any) error {
	if s == nil {
		return ErrContract
	}
	if outputs == nil {
		outputs = []string{}
	}
	var body json.RawMessage
	if metadata != nil {
		var err error
		body, err = json.Marshal(metadata)
		if err != nil {
			return ErrContract
		}
	}
	completion := TaskCompletion{HostProtocol: Version, Kind: "complete", Outputs: outputs, Metadata: body}
	wire, err := json.Marshal(completion)
	if err != nil {
		return ErrContract
	}
	if _, err = strictJSON(wire); err != nil {
		return err
	}
	if _, err = DecodeTaskCompletion(wire); err != nil {
		return err
	}
	return s.channel.complete(completion)
}
