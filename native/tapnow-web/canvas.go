package tapnowweb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type canvasMapping struct {
	LocalID  string `json:"localId"`
	RemoteID string `json:"remoteId,omitempty"`
	Next     int    `json:"next"`
	NextY    int    `json:"nextY,omitempty"`
}

func (c *Client) canvasMappingName(identity Identity, localID string) string {
	key, _ := json.Marshal([]string{c.origin, identity.User, identity.Org, localID})
	hash := sha256.Sum256(key)
	return "canvas-" + hex.EncodeToString(hash[:]) + ".json"
}

// remoteCanvas reserves a position before creating the node. A pending mapping
// prevents another create when the first create's outcome is unknown.
func (c *Client) remoteCanvas(ctx context.Context, identity Identity, localID string, revision uint64, referenceCount int, expectedRemote string) (string, map[string]int, error) {
	c.canvasMu.Lock()
	defer c.canvasMu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	current := c.accountRevision
	c.mu.Unlock()
	if current != revision {
		return "", nil, errors.New("TapNow 登录身份已变更，请重新确认任务")
	}
	name := c.canvasMappingName(identity, localID)
	var mapping canvasMapping
	b, err := os.ReadFile(filepath.Join(c.dir, name))
	if err == nil {
		if json.Unmarshal(b, &mapping) != nil || mapping.LocalID != localID || !safeFileID(mapping.RemoteID) || mapping.Next < 0 || mapping.Next > 1000000 || mapping.NextY < 0 || mapping.NextY > 2000000000 {
			return "", nil, errors.New("TapNow 画布映射未确认或损坏，请管理员检查")
		}
		if expectedRemote != "" && mapping.RemoteID != expectedRemote {
			return "", nil, errDraftUnavailable
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, errors.New("无法读取 TapNow 画布映射")
	} else if expectedRemote != "" {
		return "", nil, errDraftUnavailable
	} else {
		token, org, err := c.accountCredentials(ctx, revision)
		if err != nil {
			return "", nil, err
		}
		mapping.LocalID = localID
		pending, _ := json.Marshal(mapping)
		if err = c.save(name, pending); err != nil {
			return "", nil, errors.New("无法保存 TapNow 画布创建记录")
		}
		var remote struct {
			ID     string `json:"id"`
			Canvas struct {
				ID string `json:"id"`
			} `json:"canvas"`
		}
		body := map[string]any{"name": "Ordiweft · " + localID, "description": "Ordiweft canvas generation tasks", "is_public": false, "is_shared_with_org": false}
		if err = c.requestWithToken(ctx, "POST", "/api/canvas/v1/canvases", body, &remote, token, org); err != nil {
			if errors.Is(err, errUnauthorized) {
				c.mu.Lock()
				if c.token == token {
					c.verified = false
				}
				c.mu.Unlock()
			}
			return "", nil, errors.New("TapNow 画布创建结果未确认，请管理员检查")
		}
		mapping.RemoteID = remote.ID
		if mapping.RemoteID == "" {
			mapping.RemoteID = remote.Canvas.ID
		}
		if !safeFileID(mapping.RemoteID) {
			return "", nil, errors.New("TapNow 画布创建结果未确认，请管理员检查")
		}
	}
	if mapping.NextY == 0 {
		mapping.NextY = 100 + ((mapping.Next+3)/4)*400
	}
	position := map[string]int{"x": 1180, "y": mapping.NextY}
	mapping.NextY += max(400, ((referenceCount+2)/3)*240)
	mapping.Next++
	b, _ = json.Marshal(mapping)
	if err = c.save(name, b); err != nil {
		return "", nil, errors.New("无法保存 TapNow 画布映射")
	}
	return mapping.RemoteID, position, nil
}
