package pluginhost

import "net/url"

const MaxQueryBytes = 8192

// BuildQuery expands installed fields from the original task. No query string
// is accepted on the plugin control channel; final dispatch rebuilds it again.
func (o ConnectionOperation) BuildQuery(policy JSONRequestPolicy, values JSONRequestValues) (string, error) {
	if !o.QueryParameters || o.Method != "GET" || o.Effect != "query" || o.JSONPolicy == nil || o.ContentType != "" || o.MaxRequestBytes != 0 || o.InputBody != nil || len(o.Multipart) != 0 {
		return "", ErrContract
	}
	fields, names, err := multipartFields(policy, values)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	for _, name := range names {
		if len(fields[name]) > 2048 {
			return "", ErrContract
		}
		query.Set(name, fields[name])
	}
	encoded := query.Encode()
	if len(encoded) > MaxQueryBytes {
		return "", ErrContract
	}
	return encoded, nil
}
