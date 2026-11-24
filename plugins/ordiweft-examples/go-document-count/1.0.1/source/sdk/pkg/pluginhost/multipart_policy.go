package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strconv"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// MultipartFile selects original immutable bytes, not a plugin path or handle.
type MultipartFile struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Index    int    `json:"index"`
	Optional bool   `json:"optional,omitempty"`
}

func formName(value string) bool {
	if !policyName(value, 128) || strings.ContainsAny(value, "\"\\") {
		return false
	}
	for _, ch := range value {
		if ch < 32 || ch > 126 {
			return false
		}
	}
	return true
}

func validateMultipartFiles(files []MultipartFile) error {
	if len(files) == 0 || len(files) > 32 {
		return ErrContract
	}
	for _, file := range files {
		if !formName(file.Name) || !formName(file.Filename) || strings.Contains(file.Filename, "/") || file.Filename == "." || file.Filename == ".." || file.Index < 0 || file.Index >= 256 {
			return ErrContract
		}
	}
	return nil
}

func multipartBoundary(jobID, operation string) (string, error) {
	if !policyName(jobID, 128) || !aigc.IDPattern.MatchString(operation) {
		return "", ErrContract
	}
	body, _ := json.Marshal([]string{"multipart-v1", jobID, operation})
	h := sha256.Sum256(body)
	return "ordiweft-" + hex.EncodeToString(h[:20]), nil
}

// RequestContentType includes a deterministic boundary for the original task.
// It is part of the request identity and cannot be chosen in a control frame.
func (o ConnectionOperation) RequestContentType(jobID, operation string) (string, error) {
	if len(o.Multipart) == 0 {
		return o.ContentType, nil
	}
	if o.ContentType != "multipart/form-data" || validateMultipartFiles(o.Multipart) != nil {
		return "", ErrContract
	}
	boundary, err := multipartBoundary(jobID, operation)
	if err != nil {
		return "", err
	}
	return "multipart/form-data; boundary=" + boundary, nil
}

