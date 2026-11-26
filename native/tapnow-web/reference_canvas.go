package tapnowweb

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

// Reference nodes describe the same uploaded inputs sent to generation. The
// connections are presentation metadata, never a substitute for the wire body.
func (c *Client) createRemoteNodes(ctx context.Context, revision uint64, ticket Ticket, node map[string]any, refs []tapnow.Reference, position map[string]int) error {
	nodes := []any{node}
	connections := []any{}
	ordinals := map[string]int{}
	refs = append([]tapnow.Reference(nil), refs...)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Order < refs[j].Order })
	for i, ref := range refs {
		ordinals[ref.Kind]++
		id := fmt.Sprintf("ref-%s-%d", ticket.NodeID, i)
		label := map[string]string{"image": "Image", "video": "Video", "audio": "Audio"}[ref.Kind]
		nodes = append(nodes, map[string]any{"id": id, "canvas_id": ticket.CanvasID, "type": ref.Kind,
			"position":   map[string]int{"x": position["x"] - 1080 + (i%3)*360, "y": position["y"] + (i/3)*240},
			"dimensions": map[string]int{"width": 280, "height": 180},
			"data":       map[string]any{"type": "upload", "src": ref.URL, "title": fmt.Sprintf("%s %d", label, ordinals[ref.Kind])}})
		connections = append(connections, map[string]any{"id": fmt.Sprintf("input-%s-%d", ticket.NodeID, i), "canvas_id": ticket.CanvasID,
			"source": id, "target": ticket.NodeID, "source_handle": "right", "target_handle": "left", "type": "default", "deletable": true, "selectable": true,
			"data": map[string]any{"purpose": "generation-input", "order": ref.Order}})
	}
	path := "/api/canvas/v1/canvases/" + url.PathEscape(ticket.CanvasID)
	if err := c.requestForAccount(ctx, revision, "POST", path+"/nodes:batchActions", map[string]any{"canvas_id": ticket.CanvasID, "created_by_role": "user", "actions": []any{map[string]any{"action": "create", "creates": nodes}}}, nil); err != nil {
		return err
	}
	if len(connections) == 0 {
		return nil
	}
	return c.requestForAccount(ctx, revision, "POST", path+"/connections:batchActions", map[string]any{"canvas_id": ticket.CanvasID, "created_by_role": "user", "actions": []any{map[string]any{"action": "create", "creates": connections}}}, nil)
}
