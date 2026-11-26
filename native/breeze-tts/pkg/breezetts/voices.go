package breezetts

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func (p *Plugin) Voices(ctx context.Context, secrets map[string]string) ([]aigc.VoiceOption, error) {
	items := []aigc.VoiceOption{}
	for _, source := range []string{"default", "personal"} {
		page, err := p.SearchVoices(ctx, secrets, aigc.VoiceQuery{Page: 1, PageSize: 100, Filters: map[string]any{"source": source, "language": "zh"}})
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
	}
	return items, nil
}

// SearchVoices reads one existing catalog page; it never requests synthesis.
func (p *Plugin) SearchVoices(ctx context.Context, secrets map[string]string, query aigc.VoiceQuery) (aigc.VoicePage, error) {
	var page aigc.VoicePage
	if !p.Online {
		return page, fmt.Errorf("本地 Breeze 不提供持久音色目录")
	}
	key := strings.TrimSpace(secrets["API_KEY"])
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return page, fmt.Errorf("Breeze API Key 尚未配置")
	}
	if query.Page < 1 || query.Page > 10000 || query.PageSize < 1 || query.PageSize > 100 || len(query.PageToken) > 8192 || utf8.RuneCountInString(query.Search) > 200 {
		return page, fmt.Errorf("Breeze 音色搜索参数无效")
	}
	filters, err := aigc.NormalizeParameters(aigc.Model{Fields: p.Manifest().VoiceCatalog.Filters}, query.Filters)
	if err != nil {
		return page, err
	}
	values := url.Values{"voice_type": {stringValue(filters["source"])}, "page": {strconv.Itoa(query.Page)}, "page_size": {strconv.Itoa(query.PageSize)}}
	for field, param := range map[string]string{"language": "language_code", "category": "primary_category_code", "gender": "gender", "age": "age", "tone": "tone"} {
		if v := stringValue(filters[field]); v != "" {
			values.Set(param, v)
		}
	}
	sort := stringValue(filters["sort"])
	if filters["source"] == "personal" && sort == "trend" {
		sort = "created_at_unix"
	}
	values.Set("sort", sort)
	if query.Search != "" {
		values.Set("search", query.Search)
	}
	if query.PageToken != "" {
		values.Set("next_page_token", query.PageToken)
	}
	var result struct {
		Voices []struct {
			ID          string   `json:"voice_id"`
			Name        string   `json:"name"`
			Language    string   `json:"language_code"`
			Description string   `json:"description"`
			Preview     string   `json:"preview_url"`
			Origin      string   `json:"origin"`
			Tags        []string `json:"tags"`
		} `json:"voices"`
		Next    string `json:"next_page_token"`
		HasMore bool   `json:"has_more"`
		Total   int    `json:"total"`
		Page    int    `json:"page"`
	}
	if err = p.jsonRequest(ctx, "GET", "/v1/voices?"+values.Encode(), key, nil, &result); err != nil {
		return page, err
	}
	if result.Page != query.Page || result.Total < 0 || len(result.Next) > 8192 || len(result.Voices) > query.PageSize {
		return page, fmt.Errorf("Breeze 音色搜索分页响应无效")
	}
	page = aigc.VoicePage{Items: []aigc.VoiceOption{}, Total: result.Total, Page: result.Page, HasMore: result.HasMore, NextPageToken: result.Next}
	for _, v := range result.Voices {
		if !identifier.MatchString(v.ID) || v.Name == "" {
			continue
		}
		category := "官方"
		if filters["source"] == "personal" {
			category = "设计"
			if v.Origin == "cloned" {
				category = "复刻"
			}
		}
		preview := ""
		u, e := url.Parse(v.Preview)
		if e == nil && u.Scheme == "https" && u.Hostname() == "r.breeze.blue" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
			preview = u.String()
		}
		page.Items = append(page.Items, aigc.VoiceOption{ID: v.ID, Label: v.Name, Category: category, Language: v.Language, Description: v.Description, PreviewURL: preview, SourceURL: "https://breezeblue.ai/zh/app/voice-library", Tags: v.Tags, CompatibleModels: []string{"breeze-tts-2"}})
	}
	return page, nil
}
