package pluginhost

import (
	"encoding/json"
	"path"
	"strings"
)

const WorkspaceCompiler = "node-typescript-transform-v1"
const MaxWorkspaceBuildBytes = 64_000

// WorkspaceBuildRecipe selects a fixed host compiler, never a command or hook.
type WorkspaceBuildRecipe struct {
	Version  int               `json:"version"`
	Compiler string            `json:"compiler"`
	Entries  map[string]string `json:"entries"`
}

func (s WorkspaceSource) BuildRecipe() (WorkspaceBuildRecipe, error) {
	var recipe WorkspaceBuildRecipe
	body, exists := s.Files["build.json"]
	if !exists || s.Validate() != nil {
		return recipe, ErrContract
	}
	if _, err := strictJSON([]byte(body)); err != nil {
		return recipe, ErrContract
	}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&recipe) != nil || recipe.Version != 1 || recipe.Compiler != WorkspaceCompiler || len(recipe.Entries) == 0 || len(recipe.Entries) > 16 {
		return recipe, ErrContract
	}
	seen := map[string]bool{}
	for source, target := range recipe.Entries {
		_, exists := s.Files[source]
		key := strings.ToLower(target)
		if !exists || !WorkspacePath(source) || path.Ext(source) != ".ts" || !WorkspacePath(target) || path.Ext(target) != ".mjs" || seen[key] {
			return recipe, ErrContract
		}
		seen[key] = true
	}
	return recipe, nil
}

// ArchiveCompiled requires the exact declared output set. Its caller owns the
// actual compiler/resource evidence; this function only checks frozen bytes.
func (s WorkspaceSource) ArchiveCompiled(base Descriptor, compiled map[string]string) ([]byte, error) {
	files, err := s.compiledFiles(compiled)
	if err != nil || base.Runtime != "node" {
		return nil, ErrContract
	}
	return (WorkspaceSource{BaseDigest: s.BaseDigest, Files: files}).archive(base)
}

func (s WorkspaceSource) ValidateCompiled(compiled map[string]string) error {
	_, err := s.compiledFiles(compiled)
	return err
}

func (s WorkspaceSource) compiledFiles(compiled map[string]string) (map[string]string, error) {
	recipe, err := s.BuildRecipe()
	if err != nil || len(compiled) != len(recipe.Entries) {
		return nil, ErrContract
	}
	files := make(map[string]string, len(s.Files)+len(compiled))
	for name, body := range s.Files {
		files[name] = body
	}
	total := 0
	for _, target := range recipe.Entries {
		body, exists := compiled[target]
		if !exists {
			return nil, ErrContract
		}
		total += len(body)
		files[target] = body
	}
	if total > MaxWorkspaceBuildBytes {
		return nil, ErrContract
	}
	if (WorkspaceSource{BaseDigest: s.BaseDigest, Files: files}).Validate() != nil {
		return nil, ErrContract
	}
	return files, nil
}
