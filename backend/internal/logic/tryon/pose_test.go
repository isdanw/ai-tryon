package tryon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 豆包带姿势时必须按顺序发送三张参考图：人物、服装、姿势
func TestDoubaoGenerateWithPoseSendsThreeImages(t *testing.T) {
	var gotImages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Image []string `json:"image"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotImages = body.Image
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer server.Close()

	gen := NewDoubao(&ModelConfig{Name: "doubao", APIKey: "test-key", BaseURL: server.URL, Model: "test-model"})
	result, err := gen.GenerateWithPose(context.Background(), "cGVyc29u", "Z2FybWVudA==", "cG9zZQ==", "prompt", "1024x1536")
	if err != nil {
		t.Fatalf("GenerateWithPose returned error: %v", err)
	}
	if result != "aW1hZ2U=" {
		t.Fatalf("unexpected result: %q", result)
	}
	if len(gotImages) != 3 {
		t.Fatalf("expected 3 reference images, got %d", len(gotImages))
	}
	wantSuffix := []string{"cGVyc29u", "Z2FybWVudA==", "cG9zZQ=="}
	for i, suffix := range wantSuffix {
		if !strings.HasSuffix(gotImages[i], suffix) {
			t.Fatalf("image %d should end with %q, got %q", i, suffix, gotImages[i])
		}
	}

	// 不带姿势的旧路径仍是两张图
	gotImages = nil
	if _, err := gen.Generate(context.Background(), "cGVyc29u", "Z2FybWVudA==", "prompt", "1024x1536"); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(gotImages) != 2 {
		t.Fatalf("expected 2 reference images without pose, got %d", len(gotImages))
	}
}

// Gemini 带姿势时 parts 应为 文本 + 三张图
func TestGeminiGenerateWithPoseSendsThreeImageParts(t *testing.T) {
	var partCount, imageParts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Contents []struct {
				Parts []struct {
					Text       string `json:"text,omitempty"`
					InlineData *struct {
						Data string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		partCount = len(body.Contents[0].Parts)
		for _, p := range body.Contents[0].Parts {
			if p.InlineData != nil {
				imageParts++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`))
	}))
	defer server.Close()

	gen := NewGemini(&ModelConfig{Name: "gemini", APIKey: "test-key", BaseURL: server.URL, Model: "test-model"})
	if _, err := gen.GenerateWithPose(context.Background(), "cGVyc29u", "Z2FybWVudA==", "cG9zZQ==", "prompt", "1024x1536"); err != nil {
		t.Fatalf("GenerateWithPose returned error: %v", err)
	}
	if partCount != 4 || imageParts != 3 {
		t.Fatalf("expected 4 parts with 3 images, got %d parts / %d images", partCount, imageParts)
	}
}

// 姿势能力矩阵：云端三家支持，ComfyUI 固定工作流不支持（应被逻辑层明确拒绝）
func TestPoseCapabilityMatrix(t *testing.T) {
	if _, ok := interface{}(NewDoubao(&ModelConfig{})).(PoseImageGenerator); !ok {
		t.Fatal("Doubao must implement PoseImageGenerator")
	}
	if _, ok := interface{}(NewGemini(&ModelConfig{})).(PoseImageGenerator); !ok {
		t.Fatal("Gemini must implement PoseImageGenerator")
	}
	if _, ok := interface{}(NewOpenAIImages(&ModelConfig{})).(PoseImageGenerator); !ok {
		t.Fatal("OpenAIImages must implement PoseImageGenerator")
	}
	if _, ok := interface{}(NewComfyUI(&ModelConfig{})).(PoseImageGenerator); ok {
		t.Fatal("ComfyUI fixed workflow must NOT implement PoseImageGenerator")
	}
}

func TestAppendPoseInstructionAndNote(t *testing.T) {
	zh := AppendPoseInstruction("基础提示词", "zh")
	if !strings.Contains(zh, "第三张") || !strings.HasPrefix(zh, "基础提示词") {
		t.Fatalf("zh pose instruction wrong: %q", zh)
	}
	en := AppendPoseInstruction("base prompt", "en")
	if !strings.Contains(en, "third image") {
		t.Fatalf("en pose instruction wrong: %q", en)
	}
	if got := AppendPersonNote("p", ""); got != "p" {
		t.Fatalf("empty note must not change prompt, got %q", got)
	}
	got := AppendPersonNote("p", "身高 165cm")
	if !strings.Contains(got, "身高 165cm") {
		t.Fatalf("note not appended: %q", got)
	}
	long := AppendPersonNote("p", strings.Repeat("高", 300))
	if len([]rune(long)) > 260 {
		t.Fatalf("note must be truncated to 200 runes, got %d runes", len([]rune(long)))
	}
}
