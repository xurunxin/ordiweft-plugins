package tapnowweb

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type SubjectAsset struct {
	ID        string `json:"asset_id"`
	Type      string `json:"asset_type"`
	Name      string `json:"name"`
	URL       string `json:"source_url"`
	Thumbnail string `json:"thumbnail_url"`
	Content   string `json:"content"`
	Order     int    `json:"sort_order"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Duration  int64  `json:"duration_ms"`
}
type Subject struct {
	ID          string         `json:"element_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Revision    scalar         `json:"revision"`
	Thumbnail   string         `json:"thumbnail_url"`
	Count       int            `json:"asset_count"`
	Assets      []SubjectAsset `json:"assets,omitempty"`
}
type SubjectPage struct {
	Items      []Subject `json:"items"`
	Pagination struct {
		Page     int  `json:"page"`
		PageSize int  `json:"page_size"`
		Total    int  `json:"total"`
		HasNext  bool `json:"has_next"`
	} `json:"pagination"`
}

func (c *Client) Subjects(ctx context.Context, space, keyword string, page int) (SubjectPage, error) {
	var out SubjectPage
	scope := "SPACE_TYPE_PRIVATE"
	if space == "team" {
		scope = "SPACE_TYPE_TEAM"
	} else if space != "private" && space != "" {
		return out, errors.New("主体空间无效")
	}
	if page < 1 || page > 1000 || len(keyword) > 200 {
		return out, errors.New("主体查询超限")
	}
	query := url.Values{"space_type": {scope}, "page": {strconv.Itoa(page)}, "page_size": {"20"}}
	path := "/api/library/v1/elements"
	if strings.TrimSpace(keyword) != "" {
		path += ":search"
		query.Set("keyword", strings.TrimSpace(keyword))
	}
	e := c.request(ctx, "GET", path+"?"+query.Encode(), nil, &out)
	if out.Items == nil {
		out.Items = []Subject{}
	}
	return out, e
}
func (c *Client) Subject(ctx context.Context, id string) (Subject, error) {
	var out Subject
	if !safeFileID(id) {
		return out, errors.New("主体 ID 无效")
	}
	if e := c.request(ctx, "GET", "/api/library/v1/elements/"+url.PathEscape(id), nil, &out); e != nil {
		return out, e
	}
	if out.ID != id || len(out.Assets) > 80 {
		return Subject{}, errors.New("主体详情不一致或超限")
	}
	sort.SliceStable(out.Assets, func(i, j int) bool { return out.Assets[i].Order < out.Assets[j].Order })
	return out, nil
}
func (c *Client) CreateSubject(ctx context.Context, name, description, key string, r Request, resolve Resolve) (Subject, error) {
	var out Subject
	if strings.TrimSpace(name) == "" || len(name) > 200 || len(description) > 8000 || !safeFileID(key) {
		return out, errors.New("主体名称、描述或幂等键无效")
	}
	if e := c.PrepareReferences(ctx, r, resolve); e != nil {
		return out, e
	}
	i, e := c.Identity(ctx)
	if e != nil {
		return out, e
	}
	members := []any{}
	if strings.TrimSpace(description) != "" {
		members = append(members, map[string]any{"inlineAsset": map[string]any{"assetType": "ASSET_TYPE_TEXT", "name": name + "介绍", "content": description}, "sortOrder": 0})
	}
	for _, ref := range r.References {
		u := ref.URL
		if strings.HasPrefix(u, "/files/") {
			m, e := resolve(ctx, u)
			if e != nil {
				return out, e
			}
			m, e = inspectMaterial(m, ref.Kind)
			if e != nil {
				return out, e
			}
			u = c.cachedURL(i, m)
			if u == "" {
				return out, errors.New("主体素材尚未准备")
			}
		}
		if e := publicURL(u); e != nil {
			return out, e
		}
		typ := map[string]string{"image": "ASSET_TYPE_IMAGE", "video": "ASSET_TYPE_VIDEO", "audio": "ASSET_TYPE_AUDIO"}[ref.Kind]
		if typ == "" {
			return out, errors.New("主体素材类型无效")
		}
		members = append(members, map[string]any{"inlineAsset": map[string]any{"assetType": typ, "name": ref.Kind + " reference", "sourceUrl": u}, "sortOrder": len(members)})
	}
	e = c.request(ctx, "POST", "/api/library/v1/elements", map[string]any{"spaceType": "SPACE_TYPE_PRIVATE", "name": name, "description": description, "sortOrder": 0, "members": members, "idempotencyKey": key}, &out)
	if e == nil && !safeFileID(out.ID) {
		return out, errors.New("主体创建结果未知，请刷新主体列表，勿重复提交")
	}
	return out, e
}
func (c *Client) Transcribe(ctx context.Context, m Material) (string, error) {
	m, e := inspectMaterial(m, "audio")
	if e != nil {
		return "", e
	}
	if len(m.Bytes) > 20<<20 || m.Duration > 120 {
		return "", errors.New("语音最多 120 秒 / 20 MiB")
	}
	u, e := c.Upload(ctx, m)
	if e != nil {
		return "", e
	}
	const prefix = "https://files.tapnow.media/api/conversation/storage/uploads/"
	id := strings.TrimPrefix(u, prefix)
	if !strings.HasPrefix(u, prefix) || !safeFileID(id) {
		return "", errors.New("语音上传响应无效")
	}
	var d struct {
		Text string `json:"text"`
	}
	if e = c.request(ctx, "POST", "/api/chat/v1/beta/asr/transcribe", map[string]any{"fileId": id, "mimeType": m.Mime, "hints": []string{}, "mode": "lite"}, &d); e != nil {
		return "", e
	}
	if len(d.Text) > 20000 {
		return "", errors.New("转写结果超长")
	}
	return strings.TrimSpace(d.Text), nil
}
