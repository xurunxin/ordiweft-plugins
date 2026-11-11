package pluginhost

// MediaInfo reports facts parsed from original bytes. Inspection does not
// adopt a business output or grant access to another asset with the same SHA.
type MediaInfo struct {
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
	Mime       string `json:"mime"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
}

type MediaDerivation struct {
	Version   int         `json:"version"`
	Operation string      `json:"operation"`
	Source    AssetHandle `json:"source"`
	Result    AssetHandle `json:"result"`
}

func (d MediaDerivation) Validate() error {
	if d.Version != 1 || d.Operation != "audio.normalize-wav" || d.Source.Validate() != nil || d.Result.Validate() != nil || d.Source.Bytes < 1 || d.Source.Bytes > 64<<20 || d.Result.Bytes != d.Source.Bytes || d.Result.Mime != "audio/wav" {
		return ErrContract
	}
	return nil
}
