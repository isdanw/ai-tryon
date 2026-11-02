package tryon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	probeLayerConfigMissing = "config_missing"
	probeLayerUnreachable   = "unreachable"
	probeLayerAuth          = "auth"
	probeLayerForbidden     = "forbidden"
	probeLayerModel         = "model"
	probeLayerTimeout       = "timeout"
	probeLayerUpstreamError = "upstream_error"
	probeLayerOK            = "ok"

	probeKindNone     = "none"
	probeKindReadonly = "readonly"
	probeKindGenerate = "generate"
	probeKindComfyUI  = "comfyui"

	defaultProbeTimeout         = 15 * time.Second
	defaultProbeGenerateTimeout = 60 * time.Second
	probeMaxResponseBytes       = 1 << 20
)

var probeErrorCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{0,64}$`)

// ProbeOptions 描述一次连接诊断所需的最小信息。
// APIKey 只在本次调用的内存中使用，不会写入配置、日志或任何持久化存储。
type ProbeOptions struct {
	Mode             string        // server / personal
	Provider         string        // 协议类型或别名
	BaseURL          string        // 模型服务 Base URL
	APIKey           string        // 敏感信息，仅内存使用
	Model            string        // 服务商侧真实模型名
	EnvKey           string        // server 模式下 Key 来源的环境变量名（用于提示文案）
	ClientRestricted bool          // personal 模式启用内网/保留地址限制（与 /tryon 一致）
	AllowGenerate    bool          // 是否允许走会消耗额度的生图探针
	Timeout          time.Duration // 只读探针超时，默认 15s
	GenerateTimeout  time.Duration // 生图探针超时，默认 60s
}

// ProbeResult 诊断结果，序列化时不含 API Key、完整 Base URL 或上游原始响应体。
type ProbeResult struct {
	OK               bool
	Layer            string
	Probe            string
	Provider         string
	Model            string
	UpstreamStatus   int
	ElapsedMs        int64
	UpstreamRequests int
	Message          string
	Hint             string
	ErrorCode        string
}

// NewProbeConfigMissing 构造配置缺失结果：本层失败不会发出任何上游请求。
func NewProbeConfigMissing(provider, model, message, hint string) ProbeResult {
	return ProbeResult{
		Layer:    probeLayerConfigMissing,
		Probe:    probeKindNone,
		Provider: provider,
		Model:    model,
		Message:  message,
		Hint:     hint,
	}
}

// LoadModelConfigForProbe 加载模型配置但不因 Key 缺失而报错，供诊断接口判断配置完整性。
// 行为与 loadModelConfig 保持一致：envKey 指向的环境变量优先，其次 apiKey 配置；没有任何条目时回退 legacy 配置。
func LoadModelConfigForProbe(ctx context.Context, alias string) (*ModelConfig, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, fmt.Errorf("模型别名未配置")
	}
	prefix := "llm.models." + alias
	rawProvider := g.Cfg().MustGet(ctx, prefix+".provider", "").String()
	cfg := &ModelConfig{
		Name:     alias,
		Provider: rawProvider,
		APIKey:   g.Cfg().MustGet(ctx, prefix+".apiKey", "").String(),
		BaseURL:  g.Cfg().MustGet(ctx, prefix+".baseURL", "").String(),
		Model:    g.Cfg().MustGet(ctx, prefix+".model", "").String(),
		EnvKey:   g.Cfg().MustGet(ctx, prefix+".envKey", "").String(),
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = "openai"
	}

	if cfg.EnvKey != "" {
		if v := lookupEnv(cfg.EnvKey); v != "" {
			cfg.APIKey = v
		}
	}

	if strings.TrimSpace(rawProvider) == "" && cfg.Model == "" && cfg.BaseURL == "" && cfg.APIKey == "" {
		cfg = loadLegacyConfig(ctx, alias)
	}
	if cfg == nil {
		return nil, fmt.Errorf("模型别名未配置")
	}
	return cfg, nil
}

// CanonicalProbeProvider 将配置中的协议别名归一化为 openai / doubao / gemini / comfyui。
func CanonicalProbeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "doubao", "ark", "volc", "volcengine", "huoshan":
		return "doubao"
	case "openai", "qwen", "wanx", "zhipu", "siliconflow", "openai-compatible":
		return "openai"
	case "gemini", "google":
		return "gemini"
	case "comfyui":
		return "comfyui"
	default:
		return ""
	}
}

// TestConnection 依次执行配置完整性检查和上游探针，任何配置缺失都不会创建 HTTP 客户端。
func TestConnection(ctx context.Context, opts ProbeOptions) ProbeResult {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	rawProvider := strings.ToLower(strings.TrimSpace(opts.Provider))
	model := strings.TrimSpace(opts.Model)

	if mode != "server" && mode != "personal" {
		return NewProbeConfigMissing(rawProvider, model, "mode 必须为 server 或 personal", "请选择服务器预设或个人配置后重试")
	}

	provider := CanonicalProbeProvider(rawProvider)
	if provider == "" {
		return NewProbeConfigMissing(rawProvider, model, "不支持的协议类型", "仅支持 openai、doubao、gemini、comfyui")
	}
	if mode == "personal" && !isPersonalProbeProvider(rawProvider) {
		return NewProbeConfigMissing(rawProvider, model, "不支持的协议类型", "仅支持 openai、doubao、gemini、comfyui")
	}
	if provider == "comfyui" {
		return ProbeComfyUIConnection(ctx)
	}

	opts.Mode = mode
	opts.Provider = provider
	opts.BaseURL = strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	opts.APIKey = strings.TrimSpace(opts.APIKey)
	opts.Model = model

	if mode == "personal" {
		clean := &ModelConfig{
			Provider: provider,
			APIKey:   opts.APIKey,
			BaseURL:  opts.BaseURL,
			Model:    opts.Model,
		}
		if err := validateClientModelConfig(clean); err != nil {
			return NewProbeConfigMissing(provider, model, err.Error(), "请检查 Base URL、API Key 与模型名后重试")
		}
		opts.ClientRestricted = true
	}

	if isPlaceholderProbeKey(opts.APIKey) {
		if mode == "personal" {
			return NewProbeConfigMissing(provider, model, "API Key 仍是占位符，请填写有效 Key，本次没有发出任何上游请求", "请填写该服务商的真实 API Key")
		}
		return NewProbeConfigMissing(provider, model, probeKeyMissingMessage(opts.EnvKey, opts.APIKey), "请在后端环境变量或设置中心填写有效 Key")
	}
	if opts.BaseURL == "" {
		return NewProbeConfigMissing(provider, model, "模型服务 Base URL 未配置", "请填写 Base URL（通常以 /v1 或 /api/v3 结尾）")
	}
	if opts.Model == "" {
		return NewProbeConfigMissing(provider, model, "模型名未配置", "请填写服务商侧真实模型名")
	}

	client := newModelHTTPClient(opts.ClientRestricted)
	switch provider {
	case "gemini":
		return probeGemini(ctx, opts, client)
	case "openai":
		return probeOpenAIModels(ctx, opts, client)
	case "doubao":
		return probeDoubao(ctx, opts, client)
	default:
		return NewProbeConfigMissing(provider, model, "不支持的协议类型", "仅支持 openai、doubao、gemini、comfyui")
	}
}

// ProbeComfyUIConnection 复用现有 ComfyUI 健康检查，不执行推理。
func ProbeComfyUIConnection(ctx context.Context) ProbeResult {
	model, err := CheckComfyUI(ctx)
	if err != nil {
		return ProbeResult{
			Layer:            probeLayerUnreachable,
			Probe:            probeKindComfyUI,
			Provider:         "comfyui",
			UpstreamRequests: 1,
			Message:          "本地 ComfyUI 或固定工作流不可用",
			Hint:             "请确认 ComfyUI 已启动，且固定工作流与节点配置正确",
		}
	}
	return ProbeResult{
		OK:               true,
		Layer:            probeLayerOK,
		Probe:            probeKindComfyUI,
		Provider:         "comfyui",
		Model:            model,
		UpstreamRequests: 1,
		Message:          "本地 ComfyUI 固定工作流可用",
	}
}

func isPersonalProbeProvider(provider string) bool {
	switch provider {
	case "openai", "doubao", "gemini", "comfyui":
		return true
	default:
		return false
	}
}

func isPlaceholderProbeKey(key string) bool {
	key = strings.TrimSpace(key)
	return key == "" || key == "__FILL_BY_USER__" || strings.HasPrefix(key, "__FILL_")
}

func probeKeyMissingMessage(envKey, key string) string {
	envKey = strings.TrimSpace(envKey)
	if envKey != "" {
		if strings.TrimSpace(key) == "" {
			return fmt.Sprintf("后端尚未配置 %s，本次没有发出任何上游请求", envKey)
		}
		return fmt.Sprintf("后端尚未配置 %s（仍是占位符），本次没有发出任何上游请求", envKey)
	}
	if strings.TrimSpace(key) == "" {
		return "后端尚未配置 API Key，本次没有发出任何上游请求"
	}
	return "后端尚未配置 API Key（仍是占位符），本次没有发出任何上游请求"
}

func probeTimeoutOr(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func probeRequestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, probeTimeoutOr(timeout, defaultProbeTimeout))
}

func readProbeBody(body io.Reader) []byte {
	data, _ := io.ReadAll(io.LimitReader(body, probeMaxResponseBytes))
	return data
}

func okProbeResult(res ProbeResult, message, hint string) ProbeResult {
	res.OK = true
	res.Layer = probeLayerOK
	res.Message = message
	res.Hint = hint
	return res
}

func typedProbeResult(res ProbeResult, layer, message, hint string) ProbeResult {
	res.Layer = layer
	res.Message = message
	res.Hint = hint
	return res
}

func networkProbeFailure(res ProbeResult, err error) ProbeResult {
	if errors.Is(err, context.DeadlineExceeded) {
		return typedProbeResult(res, probeLayerTimeout, "连接模型服务超时，未收到响应", "请检查网络连通性或稍后重试")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return typedProbeResult(res, probeLayerTimeout, "连接模型服务超时，未收到响应", "请检查网络连通性或稍后重试")
	}
	return typedProbeResult(res, probeLayerUnreachable, "无法连接模型服务，请检查 Base URL 和网络", "确认地址可访问；OpenAI 兼容地址通常以 /v1 结尾，火山方舟通常以 /api/v3 结尾")
}

func sanitizeProbeErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if !probeErrorCodePattern.MatchString(code) {
		return ""
	}
	return code
}

func extractProbeErrorCode(body []byte) string {
	var env struct {
		Error struct {
			Code   interface{} `json:"code"`
			Status string      `json:"status"`
			Type   string      `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	candidates := make([]string, 0, 4)
	candidates = append(candidates, env.Error.Status, env.Error.Type)
	switch code := env.Error.Code.(type) {
	case string:
		candidates = append(candidates, code)
	case float64:
		candidates = append(candidates, strconv.FormatInt(int64(code), 10))
	}
	for _, candidate := range candidates {
		if sanitized := sanitizeProbeErrorCode(candidate); sanitized != "" {
			return sanitized
		}
	}
	return ""
}

