// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
)

func (p *plugin) modelPrice(ctx context.Context, model string) (json.RawMessage, error) {
	var result struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	payload, err := p.request(ctx, http.MethodGet, "/api/pricing/model?"+url.Values{"model": {model}}.Encode(), "", "", nil)
	if err != nil {
		return result.Data, err
	}
	var identity struct {
		Model string `json:"model_name"`
	}
	if json.Unmarshal(payload, &result) != nil || !result.Success ||
		json.Unmarshal(result.Data, &identity) != nil || identity.Model != model {
		return nil, errors.New("APIMart returned an invalid public price record")
	}
	return result.Data, nil
}

func (p *plugin) printPrices(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: provider prices <model-id>")
	}
	spec, ok := p.models[args[0]]
	if !ok {
		return fmt.Errorf("unknown APIMart model: %s", args[0])
	}
	remote, err := p.modelPrice(ctx, spec.Upstream)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"provider": provider, "model": spec.Model.ID, "tier": "gold",
		"currency": "USD", "current": remote, "manifestPricing": spec.Model.Pricing,
	})
}
