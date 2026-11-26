package tapnowweb

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ImageReview struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
type assetReview struct {
	FileID       string `json:"file_id"`
	Status       string `json:"status"`
	RejectReason string `json:"reject_reason"`
}

const assetReviewPath = "/api/canvas/v1/asset-reviews"
const tapnowUploadPrefix = "https://files.tapnow.media/api/conversation/storage/uploads/"

func tapnowUploadID(source string) string {
	id, found := strings.CutPrefix(source, tapnowUploadPrefix)
	if !found || !safeFileID(id) {
		return ""
	}
	return id
}

func (c *Client) readAssetReviews(ctx context.Context, revision uint64, ids []string) ([]assetReview, error) {
	var list struct {
		Reviews []assetReview `json:"reviews"`
	}
	query := url.Values{"file_ids": ids}
	err := c.requestForAccount(ctx, revision, "GET", assetReviewPath+"?"+query.Encode(), nil, &list)
	return list.Reviews, err
}

// Check only images uploaded under the job's account. Unknown or conflicting
// results leave the reference eligible for TapNow's regular moderation.
func (c *Client) bindSeedanceImageReviews(ctx context.Context, revision uint64, images []any, uploaded map[string]string) error {
	sources := make(map[string]bool, len(uploaded))
	for _, source := range uploaded {
		sources[source] = true
	}
	items := map[string][]map[string]any{}
	var ids []string
	for _, image := range images {
		part := obj(image)
		source := str(part, "url")
		if !sources[source] {
			continue
		}
		if id := tapnowUploadID(source); id != "" {
			if len(items[id]) == 0 {
				ids = append(ids, id)
			}
			items[id] = append(items[id], part)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reviews, err := c.readAssetReviews(check, revision, ids)
	if err != nil {
		return nil
	}
	status := make(map[string]string, len(reviews))
	for _, review := range reviews {
		if _, duplicate := status[review.FileID]; duplicate {
			status[review.FileID] = ""
			continue
		}
		status[review.FileID] = reviewResult(review).Status
	}
	for _, id := range ids {
		switch status[id] {
		case "rejected":
			return ProviderFailure("provider_media_rejected")
		case "rejected_invalid":
			return ProviderFailure("invalid_format")
		}
	}
	for id, parts := range items {
		for _, part := range parts {
			part["is_verified"] = status[id] == "approved"
		}
	}
	return nil
}

func reviewResult(r assetReview) ImageReview {
	switch r.Status {
	case "ASSET_REVIEW_STATUS_APPROVED":
		return ImageReview{"approved", "图片素材审核通过；不代表提示词或最终视频一定通过审核。"}
	case "ASSET_REVIEW_STATUS_PENDING":
		return ImageReview{"pending", "素材审核已排队，请查询原审核结果。"}
	case "ASSET_REVIEW_STATUS_PROCESSING":
		return ImageReview{"processing", "正在审核图片素材，请稍后查询结果。"}
	case "ASSET_REVIEW_STATUS_REJECTED":
		if r.RejectReason == "ASSET_REJECT_REASON_INVALID_PARAMETER" {
			return ImageReview{"rejected_invalid", "素材审核参数不符合要求，请检查图片格式或更换素材。"}
		}
		return ImageReview{"rejected", "图片素材未通过审核，请检查素材内容并更换合适的图片。"}
	default:
		return ImageReview{"unknown", "尚无可确认的审核结果；请先查询原审核，勿重复提交。"}
	}
}

// ImageReview checks the original asset review before submitting. GET never uploads.
func (c *Client) ImageReview(ctx context.Context, m Material, submit bool) (ImageReview, error) {
	m, err := inspectMaterial(m, "image")
	if err != nil || int64(len(m.Bytes)) > c.MaxReferenceBytes {
		return ImageReview{}, errors.New("图片实际格式或大小不支持")
	}
	c.reviewMu.Lock()
	defer c.reviewMu.Unlock()
	identity, err := c.Identity(ctx)
	if err != nil {
		return ImageReview{}, err
	}
	c.mu.Lock()
	revision := c.accountRevision
	c.mu.Unlock()
	source := c.cachedURL(identity, m)
	if source == "" {
		if !submit {
			return ImageReview{"unknown", "尚未提交图片素材审核。"}, nil
		}
		source, err = c.cachedUpload(ctx, identity, m)
		if err != nil {
			return ImageReview{}, err
		}
	}
	id := tapnowUploadID(source)
	if id == "" {
		return ImageReview{}, errors.New("审核素材地址无效")
	}
	reviews, err := c.readAssetReviews(ctx, revision, []string{id})
	if err != nil {
		return ImageReview{}, err
	}
	for _, review := range reviews {
		if review.FileID == id {
			return reviewResult(review), nil
		}
	}
	if !submit {
		return ImageReview{"unknown", "尚未查询到图片素材审核；若刚刚提交，请稍后继续查询。"}, nil
	}
	// Persist before the external write: a lost response or restart must not
	// turn a status check into another submission for the same account/material.
	marker := "image-review-" + materialKey(identity, m) + ".json"
	if _, err := os.Stat(filepath.Join(c.dir, marker)); err == nil {
		return ImageReview{"unknown", "原审核已尝试提交但尚未确认，请继续查询或到 TapNow 检查；不会重复提交。"}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return ImageReview{}, errors.New("无法读取原审核记录")
	}
	if err := c.save(marker, []byte(`{"submitted":true}`)); err != nil {
		return ImageReview{}, errors.New("无法保存审核记录，尚未提交审核")
	}
	var response struct {
		Review assetReview `json:"review"`
	}
	if err = c.requestForAccount(ctx, revision, "POST", assetReviewPath, map[string]string{"file_id": id, "file_url": source, "asset_type": "Image"}, &response); err != nil {
		return ImageReview{}, errors.New("素材审核提交未确认，请查询原审核；不会自动重新提交")
	}
	return reviewResult(response.Review), nil
}