func probeContainsModelKeyword(body []byte) bool {
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	text := strings.ToLower(env.Error.Code + " " + env.Error.Message)
	for _, keyword := range []string{"model", "not found", "not activated", "未开通", "不存在"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// probeOpenAIModels 只读探针：GET {base}/models，按大小写敏感精确匹配模型名。
func probeOpenAIModels(ctx context.Context, opts ProbeOptions, client *http.Client) ProbeResult {
	res := ProbeResult{Provider: "openai", Model: opts.Model, Probe: probeKindReadonly}
	reqCtx, cancel := probeRequestContext(ctx, opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, opts.BaseURL+"/models", nil)
	if err != nil {
		return NewProbeConfigMissing("openai", opts.Model, "Base URL 格式不正确", "请检查 Base URL 后重试")
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Accept", "application/json")

	res.UpstreamRequests = 1
	resp, err := client.Do(req)
	if err != nil {
		return networkProbeFailure(res, err)
	}
	defer resp.Body.Close()
	res.UpstreamStatus = resp.StatusCode
	body := readProbeBody(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			res.ErrorCode = extractProbeErrorCode(body)
			return typedProbeResult(res, probeLayerUpstreamError, "模型服务返回的模型列表无法解析", "请确认 Base URL 指向兼容 /models 的接口")
		}
		for _, item := range list.Data {
			if item.ID == opts.Model {
				return okProbeResult(res, "连接正常，模型可用", "")
			}
		}
		return typedProbeResult(res, probeLayerModel, "模型不在当前 Key 的可用模型列表中：模型名可能写错，或该模型未开通", "请核对模型名，或在服务商控制台开通该模型")
	case http.StatusUnauthorized:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerAuth, "API Key 无效或与当前服务商不匹配（401）", "请确认 Key 属于该服务商；火山方舟 Plan Key 与后付费 Key 不通用")
	case http.StatusForbidden:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerForbidden, "API Key 有效但无权访问该服务（403）", "请在服务商控制台确认模型权限，或更换有权限的 Key")
	case http.StatusNotFound:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerUpstreamError, "Base URL 路径可能不正确：/models 返回 404", "OpenAI 兼容地址通常以 /v1 结尾，请检查 Base URL")
	case http.StatusTooManyRequests:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerUpstreamError, "模型服务限流或额度不足（429）", "请稍后重试，或检查账户额度")
	default:
		res.ErrorCode = extractProbeErrorCode(body)
		if resp.StatusCode >= 500 {
			return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("模型服务暂时异常（status %d）", resp.StatusCode), "请稍后重试")
		}
		return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("模型服务拒绝了请求（status %d）", resp.StatusCode), "请检查 Base URL、Key 或模型权限")
	}
}

type probeGenerateRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Size           string `json:"size"`
	ResponseFormat string `json:"response_format"`
	Watermark      bool   `json:"watermark"`
}

type probeImageResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

// probeDoubao 先尝试只读 /models；若基址不支持只读探针，再走一次最小生图。
func probeDoubao(ctx context.Context, opts ProbeOptions, client *http.Client) ProbeResult {
	res := ProbeResult{Provider: "doubao", Model: opts.Model, Probe: probeKindReadonly}
	reqCtx, cancel := probeRequestContext(ctx, opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, opts.BaseURL+"/models", nil)
	if err != nil {
		return NewProbeConfigMissing("doubao", opts.Model, "Base URL 格式不正确", "请检查 Base URL 后重试")
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Accept", "application/json")

	res.UpstreamRequests = 1
	resp, err := client.Do(req)
	if err != nil {
		return networkProbeFailure(res, err)
	}
	defer resp.Body.Close()
	res.UpstreamStatus = resp.StatusCode
	body := readProbeBody(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &list); err == nil {
			for _, item := range list.Data {
				if item.ID == opts.Model {
					return okProbeResult(res, "连接正常，模型可用", "")
				}
			}
			return typedProbeResult(res, probeLayerModel, "模型不在当前 Key 的可用模型列表中：模型名可能写错，或该模型未开通", "请核对模型名，或在火山方舟控制台开通该模型")
		}
		// 返回体不是标准模型列表时，继续用生图探针确认。
	case http.StatusUnauthorized:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerAuth, "API Key 无效或与当前服务商不匹配（401）", "请确认 Key 属于火山方舟；Plan Key 与后付费 Key 不通用")
	case http.StatusForbidden:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerForbidden, "API Key 有效但无权访问该模型（403）", "请在火山方舟控制台开通该模型，或更换有权限的 Key")
	}

	return probeDoubaoGenerate(ctx, opts, client, res)
}

