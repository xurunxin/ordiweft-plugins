package pluginhost

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// AssetBundle is descriptive data. File formats and adoption remain host owned.
type AssetBundle struct {
	SchemaVersion int                `json:"schemaVersion"`
	Type          string             `json:"type"`
	Name          string             `json:"name"`
	License       string             `json:"license"`
	Attribution   string             `json:"attribution"`
	Models        []AssetBundleModel `json:"models"`
}

type AssetBundleModel struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	File     string   `json:"file"`
	Preview  string   `json:"preview,omitempty"`
}

func DecodeAssetBundle(body []byte) (AssetBundle, error) {
	var out AssetBundle
	if len(body) == 0 || len(body) > 64<<10 {
		return out, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return out, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || out.SchemaVersion != 1 || out.Type != "director-models" || len(out.Models) < 1 || len(out.Models) > 32 {
		return out, ErrContract
	}
	text := func(value string, max int) bool {
		return strings.TrimSpace(value) != "" && len([]rune(value)) <= max && strings.IndexFunc(value, unicode.IsControl) < 0
	}
	if !text(out.Name, 120) || !text(out.License, 120) || !text(out.Attribution, 300) || len(out.License+" · "+out.Attribution) > 480 {
		return out, ErrContract
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, model := range out.Models {
		if !aigc.IDPattern.MatchString(model.ID) || ids[model.ID] || !text(model.Name, 120) || !text(model.Category, 80) || model.Tags == nil || len(model.Tags) > 20 {
			return out, ErrContract
		}
		ids[model.ID] = true
		for _, tag := range model.Tags {
			if !text(tag, 40) {
				return out, ErrContract
			}
		}
		for index, path := range []string{model.File, model.Preview} {
			if index == 1 && path == "" {
				continue
			}
			if !ValidPath(path) || paths[path] {
				return out, ErrContract
			}
			paths[path] = true
		}
	}
	return out, nil
}
