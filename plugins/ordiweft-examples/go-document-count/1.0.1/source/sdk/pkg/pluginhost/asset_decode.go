package pluginhost

import (
	"encoding/base64"
	"encoding/json"
)

const MaxAssetDecodeBytes = 64 << 20

// AssetDecode selects bytes only from an original task-owned handle. It has no
// paths, URLs, expressions or media claims; output validation stays in the host.
type AssetDecode struct {
	ID       string `json:"id"`
	Encoding string `json:"encoding"`
	Pointer  string `json:"pointer,omitempty"`
	MaxBytes int64  `json:"maxBytes"`
}

func DecodeAsset(source []byte, selection AssetDecode) ([]byte, error) {
	if !opaqueID.MatchString(selection.ID) || selection.MaxBytes < 1 || selection.MaxBytes > MaxAssetDecodeBytes || len(source) > MaxAssetDecodeBytes {
		return nil, ErrContract
	}
	var text string
	switch selection.Encoding {
	case "base64":
		if selection.Pointer != "" {
			return nil, ErrContract
		}
		text = string(source)
	case "json-base64", "json-text", "json":
		value, err := strictJSON(source)
		if err != nil {
			return nil, err
		}
		value, present, err := pointerValue(value, selection.Pointer)
		if err != nil || !present {
			return nil, ErrContract
		}
		if selection.Encoding == "json" {
			out, err := json.Marshal(value)
			if err != nil || int64(len(out)) > selection.MaxBytes {
				return nil, ErrContract
			}
			return out, nil
		}
		var ok bool
		text, ok = value.(string)
		if !ok {
			return nil, ErrContract
		}
		if selection.Encoding == "json-text" {
			if int64(len(text)) > selection.MaxBytes {
				return nil, ErrContract
			}
			return []byte(text), nil
		}
	default:
		return nil, ErrContract
	}
	// Strict additionally rejects nonzero padding bits; canonical comparison
	// rejects CR/LF accepted by encoding/base64 and excludes alternate alphabets.
	if base64.StdEncoding.DecodedLen(len(text)) > int(selection.MaxBytes)+2 {
		return nil, ErrContract
	}
	out, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || int64(len(out)) > selection.MaxBytes || base64.StdEncoding.EncodeToString(out) != text {
		return nil, ErrContract
	}
	return out, nil
}