func probeDoubaoGenerate(ctx context.Context, opts ProbeOptions, client *http.Client, res ProbeResult) ProbeResult {
	if opts.Mode == "server" && !opts.AllowGenerate {
		// 安全闸：服务器预设的生图探针只允许本机页面发起，避免远程页面消耗本机 Key 的额度。
		res.Layer = probeLayerForbidden
		res.Probe = probeKindGenerate
		res.UpstreamStatus = 0
		res.UpstreamRequests = 0
		res.Message = "为避免消耗本机服务器 Key 的额度，服务器预设的生图探针仅允许本机页面发起"
		res.Hint = "请在运行后端的同一台机器上用浏览器打开设置中心测试"
		return res
	}

	res.Probe = probeKindGenerate
	payload := probeGenerateRequest{
		Model:          opts.Model,
		Prompt:         "ping",
		Size:           "1024x1024",
		ResponseFormat: "url",
		Watermark:      false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return NewProbeConfigMissing("doubao", opts.Model, "生图探针请求构造失败", "请稍后重试")
	}

	genTimeout := opts.GenerateTimeout
	if genTimeout <= 0 {
		genTimeout = defaultProbeGenerateTimeout
	}
	reqCtx, cancel := probeRequestContext(ctx, genTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, opts.BaseURL+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return NewProbeConfigMissing("doubao", opts.Model, "Base URL 格式不正确", "请检查 Base URL 后重试")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)

	res.UpstreamRequests++
	res.UpstreamStatus = 0
	resp, err := client.Do(req)
	if err != nil {
		return networkProbeFailure(res, err)
	}
	defer resp.Body.Close()
	res.UpstreamStatus = resp.StatusCode
	respBody := readProbeBody(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var result probeImageResponse
		if err := json.Unmarshal(respBody, &result); err != nil || len(result.Data) == 0 {
			res.ErrorCode = extractProbeErrorCode(respBody)
			return typedProbeResult(res, probeLayerUpstreamError, "生图探针未返回有效图片数据", "请确认该地址兼容 /images/generations")
		}
		for _, item := range result.Data {
			if item.URL != "" || item.B64JSON != "" {
				return okProbeResult(res, "已用一次最小生图验证连通，模型可用", "")
			}
		}
		res.ErrorCode = extractProbeErrorCode(respBody)
		return typedProbeResult(res, probeLayerUpstreamError, "生图探针未返回有效图片数据", "请确认该地址兼容 /images/generations")
	}

	res.ErrorCode = extractProbeErrorCode(respBody)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return typedProbeResult(res, probeLayerAuth, "API Key 无效或与当前服务商不匹配（401）", "请确认 Key 属于火山方舟；Plan Key 与后付费 Key 不通用")
	case http.StatusForbidden:
		return typedProbeResult(res, probeLayerForbidden, "API Key 有效但无权访问该模型（403）", "请在火山方舟控制台开通该模型，或更换有权限的 Key")
	case http.StatusNotFound:
		return typedProbeResult(res, probeLayerModel, "模型不存在或未开通", "请核对模型名，或在火山方舟控制台开通该模型")
	case http.StatusBadRequest:
		if probeContainsModelKeyword(respBody) {
			return typedProbeResult(res, probeLayerModel, "模型不存在或未开通", "请核对模型名，或在火山方舟控制台开通该模型")
		}
		return typedProbeResult(res, probeLayerUpstreamError, "模型服务拒绝了生图请求（400）", "请检查模型名、尺寸等参数是否被该服务支持")
	case http.StatusTooManyRequests:
		return typedProbeResult(res, probeLayerUpstreamError, "模型服务限流或额度不足（429）", "请稍后重试，或检查账户额度")
	default:
		if resp.StatusCode >= 500 {
			return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("模型服务暂时异常（status %d）", resp.StatusCode), "请稍后重试")
		}
		return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("模型服务拒绝了生图请求（status %d）", resp.StatusCode), "请检查 Base URL、Key 或模型权限")
	}
}

