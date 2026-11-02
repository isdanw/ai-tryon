package v1

import "github.com/gogf/gf/v2/frame/g"

// TryonReq 换装请求
type TryonReq struct {
	g.Meta `path:"/tryon" method:"post" tags:"换装" summary:"AI换装图生成"`
	// 人像照片（全身/半身站立照，base64 或 上传文件）
	PersonImage string `json:"person_image" v:"required" dc:"人像照片，base64编码"`
	// 服装图（base64）
	GarmentImage string `json:"garment_image" v:"required" dc:"服装图，base64编码"`
	// 姿势参考图（可选，base64）：姿势线画图，生成时让人物采用该图的姿势与身体朝向
	PoseImage string `json:"pose_image,omitempty" dc:"姿势参考图，base64编码，可选"`
	// 人物补充信息（可选）：如身高、体型备注，拼入提示词帮助版型贴合
	PersonNote string `json:"person_note,omitempty" dc:"人物补充信息，可选"`
	// 模型选择：gemini / gpt-image / doubao（可在后端 config.yaml 的 llm.models 中扩展），默认 gemini
	Model string `json:"model" d:"gemini" dc:"模型别名：gemini / gpt-image / doubao（可在 config.yaml 扩展）"`
	// 生成尺寸：1024x1024 / 1024x1536 / 1536x1024
	Size string `json:"size" d:"1024x1536" dc:"输出尺寸"`
	// 是否保留人物面部（保持同一个人）
	KeepFace bool `json:"keep_face" d:"true" dc:"是否保持人物面部一致"`
	// 语言：zh / en
	Lang string `json:"lang" d:"zh" dc:"提示词语言"`
	// 浏览器端自定义模型配置。仅用于当前请求，后端不得持久化或记录其中的 Key/Base URL。
	ClientConfig *ClientModelConfig `json:"client_config,omitempty" dc:"当前请求临时使用的用户模型配置"`
}

// ClientModelConfig 用户在浏览器设置中心保存的模型配置。
// 该结构只在一次请求的内存中使用，不写入配置文件、数据库或日志。
type ClientModelConfig struct {
	Provider string `json:"provider" v:"in:openai,doubao,gemini" dc:"协议类型：openai / doubao / gemini"`
	BaseURL  string `json:"base_url" dc:"模型服务 Base URL"`
	APIKey   string `json:"api_key" dc:"模型服务 API Key（敏感，仅本次请求使用）"`
	Model    string `json:"model" dc:"模型服务侧真实模型名"`
}

// TryonRes 换装响应
type TryonRes struct {
	// 生成的换装结果图（base64）
	Image string `json:"image" dc:"换装结果图，base64"`
	// 结果图片的真实 MIME 类型（根据文件签名字节检测）
	MIME string `json:"mime" dc:"结果图片 MIME 类型"`
	// 使用的模型
	Model string `json:"model" dc:"实际使用的模型"`
	// 提示词（调试用，可空）
	Prompt string `json:"prompt,omitempty" dc:"实际使用的提示词"`
}

type ComfyUIHealthReq struct {
	g.Meta `path:"/comfyui/health" method:"get" tags:"换装" summary:"检查本地 ComfyUI 固定工作流"`
}

type ComfyUIHealthRes struct {
	Available bool   `json:"available"`
	Model     string `json:"model,omitempty"`
	Message   string `json:"message"`
}

// TestConnectionReq 设置中心的模型连通性诊断请求。
// api_key 仅在本次请求的内存中使用，不写入配置、日志或任何持久化存储。
type TestConnectionReq struct {
	g.Meta `path:"/test-connection" method:"post" tags:"换装" summary:"测试模型连接"`
	// server：使用服务器预设（config.yaml + 环境变量）；personal：使用浏览器填写的个人配置
	Mode string `json:"mode" dc:"server / personal"`
	// 仅 server 模式：llm.models 下的模型别名
	Alias string `json:"alias" dc:"服务器预设模型别名"`
	// 仅 personal 模式：openai / doubao / gemini / comfyui
	Provider string `json:"provider" dc:"协议类型"`
	BaseURL  string `json:"base_url" dc:"模型服务 Base URL（personal）"`
	APIKey   string `json:"api_key" dc:"模型服务 API Key（personal，敏感，仅本次请求使用）"`
	Model    string `json:"model" dc:"服务商侧真实模型名（personal）"`
}

// TestConnectionRes 诊断结果，绝不回显 API Key、完整 Base URL 或上游原始响应。
type TestConnectionRes struct {
	OK               bool   `json:"ok"`
	Layer            string `json:"layer"`
	Probe            string `json:"probe"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	UpstreamStatus   int    `json:"upstream_status"`
	ElapsedMs        int64  `json:"elapsed_ms"`
	UpstreamRequests int    `json:"upstream_requests"`
	Message          string `json:"message"`
	Hint             string `json:"hint"`
	ErrorCode        string `json:"error_code,omitempty"`
}
