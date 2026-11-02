package imagevalidate

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

type ImageLimits struct {
	MaxRequestBodyBytes     int64    `json:"maxRequestBodyBytes"`
	MaxEncodedImageBytes    int64    `json:"maxEncodedImageBytes"`
	MaxDecodedImageBytes    int64    `json:"maxDecodedImageBytes"`
	MaxCombinedDecodedBytes int64    `json:"maxCombinedDecodedBytes"`
	MinWidth                int      `json:"minWidth"`
	MinHeight               int      `json:"minHeight"`
	MaxWidth                int      `json:"maxWidth"`
	MaxHeight               int      `json:"maxHeight"`
	MaxPixels               int64    `json:"maxPixels"`
	AllowedMimeTypes        []string `json:"allowedMimeTypes"`
}

func DefaultImageLimits() ImageLimits {
	return ImageLimits{
		MaxRequestBodyBytes:     25165824, // 24 MiB
		MaxEncodedImageBytes:    12582912, // 12 MiB
		MaxDecodedImageBytes:    8388608,  // 8 MiB
		MaxCombinedDecodedBytes: 16777216, // 16 MiB
		MinWidth:                256,
		MinHeight:               256,
		MaxWidth:                8192,
		MaxHeight:               8192,
		MaxPixels:               20000000, // 2000万像素
		AllowedMimeTypes: []string{
			"image/jpeg",
			"image/png",
			"image/webp",
		},
	}
}

func LoadImageLimits(ctx context.Context) ImageLimits {
	limits := DefaultImageLimits()
	val, err := g.Cfg().Get(ctx, "upload")
	if err != nil || val.IsEmpty() {
		return limits
	}
	_ = val.Scan(&limits)

	if limits.MaxRequestBodyBytes <= 0 {
		limits.MaxRequestBodyBytes = 25165824
	}
	if limits.MaxEncodedImageBytes <= 0 {
		limits.MaxEncodedImageBytes = 12582912
	}
	if limits.MaxDecodedImageBytes <= 0 {
		limits.MaxDecodedImageBytes = 8388608
	}
	if limits.MaxCombinedDecodedBytes <= 0 {
		limits.MaxCombinedDecodedBytes = 16777216
	}
	if limits.MinWidth <= 0 {
		limits.MinWidth = 256
	}
	if limits.MinHeight <= 0 {
		limits.MinHeight = 256
	}
	if limits.MaxWidth <= 0 {
		limits.MaxWidth = 8192
	}
	if limits.MaxHeight <= 0 {
		limits.MaxHeight = 8192
	}
	if limits.MaxPixels <= 0 {
		limits.MaxPixels = 20000000
	}
	if len(limits.AllowedMimeTypes) == 0 {
		limits.AllowedMimeTypes = []string{
			"image/jpeg",
			"image/png",
			"image/webp",
		}
	}
	return limits
}
