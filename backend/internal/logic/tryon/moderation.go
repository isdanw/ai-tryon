package tryon

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ModerationBlockedKind 标识被上游内容审核拦截的是输入图还是生成结果。
type ModerationBlockedKind string

const (
	ModerationBlockedInput  ModerationBlockedKind = "input"
	ModerationBlockedOutput ModerationBlockedKind = "output"
)

// ModerationBlockedError 火山方舟内容审核拦截错误。
// 只保留上游错误码与 Request id，不携带任何请求体或 Key 信息。
type ModerationBlockedError struct {
	Kind         ModerationBlockedKind
	UpstreamCode string
	RequestID    string
}

func (e *ModerationBlockedError) Error() string {
	msg := fmt.Sprintf("内容审核拦截 (kind=%s, code=%s)", e.Kind, e.UpstreamCode)
	if e.RequestID != "" {
		msg += ", request_id=" + e.RequestID
	}
	return msg
}

var moderationRequestIDPattern = regexp.MustCompile(`Request id:\s*([A-Za-z0-9]+)`)

// parseModerationBlocked 识别形如
// {"error":{"code":"InputImageSensitiveContentDetected","message":"... Request id: ..."}}
// 的上游 400 响应；非审核类错误返回 nil。
func parseModerationBlocked(body []byte) *ModerationBlockedError {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	code := payload.Error.Code
	var kind ModerationBlockedKind
	switch {
	case strings.Contains(code, "InputImageSensitiveContentDetected"):
		kind = ModerationBlockedInput
	case strings.Contains(code, "OutputImageSensitiveContentDetected"):
		kind = ModerationBlockedOutput
	default:
		return nil
	}

	requestID := ""
	if m := moderationRequestIDPattern.FindStringSubmatch(payload.Error.Message); len(m) == 2 {
		requestID = m[1]
	}

	return &ModerationBlockedError{
		Kind:         kind,
		UpstreamCode: code,
		RequestID:    requestID,
	}
}

// AsModerationBlocked 沿错误链判断是否为内容审核拦截错误。
func AsModerationBlocked(err error) (*ModerationBlockedError, bool) {
	var target *ModerationBlockedError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
