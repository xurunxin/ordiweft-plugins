package pluginhost

// StreamRead acknowledges only the preceding chunk of the original stream.
// Repeating the immediately preceding sequence reads the same observation.
type StreamRead struct {
	StreamID string `json:"streamId"`
	Sequence int64  `json:"sequence"`
}

type StreamReply struct {
	Status int   `json:"status"`
	Frame  Frame `json:"frame"`
}
