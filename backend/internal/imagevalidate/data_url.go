package imagevalidate

import (
	"strings"

	"backend/internal/security"
)

// ParseDataURL 解析 Data URL。如果输入以 "data:" 开头，解析声明的 MIME 类型并提取纯 Base64 数据；
// 如果不以 "data:" 开头，返回空 MIME 和原始输入。
func ParseDataURL(input string) (declaredMIME string, rawBase64 string, err error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "data:") {
		return "", trimmed, nil
	}

	commaIdx := strings.Index(trimmed, ",")
	if commaIdx == -1 {
		return "", "", security.ErrInvalidImageBase64
	}

	header := trimmed[:commaIdx]
	rawBase64 = trimmed[commaIdx+1:]

	if !strings.Contains(header, ";base64") {
		return "", "", security.ErrInvalidImageBase64
	}

	// 提取媒体类型：例如 data:image/png;base64 -> image/png
	mediaPart := strings.TrimPrefix(header, "data:")
	mediaPart = strings.TrimSuffix(mediaPart, ";base64")
	declaredMIME = strings.ToLower(strings.TrimSpace(mediaPart))

	if declaredMIME != "image/jpeg" && declaredMIME != "image/png" && declaredMIME != "image/webp" {
		return "", "", security.ErrUnsupportedImageType
	}

	return declaredMIME, rawBase64, nil
}
