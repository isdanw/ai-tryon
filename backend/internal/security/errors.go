package security

import "fmt"

type APIError struct {
	HTTPStatus int    `json:"-"`
	BizCode    int    `json:"code"`
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("[%d:%s] %s", e.BizCode, e.ErrorCode, e.Message)
}

func NewAPIError(httpStatus, bizCode int, errCode, message string) *APIError {
	return &APIError{
		HTTPStatus: httpStatus,
		BizCode:    bizCode,
		ErrorCode:  errCode,
		Message:    message,
	}
}

var (
	ErrAuthRequired = NewAPIError(401, 40101, "AUTH_REQUIRED", "请在设置中心填写后端访问 Token")
	ErrAuthInvalid  = NewAPIError(401, 40102, "AUTH_INVALID", "后端访问 Token 无效")

	ErrOriginNotAllowed = NewAPIError(403, 40301, "ORIGIN_NOT_ALLOWED", "当前页面来源不允许访问后端")

	ErrRateLimited        = NewAPIError(429, 42901, "RATE_LIMITED", "请求过于频繁，请稍后重试")
	ErrDailyQuotaExceeded = NewAPIError(429, 42902, "DAILY_QUOTA_EXCEEDED", "今日生成额度已用完")
	ErrConcurrencyLimited = NewAPIError(429, 42903, "CONCURRENCY_LIMITED", "当前生成任务较多，请稍后重试")

	ErrRequestTooLarge         = NewAPIError(413, 41301, "REQUEST_TOO_LARGE", "上传内容过大，请压缩图片后重试")
	ErrInvalidImageBase64      = NewAPIError(422, 42201, "INVALID_IMAGE_BASE64", "图片数据损坏，请重新选择图片")
	ErrUnsupportedImageType    = NewAPIError(422, 42202, "UNSUPPORTED_IMAGE_TYPE", "仅支持 JPEG、PNG 和 WebP")
	ErrImageFileTooLarge       = NewAPIError(422, 42203, "IMAGE_FILE_TOO_LARGE", "图片文件过大，请压缩后重试")
	ErrImageDimensionsInvalid  = NewAPIError(422, 42204, "IMAGE_DIMENSIONS_INVALID", "图片尺寸不符合要求")
	ErrImagePixelLimitExceeded = NewAPIError(422, 42205, "IMAGE_PIXEL_LIMIT_EXCEEDED", "图片像素过高，请缩小后重试")
)

const (
	CodeInputImageModerationBlocked  = 42206
	CodeOutputImageModerationBlocked = 42207

	ErrCodeInputImageModerationBlocked  = "INPUT_IMAGE_MODERATION_BLOCKED"
	ErrCodeOutputImageModerationBlocked = "OUTPUT_IMAGE_MODERATION_BLOCKED"
)

const (
	msgInputImageModerationBlocked  = "输入图片未通过内容审核：人物、服装或姿势图中有一张被火山方舟拦截，请更换图片后重试（原样重试仍会被拦截）"
	msgOutputImageModerationBlocked = "生成结果未通过内容审核：火山方舟拦截了本次生成结果，请调整服装或姿势后重试（原样重试仍会被拦截）"
)

// NewModerationBlockedError 构造内容审核拦截错误；requestID 来自上游错误体，可为空。
func NewModerationBlockedError(output bool, requestID string) *APIError {
	bizCode := CodeInputImageModerationBlocked
	errCode := ErrCodeInputImageModerationBlocked
	message := msgInputImageModerationBlocked
	if output {
		bizCode = CodeOutputImageModerationBlocked
		errCode = ErrCodeOutputImageModerationBlocked
		message = msgOutputImageModerationBlocked
	}
	if requestID != "" {
		message += "；上游 Request id: " + requestID
	}
	return NewAPIError(422, bizCode, errCode, message)
}
