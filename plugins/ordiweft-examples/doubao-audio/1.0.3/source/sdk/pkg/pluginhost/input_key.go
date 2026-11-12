package pluginhost

import "strings"

func objectURIPrefix(value string) bool {
	if !strings.HasSuffix(value, "://") || len(value) < 4 || len(value) > 35 {
		return false
	}
	for i, ch := range strings.TrimSuffix(value, "://") {
		if !(ch >= 'a' && ch <= 'z' || i > 0 && (ch >= '0' && ch <= '9' || strings.ContainsRune("+.-", ch))) {
			return false
		}
	}
	return true
}

func objectExtension(value string) bool {
	if len(value) < 2 || len(value) > 17 || value[0] != '.' {
		return false
	}
	for _, ch := range value[1:] {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return true
}

// Object names use only original immutable metadata, not a plugin filename.
func renderInputKey(source requestSource, values JSONRequestValues) (any, bool, error) {
	if source.index >= len(values.Inputs) || !ValidDigest(values.Inputs[source.index].SHA256) {
		return nil, false, ErrContract
	}
	extension := source.extension
	if len(source.extensionsByMime) > 0 {
		extension = source.extensionsByMime[values.Inputs[source.index].Mime]
		if extension == "" {
			return nil, false, ErrContract
		}
	}
	raw, exists := values.Receipts[source.ref]
	if !exists {
		return nil, false, ErrContract
	}
	value, err := strictJSON(raw)
	if err != nil {
		return nil, false, err
	}
	selected, present, err := pointerValue(value, source.pointer)
	dir, ok := selected.(string)
	if err != nil || !present || !ok || len(dir) == 0 || len(dir) > 1024 || strings.HasPrefix(dir, "/") || strings.HasSuffix(dir, "//") {
		return nil, false, ErrContract
	}
	dir = strings.TrimSuffix(dir, "/")
	for _, part := range strings.Split(dir, "/") {
		if part == "" || part == "." || part == ".." {
			return nil, false, ErrContract
		}
		for _, ch := range part {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-_.", ch)) {
				return nil, false, ErrContract
			}
		}
	}
	return source.uriPrefix + dir + "/" + values.Inputs[source.index].SHA256 + extension, true, nil
}