// probeGemini 使用官方只读接口：先 /models 验证 Key，再 /models/{model} 验证模型。
func probeGemini(ctx context.Context, opts ProbeOptions, client *http.Client) ProbeResult {
	res := ProbeResult{Provider: "gemini", Model: opts.Model, Probe: probeKindReadonly}
	reqCtx, cancel := probeRequestContext(ctx, opts.Timeout)
	defer cancel()

	listURL := opts.BaseURL + "/models?key=" + url.QueryEscape(opts.APIKey)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, listURL, nil)
	if err != nil {
		return NewProbeConfigMissing("gemini", opts.Model, "Base URL 格式不正确", "请检查 Base URL 后重试")
	}
	req.Header.Set("Accept", "application/json")

	res.UpstreamRequests = 1
	resp, err := client.Do(req)
	if err != nil {
		return networkProbeFailure(res, err)
	}
	body := readProbeBody(resp.Body)
	resp.Body.Close()
	res.UpstreamStatus = resp.StatusCode

	switch resp.StatusCode {
	case http.StatusOK:
		// 继续验证目标模型是否存在。
	case http.StatusBadRequest, http.StatusUnauthorized:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerAuth, "Gemini API Key 无效或请求参数不被接受（400/401）", "请确认 Key 属于 Gemini，并检查 Base URL 是否为 /v1beta")
	case http.StatusForbidden:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerForbidden, "Gemini API Key 有效但无权访问该服务（403）", "请确认项目已启用 Generative Language API")
	case http.StatusNotFound:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerUpstreamError, "Base URL 路径可能不正确：/models 返回 404", "Gemini 官方地址通常以 /v1beta 结尾")
	case http.StatusTooManyRequests:
		res.ErrorCode = extractProbeErrorCode(body)
		return typedProbeResult(res, probeLayerUpstreamError, "Gemini 限流或额度不足（429）", "请稍后重试，或检查账户额度")
	default:
		res.ErrorCode = extractProbeErrorCode(body)
		if resp.StatusCode >= 500 {
			return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("Gemini 服务暂时异常（status %d）", resp.StatusCode), "请稍后重试")
		}
		return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("Gemini 拒绝了请求（status %d）", resp.StatusCode), "请检查 Base URL、Key 或模型权限")
	}

	modelCtx, modelCancel := probeRequestContext(ctx, opts.Timeout)
	defer modelCancel()
	modelURL := opts.BaseURL + "/models/" + url.PathEscape(opts.Model) + "?key=" + url.QueryEscape(opts.APIKey)
	modelReq, err := http.NewRequestWithContext(modelCtx, http.MethodGet, modelURL, nil)
	if err != nil {
		return NewProbeConfigMissing("gemini", opts.Model, "Base URL 格式不正确", "请检查 Base URL 后重试")
	}
	modelReq.Header.Set("Accept", "application/json")

	res.UpstreamRequests++
	res.UpstreamStatus = 0
	modelResp, err := client.Do(modelReq)
	if err != nil {
		return networkProbeFailure(res, err)
	}
	defer modelResp.Body.Close()
	res.UpstreamStatus = modelResp.StatusCode
	modelBody := readProbeBody(modelResp.Body)

	switch modelResp.StatusCode {
	case http.StatusOK:
		return okProbeResult(res, "连接正常，模型可用", "")
	case http.StatusNotFound:
		res.ErrorCode = extractProbeErrorCode(modelBody)
		return typedProbeResult(res, probeLayerModel, "模型不存在或未开通", "请核对模型名，或在 Gemini 控制台确认该模型可用")
	case http.StatusBadRequest, http.StatusUnauthorized:
		res.ErrorCode = extractProbeErrorCode(modelBody)
		return typedProbeResult(res, probeLayerAuth, "Gemini API Key 无效或请求参数不被接受（400/401）", "请确认 Key 属于 Gemini，并检查模型名")
	case http.StatusForbidden:
		res.ErrorCode = extractProbeErrorCode(modelBody)
		return typedProbeResult(res, probeLayerForbidden, "Gemini API Key 有效但无权访问该模型（403）", "请确认项目已启用该模型")
	case http.StatusTooManyRequests:
		res.ErrorCode = extractProbeErrorCode(modelBody)
		return typedProbeResult(res, probeLayerUpstreamError, "Gemini 限流或额度不足（429）", "请稍后重试，或检查账户额度")
	default:
		res.ErrorCode = extractProbeErrorCode(modelBody)
		if modelResp.StatusCode >= 500 {
			return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("Gemini 服务暂时异常（status %d）", modelResp.StatusCode), "请稍后重试")
		}
		return typedProbeResult(res, probeLayerUpstreamError, fmt.Sprintf("Gemini 拒绝了请求（status %d）", modelResp.StatusCode), "请检查 Base URL、Key 或模型权限")
	}
}
