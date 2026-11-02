package imagevalidate

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"backend/internal/security"
)

func createTestPNG(w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func createTestJPEG(w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 50, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestValidateImageValid(t *testing.T) {
	limits := DefaultImageLimits()

	// PNG 300x300
	pngB64 := createTestPNG(300, 300)
	vImg, err := ValidateImage(pngB64, limits)
	if err != nil {
		t.Fatalf("expected valid png to pass: %v", err)
	}
	if vImg.Width != 300 || vImg.Height != 300 {
		t.Fatalf("unexpected dimensions: %dx%d", vImg.Width, vImg.Height)
	}
	if vImg.MIME != "image/png" {
		t.Fatalf("unexpected MIME: %s", vImg.MIME)
	}

	// JPEG with Data URL
	jpegB64 := "data:image/jpeg;base64," + createTestJPEG(300, 300)
	vImg2, err := ValidateImage(jpegB64, limits)
	if err != nil {
		t.Fatalf("expected valid jpeg with data uri to pass: %v", err)
	}
	if vImg2.MIME != "image/jpeg" {
		t.Fatalf("unexpected MIME: %s", vImg2.MIME)
	}

	// WebP
	webpData, err := os.ReadFile("../../../assets/img/SPR-0001.webp")
	if err == nil {
		webpB64 := base64.StdEncoding.EncodeToString(webpData)
		vImg3, err := ValidateImage(webpB64, limits)
		if err != nil {
			t.Fatalf("expected valid webp to pass: %v", err)
		}
		if vImg3.MIME != "image/webp" {
			t.Fatalf("unexpected MIME for webp: %s", vImg3.MIME)
		}
	}
}

func TestValidateImageUnsupportedType(t *testing.T) {
	limits := DefaultImageLimits()

	// 纯文本 base64
	textB64 := base64.StdEncoding.EncodeToString([]byte("hello world this is plain text"))
	_, err := ValidateImage(textB64, limits)
	if err == nil {
		t.Fatal("expected text base64 to fail")
	}
	apiErr, ok := err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "UNSUPPORTED_IMAGE_TYPE" {
		t.Fatalf("expected UNSUPPORTED_IMAGE_TYPE, got %v", err)
	}

	// Data URL 声明与实际内容不符（声明 jpeg 实际是 png）
	pngB64 := "data:image/jpeg;base64," + createTestPNG(300, 300)
	_, err = ValidateImage(pngB64, limits)
	if err == nil {
		t.Fatal("expected mime mismatch to fail")
	}
	apiErr, ok = err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "UNSUPPORTED_IMAGE_TYPE" {
		t.Fatalf("expected UNSUPPORTED_IMAGE_TYPE, got %v", err)
	}
}

func TestValidateImageCorruptedBase64(t *testing.T) {
	limits := DefaultImageLimits()

	_, err := ValidateImage("!!not-valid-base64!!", limits)
	if err == nil {
		t.Fatal("expected corrupted base64 to fail")
	}
	apiErr, ok := err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "INVALID_IMAGE_BASE64" {
		t.Fatalf("expected INVALID_IMAGE_BASE64, got %v", err)
	}
}

func TestValidateImageDimensions(t *testing.T) {
	limits := DefaultImageLimits() // min 256, max 8192

	// 过小: 100x100
	smallB64 := createTestPNG(100, 100)
	_, err := ValidateImage(smallB64, limits)
	if err == nil {
		t.Fatal("expected too small image to fail")
	}
	apiErr, ok := err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "IMAGE_DIMENSIONS_INVALID" {
		t.Fatalf("expected IMAGE_DIMENSIONS_INVALID, got %v", err)
	}
}

func TestValidateImagePixelLimit(t *testing.T) {
	limits := DefaultImageLimits()
	limits.MaxPixels = 100000 // 降低限制进行测试 (10万像素)
	limits.MinWidth = 100
	limits.MinHeight = 100

	// 400x300 = 120,000 > 100,000
	bigPixelsB64 := createTestPNG(400, 300)
	_, err := ValidateImage(bigPixelsB64, limits)
	if err == nil {
		t.Fatal("expected pixel limit to be exceeded")
	}
	apiErr, ok := err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "IMAGE_PIXEL_LIMIT_EXCEEDED" {
		t.Fatalf("expected IMAGE_PIXEL_LIMIT_EXCEEDED, got %v", err)
	}
}

func TestValidateImagesPairCombinedSize(t *testing.T) {
	limits := DefaultImageLimits()
	limits.MaxCombinedDecodedBytes = 1000 // 1000 字节

	img1 := createTestPNG(300, 300)
	img2 := createTestPNG(300, 300)

	_, _, err := ValidateImagesPair(img1, img2, limits)
	if err == nil {
		t.Fatal("expected combined size limit to fail")
	}
	apiErr, ok := err.(*security.APIError)
	if !ok || apiErr.ErrorCode != "IMAGE_FILE_TOO_LARGE" {
		t.Fatalf("expected IMAGE_FILE_TOO_LARGE, got %v", err)
	}
}

func FuzzValidateImage(f *testing.F) {
	// 语料种子
	f.Add("")
	f.Add("hello")
	f.Add("data:image/png;base64,")
	f.Add("data:image/jpeg;base64,12345")
	f.Add(createTestPNG(256, 256))

	limits := DefaultImageLimits()
	f.Fuzz(func(t *testing.T, data string) {
		// 任意输入不应 panic
		_, _ = ValidateImage(data, limits)
	})
}
