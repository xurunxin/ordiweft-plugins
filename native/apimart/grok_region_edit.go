// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func normalizeGrokRegions(body map[string]any) error {
	regions, _ := body["regions"].([]any)
	if len(regions) == 0 || len(regions) > 16 {
		return errors.New("Grok 区域编辑需要 1 至 16 个矩形选区")
	}
	boxes := make([][4]float64, 0, len(regions))
	for _, value := range regions {
		region, _ := value.(map[string]any)
		x1, x1OK := region["x1"].(float64)
		y1, y1OK := region["y1"].(float64)
		x2, x2OK := region["x2"].(float64)
		y2, y2OK := region["y2"].(float64)
		if !x1OK || !x2OK || !y1OK || !y2OK || x1 < 0 || y1 < 0 || x2 > 1 || y2 > 1 || x1 >= x2 || y1 >= y2 {
			return errors.New("Grok 选区坐标需要位于 0–1 之间，右下角必须在左上角之后")
		}
		boxes = append(boxes, [4]float64{x1, y1, x2, y2})
	}
	delete(body, "regions")
	body["boxes"] = boxes
	return nil
}

func (p *plugin) prepareGrokRegionEdit(ctx context.Context, key string, spec modelSpec, edit map[string]any, source string) (string, error) {
	segment, _ := json.Marshal(map[string]any{"model": spec.Upstream, "operation": "segment", "source_task_id": source, "include_mask_rle": false})
	reply, err := p.request(ctx, http.MethodPost, spec.Endpoint, key, "application/json", segment)
	if err != nil {
		return "", err
	}
	ids, err := submission(reply)
	if err != nil || len(ids) != 1 {
		return "", errors.New("APIMart did not return a single Grok segmentation task")
	}
	task, err := p.wait(ctx, key, "/v1/tasks/", ids[0])
	if err != nil {
		return "", err
	}
	var result struct {
		ImageID     string `json:"image_id"`
		CacheStatus string `json:"cache_status"`
	}
	if json.Unmarshal(task.Result, &result) != nil || result.CacheStatus == "miss" || strings.TrimSpace(result.ImageID) == "" || len(result.ImageID) > 256 {
		return "", errors.New("APIMart did not return the source image ID; region edit was not submitted")
	}
	edit["operation"], edit["image_id"] = "region_edit", result.ImageID
	return ids[0], nil
}
