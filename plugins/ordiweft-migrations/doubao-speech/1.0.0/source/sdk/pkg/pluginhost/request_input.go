package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
)

// RequestInput is private host material. Metadata is frozen at original
// admission; Data is resolved under current asset authority by the runner.
type RequestInput struct {
	Kind, Role, Mime, SHA256 string
	Bytes                    int64
	Data                     []byte `json:"-"`
	PublicURL                string `json:"publicURL,omitempty"`
	VideoDurationMS          int64  `json:"videoDurationMs,omitempty"`
}

func inputIndices(source requestSource, inputs []RequestInput) []int {
	if source.kind == "input" {
		if source.index >= len(inputs) {
			return nil
		}
		return []int{source.index}
	}
	out := []int{}
	for i, input := range inputs {
		if (source.inputKind == "" || source.inputKind == input.Kind) && (source.role == "" || source.role == input.Role) {
			out = append(out, i)
		}
	}
	return out
}

func encodeRequestInput(input RequestInput, encoding string) (string, error) {
	if encoding == "public-url" {
		if !ValidInputPublicURL(input.PublicURL) || input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes || !ValidDigest(input.SHA256) || !validInputMime(input.Mime) || input.Data != nil && validateRequestInputBytes(input) != nil {
			return "", ErrContract
		}
		return input.PublicURL, nil
	}
	if err := validateRequestInputBytes(input); err != nil {
		return "", err
	}
	text := base64.StdEncoding.EncodeToString(input.Data)
	if encoding == "data-url" {
		text = "data:" + input.Mime + ";base64," + text
	}
	return text, nil
}

// Empty means that the original host had no published CDN representation.
// Callers cannot substitute a plugin-selected URL for that missing authority.
func ValidInputPublicURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 4096 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && !strings.ContainsAny(raw, "\x00\r\n")
}

func validateRequestInputBytes(input RequestInput) error {
	if input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes || int64(len(input.Data)) != input.Bytes || !ValidDigest(input.SHA256) || !validInputMime(input.Mime) {
		return ErrContract
	}
	h := sha256.Sum256(input.Data)
	if hex.EncodeToString(h[:]) != input.SHA256 {
		return ErrContract
	}
	return nil
}

func validInputMime(mime string) bool {
	parts := strings.Split(mime, "/")
	return len(mime) <= 128 && len(parts) == 2 && headerToken(parts[0]) && headerToken(parts[1])
}

func renderInputSource(source requestSource, inputs []RequestInput) (any, bool, error) {
	indices := inputIndices(source, inputs)
	if source.kind == "input" && len(indices) == 0 {
		return nil, false, nil
	}
	out := make([]string, 0, len(indices))
	var size int
	for _, index := range indices {
		input := inputs[index]
		// Bound encoded expansion before allocating it, including repeated inputs.
		if input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes {
			return nil, false, ErrContract
		}
		if source.encoding == "public-url" {
			size += len(input.PublicURL)
		} else {
			size += base64.StdEncoding.EncodedLen(int(input.Bytes)) + len(input.Mime) + 14
		}
		if size > MaxJSONRequestBytes {
			return nil, false, ErrContract
		}
		text, err := encodeRequestInput(input, source.encoding)
		if err != nil {
			return nil, false, err
		}
		out = append(out, text)
	}
	if source.kind == "input" {
		return out[0], true, nil
	}
	if source.property != "" {
		objects := make([]map[string]string, 0, len(out))
		for _, text := range out {
			objects = append(objects, map[string]string{source.property: text})
		}
		return objects, true, nil
	}
	return out, true, nil
}

func bindRequestInputBytes(template, actual any, inputs []RequestInput) error {
	switch value := template.(type) {
	case map[string]any:
		if _, exists := value["$host"]; exists {
			source, err := parseRequestSource(value)
			if err != nil {
				return err
			}
			if source.kind != "input" && source.kind != "inputs" {
				return nil
			}
			if source.encoding == "video-duration-ms" {
				// ValidateBody rebuilds numeric values from the original input
				// snapshot. No authority for these facts comes from the body.
				return nil
			}
			indices := inputIndices(source, inputs)
			if source.kind == "input" && len(indices) == 0 {
				return nil
			}
			var encoded []any
			if source.kind == "input" {
				encoded = []any{actual}
			} else {
				var ok bool
				encoded, ok = actual.([]any)
				if !ok || len(encoded) != len(indices) {
					return ErrContract
				}
			}
			for i, index := range indices {
				value := encoded[i]
				if source.property != "" {
					object, ok := value.(map[string]any)
					if !ok || len(object) != 1 {
						return ErrContract
					}
					value = object[source.property]
				}
				text, ok := value.(string)
				if !ok {
					return ErrContract
				}
				input := &inputs[index]
				if source.encoding == "public-url" {
					if text != input.PublicURL || !ValidInputPublicURL(text) || input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes || !ValidDigest(input.SHA256) || !validInputMime(input.Mime) {
						return ErrContract
					}
					continue
				}
				if input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes || !validInputMime(input.Mime) {
					return ErrContract
				}
				if source.encoding == "data-url" {
					prefix := "data:" + input.Mime + ";base64,"
					if !strings.HasPrefix(text, prefix) {
						return ErrContract
					}
					text = strings.TrimPrefix(text, prefix)
				}
				if len(text) != base64.StdEncoding.EncodedLen(int(input.Bytes)) {
					return ErrContract
				}
				decoded, err := base64.StdEncoding.Strict().DecodeString(text)
				if err != nil || int64(len(decoded)) != input.Bytes || base64.StdEncoding.EncodeToString(decoded) != text {
					return ErrContract
				}
				h := sha256.Sum256(decoded)
				if hex.EncodeToString(h[:]) != input.SHA256 || input.Data != nil && !bytes.Equal(input.Data, decoded) {
					return ErrContract
				}
				input.Data = decoded
			}
			return nil
		}
		object, ok := actual.(map[string]any)
		if !ok {
			return ErrContract
		}
		for key, child := range value {
			if err := bindRequestInputBytes(child, object[key], inputs); err != nil {
				return err
			}
		}
	case []any:
		array, ok := actual.([]any)
		if !ok || len(array) != len(value) {
			return ErrContract
		}
		for i, child := range value {
			if err := bindRequestInputBytes(child, array[i], inputs); err != nil {
				return err
			}
		}
	}
	return nil
}
