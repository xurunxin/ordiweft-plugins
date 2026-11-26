package tapnowweb

import (
	"context"
	"errors"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"net/url"
	"slices"
)

func (c *Client) writeFailure(ctx context.Context, ticket Ticket, original map[string]any, failure *TaskFailedError, revision uint64) error {
	if len(ticket.TaskIDs) == 0 {
		return errors.New("缺少原任务编号")
	}
	data := merge(original, map[string]any{"taskInfo": map[string]any{"taskId": ticket.TaskIDs[0], "status": "failed", "injectType": "src", "error": failure.Error()}, "error": failure.Error()})
	return c.requestForAccount(ctx, revision, "POST", "/api/canvas/v1/canvases/"+url.PathEscape(ticket.CanvasID)+"/nodes:batchActions", map[string]any{"canvas_id": ticket.CanvasID, "created_by_role": "user", "actions": []any{map[string]any{"action": "update", "updates": []any{map[string]any{"id": ticket.NodeID, "data": data}}}}}, nil)
}

func (c *Client) writeBack(ctx context.Context, t Ticket, r Request, mode Mode, media []tapnow.Media, revision uint64) error {
	if len(media) == 0 {
		return errors.New("产物为空")
	}
	urls := []string{}
	for _, m := range media {
		urls = append(urls, m.URL)
	}
	params := r.remoteParameters(merge(mode.Defaults, r.Parameters))
	data := map[string]any{"type": "generate", "prompt": r.Prompt, "params": params, "src": urls[0], "options": urls, "historyPreviewSrc": "", "historyVariantCount": len(urls), "taskInfo": map[string]any{"taskId": t.TaskIDs[0], "status": "completed", "injectType": "src"}}
	if fileID := DraftFileID(urls[0]); r.Kind == "video" && fileID != "" {
		data["currentSourceFileId"] = fileID
	}
	if r.CameraControl != nil {
		data["cameraControl"] = r.CameraControl
	}
	path := "/api/canvas/v1/canvases/" + url.PathEscape(t.CanvasID)
	body := map[string]any{"canvas_id": t.CanvasID, "created_by_role": "user", "actions": []any{map[string]any{"action": "update", "updates": []any{map[string]any{"id": t.NodeID, "data": data}}}}}
	if e := c.requestForAccount(ctx, revision, "POST", path+"/nodes:batchActions", body, nil); e != nil {
		return errors.New("生成已完成，但远端节点写回未确认；保留票据，禁止重发")
	}
	type remoteNode struct {
		ID   string `json:"id"`
		Data struct {
			Src     string   `json:"src"`
			Options []string `json:"options"`
		} `json:"data"`
	}
	var read struct {
		Nodes  []remoteNode `json:"nodes"`
		Canvas struct {
			Nodes []remoteNode `json:"nodes"`
		} `json:"canvas"`
	}
	if e := c.requestForAccount(ctx, revision, "GET", path+"?with_nodes=true&with_connections=true", nil, &read); e != nil {
		return errors.New("远端产物写回读取未确认；保留票据")
	}
	nodes := read.Nodes
	if len(nodes) == 0 {
		nodes = read.Canvas.Nodes
	}
	for _, n := range nodes {
		if n.ID == t.NodeID && n.Data.Src == urls[0] {
			for _, u := range urls {
				if !slices.Contains(n.Data.Options, u) {
					return errors.New("远端节点遗漏产物，保留票据")
				}
			}
			return nil
		}
	}
	return errors.New("远端节点写回不一致；禁止重复生成")
}
