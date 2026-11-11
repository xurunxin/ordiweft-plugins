package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const MaxJSONRequestBytes = 64 << 20

// JSONRequestPolicy is reviewed data, not permission to connect. An installed
// connection still binds origin/credentials, original account and effect slots.
type JSONRequestPolicy struct {
	Version      int                            `json:"version"`
	Models       map[string]string              `json:"models"`
	Body         json.RawMessage                `json:"body"`
	ModelPointer string                         `json:"modelPointer,omitempty"`
	FixedModel   string                         `json:"fixedModel,omitempty"`
	CountPointer string                         `json:"countPointer,omitempty"`
	SingleOutput bool                           `json:"singleOutput,omitempty"`
	QueryOnly    bool                           `json:"queryOnly,omitempty"`
	UploadOnly   bool                           `json:"uploadOnly,omitempty"`
	WhenAny      []map[string][]json.RawMessage `json:"whenAny,omitempty"`
}

// JSONRequestValues is supplied from one original host-approved request and its
// original saved responses. It is never decoded from a plugin control message.
type JSONRequestValues struct {
	Generation aigc.Generation
	Count      int
	JobID      string
	Receipts   map[string]json.RawMessage
	Inputs     []RequestInput
}

type requestSource struct {
	kind, name, ref, pointer  string
	optional                  bool
	index                     int
	encoding, inputKind, role string
	extension, uriPrefix      string
	extensionsByMime          map[string]string
}

func DecodeJSONRequestPolicy(body []byte) (JSONRequestPolicy, error) {
	var policy JSONRequestPolicy
	if len(body) == 0 || len(body) > 128<<10 {
		return policy, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return policy, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&policy) != nil || d.Decode(new(any)) != io.EOF {
		return policy, ErrContract
	}
	return policy, policy.Validate()
}

func (p JSONRequestPolicy) template() (any, error) {
	if p.Version != Version || len(p.Models) == 0 || len(p.Models) > 256 || len(p.Body) == 0 || len(p.Body) > 64<<10 || (p.ModelPointer == "") == (p.FixedModel == "") {
		return nil, ErrContract
	}
	if p.QueryOnly && p.UploadOnly || (p.QueryOnly || p.UploadOnly) && (p.CountPointer != "" || p.SingleOutput) || !p.QueryOnly && !p.UploadOnly && (p.CountPointer == "") != p.SingleOutput {
		return nil, ErrContract
	}
	if validateRequestConditions(p.WhenAny) != nil {
		return nil, ErrContract
	}
	for model, remote := range p.Models {
		if !policyName(model, 256) || !policyName(remote, 256) || p.FixedModel != "" && remote != p.FixedModel {
			return nil, ErrContract
		}
	}
	if p.ModelPointer != "" {
		if _, err := pointerParts(p.ModelPointer); err != nil {
			return nil, err
		}
	}
	if p.CountPointer != "" {
		if _, err := pointerParts(p.CountPointer); err != nil {
			return nil, err
		}
	}
	template, err := strictJSON(p.Body)
	if _, ok := template.(map[string]any); err != nil || !ok {
		return nil, ErrContract
	}
	return template, nil
}

func (p JSONRequestPolicy) Validate() error {
	_, err := p.ReceiptReferences()
	return err
}

