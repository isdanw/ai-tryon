package tryon

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// DetectGeneratedImageMIME determines the real media type from the generated
// file signature instead of trusting provider headers or declarations.
func DetectGeneratedImageMIME(value string) (string, error) {
	raw := strings.TrimSpace(ensureBase64(value))
	if raw == "" {
		return "", fmt.Errorf("生成结果图片为空")
	}

	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("生成结果图片 base64 格式错误")
	}

	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", nil
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", nil
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp", nil
	}
	return "", fmt.Errorf("生成结果不是受支持的图片格式")
}
