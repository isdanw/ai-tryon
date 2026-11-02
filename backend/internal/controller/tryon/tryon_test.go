package tryon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"backend/api/tryon/v1"
	"backend/internal/middleware"
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

func TestControllerTryonImageValidation(t *testing.T) {
	// 启动测试服务
	s := g.Server("test-ctrl-tryon")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	// 1. 无效 Base64
	badReq := v1.TryonReq{
		PersonImage:  "!!not-valid-base64!!",
		GarmentImage: createTestPNG(300, 300),
	}
	badBody, _ := json.Marshal(badReq)
	req1, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(badBody))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp1.StatusCode)
	}
	b1, _ := io.ReadAll(resp1.Body)
	var res1 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b1, &res1)
	if res1.Code != 42201 || res1.Data.ErrorCode != "INVALID_IMAGE_BASE64" {
		t.Fatalf("unexpected res1: %+v", res1)
	}

	// 2. 文本图片（不支持类型）
	textB64 := base64.StdEncoding.EncodeToString([]byte("plain text not image"))
	textReq := v1.TryonReq{
		PersonImage:  textB64,
		GarmentImage: createTestPNG(300, 300),
	}
	textBody, _ := json.Marshal(textReq)
	req2, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(textBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp2.StatusCode)
	}
	b2, _ := io.ReadAll(resp2.Body)
	var res2 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b2, &res2)
	if res2.Code != 42202 || res2.Data.ErrorCode != "UNSUPPORTED_IMAGE_TYPE" {
		t.Fatalf("unexpected res2: %+v", res2)
	}

	// 3. 尺寸过小
	smallB64 := createTestPNG(100, 100)
	smallReq := v1.TryonReq{
		PersonImage:  smallB64,
		GarmentImage: createTestPNG(300, 300),
	}
	smallBody, _ := json.Marshal(smallReq)
	req3, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(smallBody))
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp3.StatusCode)
	}
	b3, _ := io.ReadAll(resp3.Body)
	var res3 struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b3, &res3)
	if res3.Code != 42204 || res3.Data.ErrorCode != "IMAGE_DIMENSIONS_INVALID" {
		t.Fatalf("unexpected res3: %+v", res3)
	}
}

func TestControllerTryonQuotaAndConcurrency(t *testing.T) {
	secService := security.GetService()
	// 设置配额为 1 进行测试
	secService.DailyQuotaTracker = security.NewDailyQuotaTracker(1)
	secService.ConcurrencyManager = security.NewConcurrencyManager(1, 1)

	s := g.Server("test-ctrl-quota")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	validPerson := createTestPNG(300, 300)
	validGarment := createTestPNG(300, 300)

	// 先直接耗尽配额
	secService.DailyQuotaTracker.CheckAndDeduct("anonymous")

	// 发起请求，应当直接触发 DAILY_QUOTA_EXCEEDED (42902)
	tryReq := v1.TryonReq{
		PersonImage:  validPerson,
		GarmentImage: validGarment,
	}
	reqBody, _ := json.Marshal(tryReq)
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/tryon", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	var res struct {
		Code int `json:"code"`
		Data struct {
			ErrorCode string `json:"error_code"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &res)
	if res.Code != 42902 || res.Data.ErrorCode != "DAILY_QUOTA_EXCEEDED" {
		t.Fatalf("unexpected quota res: %+v", res)
	}
}

func TestControllerTestConnectionConfigMissing(t *testing.T) {
	s := g.Server("test-ctrl-testconn")
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.RequestID, ghttp.MiddlewareHandlerResponse)
		group.Bind(NewV1())
	})
	s.SetPort(0)
	_ = s.Start()
	defer s.Shutdown()

	port := strings.Split(s.GetListenedAddress(), ":")[1]
	client := &http.Client{}

	cases := []struct {
		name string
		body string
	}{
		{"missing mode", `{}`},
		{"missing alias", `{"mode":"server"}`},
		{"mode type mismatch", `{"mode":123}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/test-connection", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			var res struct {
				Code int                  `json:"code"`
				Data v1.TestConnectionRes `json:"data"`
			}
			if err := json.Unmarshal(body, &res); err != nil {
				t.Fatal(err)
			}
			if res.Code != 0 {
				t.Fatalf("expected code 0, got %d (%s)", res.Code, body)
			}
			if res.Data.Layer != "config_missing" {
				t.Fatalf("expected config_missing, got %q (%s)", res.Data.Layer, body)
			}
			if res.Data.Probe != "none" || res.Data.UpstreamRequests != 0 || res.Data.UpstreamStatus != 0 {
				t.Fatalf("expected zero upstream activity, got %+v", res.Data)
			}
		})
	}
}

func init() {
	// 确保测试环境未设置任何破坏性环境变量
	os.Unsetenv("TRYON_API_KEYS")
}