func (p JSONRequestPolicy) Digest() (string, error) {
	if p.Validate() != nil {
		return "", ErrContract
	}
	template, _ := strictJSON(p.Body)
	p.Body, _ = json.Marshal(template)
	p.WhenAny = canonicalRequestConditions(p.WhenAny)
	body, err := json.Marshal(p)
	if err != nil {
		return "", ErrContract
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}

func (p JSONRequestPolicy) Build(values JSONRequestValues) ([]byte, error) {
	template, err := p.template()
	remote, exists := p.Models[values.Generation.Model]
	if err != nil || !exists || len(values.Inputs) > 256 || values.Count < 1 || values.Count > MaxTaskOutputs || p.SingleOutput && values.Count != 1 {
		return nil, ErrContract
	}
	if !matchesRequestConditions(p.WhenAny, values) {
		return nil, ErrContract
	}
	inputBudget := MaxJSONRequestBytes
	result, present, err := renderRequest(template, func(source requestSource) (any, bool, error) {
		switch source.kind {
		case "model":
			return remote, true, nil
		case "count":
			return json.Number(strconv.Itoa(values.Count)), true, nil
		case "prompt":
			return values.Generation.Prompt, true, nil
		case "job":
			if !opaqueID.MatchString(values.JobID) {
				return nil, false, ErrContract
			}
			return values.JobID, true, nil
		case "parameter":
			value, ok := values.Generation.Parameters[source.name]
			if !ok {
				return nil, false, nil
			}
			encoded, err := json.Marshal(value)
			if err != nil || len(encoded) > MaxJSONRequestBytes {
				return nil, false, ErrContract
			}
			normalized, err := strictJSON(encoded)
			if err != nil {
				return nil, false, err
			}
			return pointerValue(normalized, source.pointer)
		case "receipt":
			raw, ok := values.Receipts[source.ref]
			if !ok {
				return nil, false, nil
			}
			if len(raw) > 256<<20 {
				return nil, false, ErrContract
			}
			value, err := strictJSON(raw)
			if err != nil {
				return nil, false, err
			}
			return pointerValue(value, source.pointer)
		case "input-key":
			return renderInputKey(source, values)
		case "input", "inputs":
			for _, index := range inputIndices(source, values.Inputs) {
				input := values.Inputs[index]
				if input.Bytes < 0 || input.Bytes > MaxJSONRequestBytes {
					return nil, false, ErrContract
				}
				inputBudget -= ((int(input.Bytes)+2)/3)*4 + len(input.Mime) + 14
				if inputBudget < 0 {
					return nil, false, ErrContract
				}
			}
			return renderInputSource(source, values.Inputs)
		}
		return nil, false, ErrContract
	})
	if err != nil || !present {
		return nil, ErrContract
	}
	if p.ModelPointer != "" {
		model, ok, err := pointerValue(result, p.ModelPointer)
		if err != nil || !ok || model != remote {
			return nil, ErrContract
		}
	}
	if p.CountPointer != "" {
		count, ok, err := pointerValue(result, p.CountPointer)
		encoded, encodeErr := json.Marshal(count)
		if err != nil || !ok || encodeErr != nil || string(encoded) != strconv.Itoa(values.Count) {
			return nil, ErrContract
		}
	}
	body, err := json.Marshal(result)
	if err != nil || len(body) > MaxJSONRequestBytes {
		return nil, ErrContract
	}
	if _, err = strictJSON(body); err != nil {
		return nil, err
	}
	return body, nil
}

// ReceiptReferences names original call IDs selected by this installed policy.
// A reference is not a response or authority; the host loads committed bytes.
func (p JSONRequestPolicy) ReceiptReferences() ([]string, error) {
	template, err := p.template()
	if err != nil {
		return nil, err
	}
	refs := map[string]bool{}
	_, _, err = renderRequest(template, func(source requestSource) (any, bool, error) {
		if source.kind == "receipt" || source.kind == "input-key" {
			refs[source.ref] = true
		}
		return nil, true, nil
	})
	if err != nil {
		return nil, err
	}
	if len(refs) > 64 {
		return nil, ErrContract
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out, nil
}

// ValidateBody rejects a changed model/count/field, duplicate JSON member or
// parser ambiguity before the original effect slot can be claimed.
func (p JSONRequestPolicy) ValidateBody(values JSONRequestValues, body []byte) error {
	if len(body) > MaxJSONRequestBytes {
		return ErrContract
	}
	actual, err := strictJSON(body)
	if err != nil {
		return err
	}
	template, err := p.template()
	if err != nil {
		return err
	}
	// The final store transaction has only original metadata, never a second
	// blob store. Recover bytes from each selected body field only after checking
	// its canonical encoding, original size and SHA. Repeated uses must agree.
	values.Inputs = append([]RequestInput(nil), values.Inputs...)
	if err = bindRequestInputBytes(template, actual, values.Inputs); err != nil {
		return err
	}
	expected, err := p.Build(values)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(actual)
	if err != nil || !bytes.Equal(expected, canonical) {
		return ErrContract
	}
	return nil
}

func policyName(value string, max int) bool {
	return value != "" && len(value) <= max && !strings.ContainsAny(value, "\x00\r\n")
}

func parseRequestSource(value map[string]any) (requestSource, error) {
	var out requestSource
	out.kind, _ = value["$host"].(string)
	allowed := map[string]bool{"$host": true}
	switch out.kind {
	case "model", "count", "prompt", "job":
	case "parameter":
		allowed["name"], allowed["optional"], allowed["pointer"] = true, true, true
		out.name, _ = value["name"].(string)
		if !policyName(out.name, 128) {
			return out, ErrContract
		}
		if raw, present := value["pointer"]; present {
			pointer, ok := raw.(string)
			if !ok {
				return out, ErrContract
			}
			if _, err := pointerParts(pointer); err != nil {
				return out, err
			}
			out.pointer = pointer
		}
	case "receipt":
		allowed["ref"], allowed["pointer"], allowed["optional"] = true, true, true
		out.ref, _ = value["ref"].(string)
		pointer, ok := value["pointer"].(string)
		if !opaqueID.MatchString(out.ref) || !ok {
			return out, ErrContract
		}
		if _, err := pointerParts(pointer); err != nil {
			return out, err
		}
		out.pointer = pointer
	case "input-key":
		allowed["ref"], allowed["pointer"], allowed["index"], allowed["extension"], allowed["uriPrefix"], allowed["extensionsByMime"] = true, true, true, true, true, true
		out.ref, _ = value["ref"].(string)
		out.pointer, _ = value["pointer"].(string)
		out.extension, _ = value["extension"].(string)
		number, ok := value["index"].(json.Number)
		index, err := strconv.Atoi(string(number))
		_, pointerPresent := value["pointer"].(string)
		if !ok || err != nil || index < 0 || index >= 256 || strconv.Itoa(index) != string(number) || !opaqueID.MatchString(out.ref) || !pointerPresent {
			return out, ErrContract
		}
		_, fixedExtension := value["extension"]
		mapping, mappedExtension := value["extensionsByMime"]
		if fixedExtension == mappedExtension {
			return out, ErrContract
		}
		if fixedExtension && !objectExtension(out.extension) {
			return out, ErrContract
		}
		if mappedExtension {
			entries, ok := mapping.(map[string]any)
			if !ok || len(entries) == 0 || len(entries) > 16 {
				return out, ErrContract
			}
			out.extensionsByMime = map[string]string{}
			for mime, raw := range entries {
				extension, ok := raw.(string)
				if !ok || !validInputMime(mime) || !objectExtension(extension) {
					return out, ErrContract
				}
				out.extensionsByMime[mime] = extension
			}
		}
		out.index = index
		if _, err := pointerParts(out.pointer); err != nil {
			return out, err
		}
		if raw, exists := value["uriPrefix"]; exists {
			prefix, ok := raw.(string)
			if !ok || !objectURIPrefix(prefix) {
				return out, ErrContract
			}
			out.uriPrefix = prefix
		}
	case "input", "inputs":
		allowed["encoding"] = true
		out.encoding, _ = value["encoding"].(string)
		if out.encoding != "base64" && out.encoding != "data-url" {
			return out, ErrContract
		}
		if out.kind == "input" {
			allowed["index"], allowed["optional"] = true, true
			number, ok := value["index"].(json.Number)
			if !ok {
				return out, ErrContract
			}
			index, err := strconv.Atoi(string(number))
			if err != nil || index < 0 || index >= 256 || strconv.Itoa(index) != string(number) {
				return out, ErrContract
			}
			out.index = index
		} else {
			allowed["kind"], allowed["role"] = true, true
			for key, target := range map[string]*string{"kind": &out.inputKind, "role": &out.role} {
				if raw, exists := value[key]; exists {
					text, ok := raw.(string)
					if !ok || !policyName(text, 128) {
						return out, ErrContract
					}
					*target = text
				}
			}
		}
	default:
		return out, ErrContract
	}
	for key := range value {
		if !allowed[key] {
			return out, ErrContract
		}
	}
	if optional, ok := value["optional"]; ok {
		flag, valid := optional.(bool)
		if !valid {
			return out, ErrContract
		}
		out.optional = flag
	}
	return out, nil
}

func renderRequest(template any, resolve func(requestSource) (any, bool, error)) (any, bool, error) {
	switch value := template.(type) {
	case map[string]any:
		if _, exists := value["$host"]; exists {
			source, err := parseRequestSource(value)
			if err != nil {
				return nil, false, err
			}
			resolved, present, err := resolve(source)
			if err != nil || !present && !source.optional {
				return nil, false, ErrContract
			}
			return resolved, present, nil
		}
		out := map[string]any{}
		for key, entry := range value {
			resolved, present, err := renderRequest(entry, resolve)
			if err != nil {
				return nil, false, err
			}
			if present {
				out[key] = resolved
			}
		}
		return out, true, nil
	case []any:
		out := make([]any, len(value))
		for i, entry := range value {
			resolved, present, err := renderRequest(entry, resolve)
			if err != nil || !present {
				return nil, false, ErrContract
			}
			out[i] = resolved
		}
		return out, true, nil
	default:
		return value, true, nil
	}
}

func pointerParts(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if pointer[0] != '/' || len(pointer) > 1024 || strings.ContainsAny(pointer, "\x00\r\n") {
		return nil, ErrContract
	}
	parts := strings.Split(pointer[1:], "/")
	if len(parts) > 32 {
		return nil, ErrContract
	}
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' {
				if j+1 >= len(part) || part[j+1] != '0' && part[j+1] != '1' {
					return nil, ErrContract
				}
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func pointerValue(value any, pointer string) (any, bool, error) {
	parts, err := pointerParts(pointer)
	if err != nil {
		return nil, false, err
	}
	for _, part := range parts {
		switch current := value.(type) {
		case map[string]any:
			var exists bool
			value, exists = current[part]
			if !exists {
				return nil, false, nil
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return nil, false, nil
			}
			value = current[index]
		default:
			return nil, false, nil
		}
	}
	return value, true, nil
}

// strictJSON preserves decimal tokens and rejects duplicate names. Comparing a
// last-key-wins parser with a first-key-wins upstream must never authorize spend.
func strictJSON(body []byte) (any, error) {
	if !utf8.Valid(body) {
		return nil, ErrContract
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	nodes := 0
	value, err := strictJSONValue(decoder, 0, &nodes)
	if err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrContract
	}
	return value, nil
}

func strictJSONValue(decoder *json.Decoder, depth int, nodes *int) (any, error) {
	(*nodes)++
	if depth > 32 || *nodes > 8192 {
		return nil, ErrContract
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, ErrContract
	}
	switch token {
	case json.Delim('{'):
		out := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok {
				return nil, ErrContract
			}
			if _, duplicate := out[name]; duplicate {
				return nil, ErrContract
			}
			entry, err := strictJSONValue(decoder, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[name] = entry
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
			return nil, ErrContract
		}
		return out, nil
	case json.Delim('['):
		out := []any{}
		for decoder.More() {
			entry, err := strictJSONValue(decoder, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out = append(out, entry)
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			return nil, ErrContract
		}
		return out, nil
	default:
		if _, delimiter := token.(json.Delim); delimiter {
			return nil, ErrContract
		}
		return token, nil
	}
}
