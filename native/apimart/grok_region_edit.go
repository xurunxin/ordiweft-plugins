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
	mode, _ := body["selection_mode"].(string)
	delete(body, "selection_mode")
	switch mode {
	case "polygon":
		return normalizeGrokPolygons(body)
	case "objects":
		return validateGrokObjectIndices(body)
	default:
		return normalizeGrokRectangles(body)
	}
}

func normalizeGrokRectangles(body map[string]any) error {
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
		Objects     []struct {
			Index *int `json:"index"`
		} `json:"objects"`
	}
	if json.Unmarshal(task.Result, &result) != nil || result.CacheStatus == "miss" || strings.TrimSpace(result.ImageID) == "" || len(result.ImageID) > 256 {
		return "", errors.New("APIMart did not return the source image ID; region edit was not submitted")
	}
	if indices, ok := edit["object_indices"].([]int); ok {
		members := make(map[int]bool, len(result.Objects))
		for _, object := range result.Objects {
			if object.Index != nil {
				members[*object.Index] = true
			}
		}
		for _, index := range indices {
			if !members[index] {
				return "", errors.New("所选对象索引不属于原图分段结果")
			}
		}
	}
	edit["operation"], edit["image_id"] = "region_edit", result.ImageID
	return ids[0], nil
}

func validateGrokObjectIndices(body map[string]any) error {
	values, _ := body["object_indices"].([]any)
	if len(values) == 0 || len(values) > 16 {
		return errors.New("请选择 1 至 16 个对象索引")
	}
	ids := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		id, ok := value.(float64)
		if !ok || id < 0 || id > 1000000 || id != float64(int(id)) || seen[int(id)] {
			return errors.New("Grok 对象索引必须是唯一的非负整数")
		}
		seen[int(id)] = true
		ids = append(ids, int(id))
	}
	body["object_indices"] = ids
	return nil
}

type grokRing struct {
	Points []float64 `json:"points"`
}
type grokPolygon struct {
	Outer grokRing   `json:"outer"`
	Holes []grokRing `json:"holes,omitempty"`
}

func normalizeGrokPolygons(body map[string]any) error {
	values, _ := body["polygons"].([]any)
	if len(values) == 0 || len(values) > 16 {
		return errors.New("请提供 1 至 16 个多边形")
	}
	regions := make([]grokPolygon, 0, len(values))
	for _, item := range values {
		region, _ := item.(map[string]any)
		outer, err := readGrokRing(region["outer"])
		if err != nil {
			return err
		}
		polygon := grokPolygon{Outer: outer}
		holes, _ := region["holes"].([]any)
		if len(holes) > 8 {
			return errors.New("每个多边形最多 8 个孔洞")
		}
		for _, item := range holes {
			hole, _ := item.(map[string]any)
			ring, err := readGrokRing(hole["points"])
			if err != nil {
				return err
			}
			polygon.Holes = append(polygon.Holes, ring)
		}
		regions = append(regions, polygon)
	}
	delete(body, "polygons")
	body["selection_regions"] = regions
	return nil
}

func readGrokRing(value any) (grokRing, error) {
	input, _ := value.([]any)
	if len(input) < 3 || len(input) > 128 {
		return grokRing{}, errors.New("每条轮廓需要 3 至 128 个坐标点")
	}
	ring := grokRing{Points: make([]float64, 0, 2*len(input))}
	for _, item := range input {
		point, _ := item.(map[string]any)
		x, xOK := point["x"].(float64)
		y, yOK := point["y"].(float64)
		if !xOK || !yOK || x < 0 || y < 0 || x > 1 || y > 1 {
			return grokRing{}, errors.New("Grok 坐标必须位于 0–1 之间")
		}
		ring.Points = append(ring.Points, x, y)
	}
	var area float64
	for i := 0; i < len(ring.Points); i += 2 {
		next := (i + 2) % len(ring.Points)
		area += ring.Points[i]*ring.Points[next+1] - ring.Points[next]*ring.Points[i+1]
	}
	if area == 0 {
		return grokRing{}, errors.New("多边形轮廓需要围成有效区域")
	}
	return ring, nil
}
