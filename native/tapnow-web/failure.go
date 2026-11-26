package tapnowweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const videoEditRequired = "Based on your prompt, this is a video editing task. Please switch to the Video Edit generation method and try again."
const rejectedOutputVideo = "The generated video may contain sensitive content. Please modify the description and try again."

var ErrSubmissionUnconfirmed = errors.New("TapNow Web 提交回执未确认，请核对原任务")

type ProviderFailure string

type FailureInfo struct {
	Code, Stage, Summary, Action string
}

var failures = map[string]FailureInfo{
	"insufficient_credits":       {"quota", "admission", "TapNow 积分不足", "请管理员补充 Tapies，或降低分辨率、时长与生成份数"},
	"team_quota":                 {"quota", "admission", "TapNow 团队额度受限", "请管理员核对该成员的团队额度与使用权限"},
	"cost_cap":                   {"quota", "admission", "TapNow 最新估价超过本次确认上限", "请刷新报价并重新确认金额"},
	"content_violation":          {"content_review", "moderation", "请求未通过审核", "请检查提示词和参考素材，调整后再决定是否生成"},
	"output_moderation":          {"content_review", "moderation", "生成的视频未通过内容审核", "请检查提示词与参考素材，调整画面内容后再决定是否重新生成"},
	"provider_media_rejected":    {"content_review", "moderation", "参考素材未通过上游审核", "请更换未通过审核的素材，再确认是否生成"},
	"invalid_parameters":         {"input", "input", "上游拒绝生成参数", "请检查生成方式、输入素材和参数"},
	"video_edit_required":        {"input", "input", "生成方式不匹配", "上游将本次请求判定为视频编辑，请切换为「视频编辑」并检查原视频"},
	"input_resolution_too_large": {"input", "input", "参考素材分辨率过高", "请按模型要求降低素材分辨率，或直接使用原素材"},
	"invalid_format":             {"input", "input", "参考素材格式不符合要求", "请检查素材格式或更换素材"},
	"invalid_duration":           {"input", "input", "参考素材时长不符合要求", "请按所选模型的时长范围剪辑素材"},
	"file_size_too_large":        {"input", "input", "参考素材文件太大", "请压缩素材或更换较小的文件"},
	"prompt_too_long":            {"input", "input", "提示词超过模型长度限制", "请缩短提示词再决定是否生成"},
	"rate_limited":               {"rate_limit", "admission", "TapNow 请求过于频繁", "请等当前任务结束并核对并发额度"},
	"provider_overloaded":        {"unavailable", "connection", "TapNow 模型服务繁忙", "请核对原任务和费用后再决定是否提交新任务"},
	"unsupported_model":          {"model_access", "admission", "当前 TapNow 模型不可用", "请检查账号的模型权限，或选择已开通的模型"},
	"permission":                 {"authorization", "authorization", "TapNow 账号权限不足", "请管理员核对账号、团队和模型权限"},
	"authorization":              {"authorization", "authorization", "TapNow 登录已失效", "请联系管理员重新连接 TapNow Web 账号"},
}

func (e ProviderFailure) Error() string {
	info := failures[string(e)]
	return "TapNow Web：" + info.Summary + "。" + info.Action + "。"
}

func (e *TaskFailedError) Error() string {
	if _, info, ok := KnownFailure(e); ok {
		return "TapNow Web：" + info.Summary + "。" + info.Action + "。"
	}
	return "TapNow Web 任务终止。请在 TapNow 查看原任务详情，核实原因和费用后再决定是否重新生成。"
}

func KnownFailure(err error) (string, FailureInfo, bool) {
	var failure ProviderFailure
	var task *TaskFailedError
	var reason string
	switch {
	case errors.As(err, &failure):
		reason = string(failure)
	case errors.As(err, &task):
		safe := task.Safe()
		reason = safe.Reason
		switch safe.Detail {
		case videoEditRequired:
			reason = "video_edit_required"
		case rejectedOutputVideo:
			reason = "output_moderation"
		}
	case errors.Is(err, errUnauthorized):
		reason = "authorization"
	}
	info, ok := failures[reason]
	return reason, info, ok
}

func failureReason(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	switch strings.TrimPrefix(key, "conversation_") {
	case "10006", "1402", "402", "http_402", "insufficient_credits", "insufficient_balance":
		return "insufficient_credits"
	case "10007", "10008", "team_quota":
		return "team_quota"
	case "provider_content_rejected":
		return "content_violation"
	case "media_rejected", "provider_media_rejected":
		return "provider_media_rejected"
	case "invalid_file_type", "file_type", "invalid_format":
		return "invalid_format"
	case "duration_too_long", "duration_too_short", "invalid_duration":
		return "invalid_duration"
	case "string_too_long", "prompt_too_long":
		return "prompt_too_long"
	case "provider_rate_limit", "rate_limited":
		return "rate_limited"
	case "provider_model_not_available", "provider_model_not_supported", "provider_model_not_found", "model_unavailable", "model_unavailable_timeout", "unsupported_model":
		return "unsupported_model"
	case "provider_permission_error":
		return "permission"
	case "content_violation", "invalid_parameters", "input_resolution_too_large", "file_size_too_large", "provider_overloaded":
		return key
	}
	return ""
}

func errorValue(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return errorScalar(value)
}

func errorScalar(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprint(v)
	}
	return ""
}

func responseFailure(status int, body []byte, authPath bool) error {
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(body, &envelope)
	if errorValue(envelope["code"]) == "990102" {
		return errUnauthorized
	}
	reason := responseReason(envelope, 0)
	if reason != "" {
		return ProviderFailure(reason)
	}
	switch status {
	case 402:
		return ProviderFailure("insufficient_credits")
	case 429:
		return ProviderFailure("rate_limited")
	case 403:
		if !authPath {
			return ProviderFailure("permission")
		}
	}
	if authPath {
		return errAuthRejected
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("TapNow Web HTTP %d", status)
	}
	return errors.New("TapNow Web 拒绝请求，未返回可识别的错误分类")
}

func responseReason(fields map[string]json.RawMessage, depth int) string {
	var reason string
	for _, key := range []string{"code", "error_code", "errorCode", "reason", "failed_reason", "error", "data"} {
		raw := fields[key]
		candidate := failureReason(errorValue(raw))
		if depth < 2 && (key == "error" || key == "data") && candidate == "" {
			var nested map[string]json.RawMessage
			if json.Unmarshal(raw, &nested) == nil {
				candidate = responseReason(nested, depth+1)
			}
		}
		if candidate == "team_quota" {
			return candidate
		}
		if reason == "" {
			reason = candidate
		}
	}
	return reason
}

// Safe retains recognized diagnostics, never arbitrary provider text or URLs.
func (e *TaskFailedError) Safe() *TaskFailedError {
	out := &TaskFailedError{Reason: failureReason(e.Reason)}
	switch out.Reason {
	case "invalid_parameters":
		if e.Detail == videoEditRequired {
			out.Detail = e.Detail
		}
	case "content_violation":
		switch e.Detail {
		case "The input text may violate platform rules. Please modify the text and try again.", rejectedOutputVideo:
			out.Detail = e.Detail
		}
	}
	return out
}

func (c *Client) ConfirmFailed(id string, failure *TaskFailedError) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.Receipt(id)
	if !ok || failure == nil || len(r.Ticket.TaskIDs) == 0 || r.Phase == "canceled" {
		return errors.New("原任务票据不可更新")
	}
	r.Phase, r.Failure = "failed", failure.Safe()
	return c.receipt(id, r)
}
