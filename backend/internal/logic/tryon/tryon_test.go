package tryon

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	gptZh := BuildGPT("zh")
	if !strings.Contains(gptZh, "虚拟试衣") {
		t.Errorf("BuildGPT(zh) should contain 虚拟试衣, got: %s", gptZh)
	}

	gptEn := BuildGPT("en")
	if !strings.Contains(gptEn, "virtual try-on") {
		t.Errorf("BuildGPT(en) should contain virtual try-on, got: %s", gptEn)
	}

	// Gemini 保留原英文稳定提示词
	geminiPrompt := BuildGemini("zh")
	if !strings.Contains(geminiPrompt, "virtual try-on") || !strings.Contains(geminiPrompt, "Keep the person's identity EXACTLY the same") {
		t.Errorf("BuildGemini should return stable original prompt, got: %s", geminiPrompt)
	}

	// 通用/其他模型（如豆包）保留原中英文提示词
	defaultZh := BuildDefault("zh")
	if !strings.Contains(defaultZh, "你是一名专业的时尚虚拟试衣助手") {
		t.Errorf("BuildDefault(zh) should return original zhPrompt, got: %s", defaultZh)
	}
	defaultEn := BuildDefault("en")
	if !strings.Contains(defaultEn, "You are a professional fashion virtual try-on assistant") {
		t.Errorf("BuildDefault(en) should return original enPrompt, got: %s", defaultEn)
	}

	// GPT-Image 安全提示词验证
	if !strings.Contains(gptEn, "modest neckline") || !strings.Contains(gptEn, "Fully clothed") {
		t.Errorf("gptSafetyEnPrompt missing safety constraints")
	}
	if !strings.Contains(gptZh, "穿着完整端庄") || !strings.Contains(gptZh, "避免过度露肤") {
		t.Errorf("gptSafetyZhPrompt missing safety constraints")
	}

	// isGPTImageModel 识别测试
	if !isGPTImageModel("gpt-image", nil, "gpt-image") {
		t.Errorf("expected gpt-image to be detected as GPT model")
	}
	if !isGPTImageModel("gemini", &ModelConfig{Model: "gpt-image-2"}, "custom") {
		t.Errorf("expected clientConfig gpt-image-2 to be detected as GPT model")
	}
	if isGPTImageModel("doubao", nil, "doubao") {
		t.Errorf("doubao should not be detected as GPT model")
	}
}

func TestForbiddenClientIP(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "::1"} {
		if !isForbiddenClientIP(net.ParseIP(raw)) {
			t.Fatalf("expected %s to be forbidden", raw)
		}
	}
	if isForbiddenClientIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected public IP to be allowed")
	}
}

func TestDecodeB64(t *testing.T) {
	validB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	if err := decodeB64(validB64); err != nil {
		t.Errorf("decodeB64 should succeed for valid base64: %v", err)
	}

	dataUri := "data:image/png;base64," + validB64
	if err := decodeB64(dataUri); err != nil {
		t.Errorf("decodeB64 should succeed for data uri: %v", err)
	}

	invalidB64 := "!!!not-valid-base64!!!"
	if err := decodeB64(invalidB64); err == nil {
		t.Errorf("decodeB64 should fail for invalid base64")
	}
}

func TestDoTryonValidation(t *testing.T) {
	ctx := context.Background()

	// Empty person
	_, _, _, err := DoTryon(ctx, "", "somegarment", "gemini", "1024x1536", "zh")
	if err == nil || !strings.Contains(err.Error(), "人像照片不能为空") {
		t.Errorf("expected empty person error, got: %v", err)
	}

	// Empty garment
	_, _, _, err = DoTryon(ctx, "someperson", "", "gemini", "1024x1536", "zh")
	if err == nil || !strings.Contains(err.Error(), "服装图不能为空") {
		t.Errorf("expected empty garment error, got: %v", err)
	}
}

func TestValidateClientModelConfig(t *testing.T) {
	valid := &ModelConfig{
		Provider: "openai",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.com/v1",
		Model:    "vendor/image-model-v1",
	}
	if err := validateClientModelConfig(valid); err != nil {
		t.Fatalf("expected valid browser config, got: %v", err)
	}

	tests := []struct {
		name string
		cfg  ModelConfig
	}{
		{"missing key", ModelConfig{Provider: "openai", BaseURL: "https://api.example.com/v1", Model: "image-model"}},
		{"unsupported provider", ModelConfig{Provider: "other", APIKey: "key", BaseURL: "https://api.example.com", Model: "image-model"}},
		{"localhost", ModelConfig{Provider: "openai", APIKey: "key", BaseURL: "http://127.0.0.1:8080/v1", Model: "image-model"}},
		{"private network", ModelConfig{Provider: "openai", APIKey: "key", BaseURL: "http://192.168.1.20/v1", Model: "image-model"}},
		{"query in base url", ModelConfig{Provider: "gemini", APIKey: "key", BaseURL: "https://api.example.com/v1?key=bad", Model: "image-model"}},
		{"query in model", ModelConfig{Provider: "gemini", APIKey: "key", BaseURL: "https://api.example.com/v1", Model: "image-model?key=bad"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateClientModelConfig(&tt.cfg); err == nil {
				t.Fatalf("expected config validation error")
			}
		})
	}
}

func TestClientConfigOverridesTopLevelModel(t *testing.T) {
	validB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	clientConfig := &ModelConfig{
		Provider: "unsupported",
		APIKey:   "test-key",
		BaseURL:  "https://api.example.com/v1",
		Model:    "gpt-image-2",
	}

	_, _, _, err := DoTryonWithClientConfig(
		context.Background(), validB64, validB64, "gemini", "1024x1536", "zh", clientConfig,
	)
	if err == nil || !strings.Contains(err.Error(), "不支持的协议类型") {
		t.Fatalf("expected client config validation to override the top-level gemini model, got: %v", err)
	}
}

func TestOpenAIImagesVisualFallback(t *testing.T) {
	cfg := &ModelConfig{
		Name:     "gpt-image",
		Provider: "openai",
		APIKey:   "dummy-key",
		BaseURL:  "https://127.0.0.1:9999/v1", // Unreachable URL to test graceful fallback
		Model:    "gpt-image-2",
	}
	adapter := NewOpenAIImages(cfg)
	if adapter == nil {
		t.Fatalf("expected NewOpenAIImages to return adapter")
	}

	// describeVisualFeatures should gracefully return empty string when unreachable, not panic
	desc := adapter.describeVisualFeatures(context.Background(), "invalid-b64", "invalid-b64")
	if desc != "" {
		t.Errorf("expected empty description on network failure, got: %s", desc)
	}
}