func multipartFields(policy JSONRequestPolicy, values JSONRequestValues) (map[string]string, []string, error) {
	template, err := policy.template()
	if err != nil {
		return nil, nil, err
	}
	_, _, err = renderRequest(template, func(source requestSource) (any, bool, error) {
		if source.kind == "input" || source.kind == "inputs" {
			return nil, false, ErrContract
		}
		return nil, true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	body, err := policy.Build(values)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := strictJSON(body)
	fields, ok := decoded.(map[string]any)
	if err != nil || !ok || len(fields) > 128 {
		return nil, nil, ErrContract
	}
	out := map[string]string{}
	names := make([]string, 0, len(fields))
	for name, value := range fields {
		if !formName(name) {
			return nil, nil, ErrContract
		}
		switch scalar := value.(type) {
		case string:
			out[name] = scalar
		case json.Number:
			out[name] = string(scalar)
		case bool:
			out[name] = strconv.FormatBool(scalar)
		default:
			return nil, nil, ErrContract
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return out, names, nil
}

func selectedMultipartFiles(files []MultipartFile, inputs []RequestInput, fields map[string]string) ([]MultipartFile, error) {
	if validateMultipartFiles(files) != nil || len(inputs) > 256 {
		return nil, ErrContract
	}
	selected := make([]MultipartFile, 0, len(files))
	for _, file := range files {
		if _, exists := fields[file.Name]; exists {
			return nil, ErrContract
		}
		if file.Index >= len(inputs) {
			if file.Optional {
				continue
			}
			return nil, ErrContract
		}
		selected = append(selected, file)
	}
	return selected, nil
}

type multipartBuffer struct{ body bytes.Buffer }

func (b *multipartBuffer) Write(body []byte) (int, error) {
	if len(body) > MaxJSONRequestBytes-b.body.Len() {
		return 0, ErrContract
	}
	return b.body.Write(body)
}

// BuildBody expands only installed fields and original input selections.
func (o ConnectionOperation) BuildBody(policy JSONRequestPolicy, values JSONRequestValues, operation string) ([]byte, error) {
	if o.QueryParameters || policy.QueryOnly && o.Effect != "query" {
		return nil, ErrContract
	}
	if o.InputBody != nil {
		return o.InputBody.build(values.Inputs)
	}
	if len(o.Multipart) == 0 {
		return policy.Build(values)
	}
	if _, err := o.RequestContentType(values.JobID, operation); err != nil {
		return nil, err
	}
	fields, names, err := multipartFields(policy, values)
	if err != nil {
		return nil, err
	}
	files, err := selectedMultipartFiles(o.Multipart, values.Inputs, fields)
	if err != nil {
		return nil, err
	}
	boundary, _ := multipartBoundary(values.JobID, operation)
	var buffer multipartBuffer
	writer := multipart.NewWriter(&buffer)
	if writer.SetBoundary(boundary) != nil {
		return nil, ErrContract
	}
	for _, name := range names {
		if strings.Contains(fields[name], boundary) {
			return nil, ErrContract
		}
		part, err := writer.CreateFormField(name)
		if err != nil {
			return nil, err
		}
		if _, err = io.WriteString(part, fields[name]); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		input := values.Inputs[file.Index]
		if validateRequestInputBytes(input) != nil || bytes.Contains(input.Data, []byte(boundary)) {
			return nil, ErrContract
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": file.Name, "filename": file.Filename}))
		header.Set("Content-Type", input.Mime)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(input.Data); err != nil {
			return nil, err
		}
	}
	if writer.Close() != nil {
		return nil, ErrContract
	}
	return buffer.body.Bytes(), nil
}

// ValidateBody reconstructs the canonical multipart from verified file bytes.
// Final transactions need only original metadata and the actual request body.
func (o ConnectionOperation) ValidateBody(policy JSONRequestPolicy, values JSONRequestValues, operation string, body []byte) error {
	if policy.QueryOnly && o.Effect != "query" {
		return ErrContract
	}
	if o.QueryParameters {
		if len(body) != 0 {
			return ErrContract
		}
		return nil
	}
	if o.InputBody != nil {
		return o.InputBody.validate(values.Inputs, body)
	}
	if len(o.Multipart) == 0 {
		return policy.ValidateBody(values, body)
	}
	if len(body) > MaxJSONRequestBytes {
		return ErrContract
	}
	if _, err := o.RequestContentType(values.JobID, operation); err != nil {
		return err
	}
	fields, names, err := multipartFields(policy, values)
	if err != nil {
		return err
	}
	files, err := selectedMultipartFiles(o.Multipart, values.Inputs, fields)
	if err != nil {
		return err
	}
	boundary, _ := multipartBoundary(values.JobID, operation)
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for _, name := range names {
		part, err := reader.NextRawPart()
		if err != nil || part.FormName() != name || part.FileName() != "" {
			return ErrContract
		}
		data, err := io.ReadAll(io.LimitReader(part, int64(len(fields[name]))+1))
		if err != nil || string(data) != fields[name] {
			return ErrContract
		}
	}
	values.Inputs = append([]RequestInput(nil), values.Inputs...)
	for _, file := range files {
		input := &values.Inputs[file.Index]
		if input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes || !validInputMime(input.Mime) {
			return ErrContract
		}
		part, err := reader.NextRawPart()
		if err != nil || part.FormName() != file.Name || part.FileName() != file.Filename || part.Header.Get("Content-Type") != input.Mime {
			return ErrContract
		}
		data, err := io.ReadAll(io.LimitReader(part, input.Bytes+1))
		if err != nil || input.Data != nil && !bytes.Equal(input.Data, data) {
			return ErrContract
		}
		input.Data = data
		if validateRequestInputBytes(*input) != nil {
			return ErrContract
		}
	}
	if _, err = reader.NextRawPart(); err != io.EOF {
		return ErrContract
	}
	expected, err := o.BuildBody(policy, values, operation)
	if err != nil || !bytes.Equal(expected, body) {
		return ErrContract
	}
	return nil
}
