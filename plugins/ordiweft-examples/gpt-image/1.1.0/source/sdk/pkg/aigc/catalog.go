package aigc

import (
	"context"
	"errors"
	"strings"
)

// ModelCatalog contains identifiers, never credentials or executable schemas.
// The adapter resolves each ID against its verified parameter/API contracts.
type ModelCatalog struct {
	IDs         []string `json:"ids"`
	UpdatedAt   string   `json:"updatedAt"`
	Unsupported []string `json:"unsupported"`
}

// ModelDiscovery is optional and does not change the SDK v1/v2 wire protocol.
// WithModelCatalog returns an immutable plugin snapshot and must not make calls.
type ModelDiscovery interface {
	ListModels(context.Context, map[string]string) ([]string, error)
	WithModelCatalog(ModelCatalog) (Plugin, []string, error)
}

// CatalogManifest uses exact known contracts. A listed name alone never grants
// a new API, parameter, input role, or scheduling policy.
func CatalogManifest(base Manifest, catalog ModelCatalog) (Manifest, []string, error) {
	if len(catalog.IDs) == 0 || len(catalog.IDs) > 5000 {
		return Manifest{}, nil, errors.New("模型目录为空或过大，保留原目录")
	}
	known := map[string]Model{}
	for _, m := range base.Models {
		known[m.ID] = m
	}
	models := []Model{}
	unsupported := []string{}
	seen := map[string]bool{}
	for _, id := range catalog.IDs {
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "\r\n\x00") || seen[id] {
			return Manifest{}, nil, errors.New("模型目录 ID 无效或重复")
		}
		seen[id] = true
		if m, ok := known[id]; ok {
			models = append(models, m)
		} else {
			unsupported = append(unsupported, id)
		}
	}
	if len(models) == 0 {
		return Manifest{}, nil, errors.New("目录未返回已适配的生成模型，保留原目录")
	}
	for _, m := range base.Models {
		if m.Capability == "voice.design" && !seen[m.ID] {
			models = append(models, m)
		}
	}
	base.Models = models
	nodes := []Node{}
	for _, n := range base.Nodes {
		first := ""
		preferred := false
		for _, m := range models {
			if m.Capability == n.Capability {
				if first == "" {
					first = m.ID
				}
				if m.ID == n.Model {
					preferred = true
				}
			}
		}
		if first != "" {
			if !preferred {
				n.Model = first
			}
			nodes = append(nodes, n)
		}
	}
	base.Nodes = nodes
	return base, unsupported, base.Validate()
}
