# AI 换装接口保护与图片校验功能需求

> 文档日期：2026-10-30  
> 适用项目：`nvzhuang`  
> 需求范围：功能 1「接口鉴权、限流和费用保护」与功能 3「图片校验和资源限制」  
> 实现方式：保留现有同步 `POST /tryon`，不引入异步任务系统

## 1. 需求概述

当前 AI 换装接口能够直接触发云端付费模型或本地 ComfyUI 推理，但后端没有独立的访问鉴权、调用额度和并发保护。图片输入目前主要验证 Base64 是否可解码，没有严格限制真实图片格式、文件大小、图片尺寸和像素总量。

本次只实现以下两个功能：

1. 为 `/tryon` 增加后端访问 Token、CORS 白名单、调用频率、每日额度和并发限制。
2. 为人像和服装图增加请求体大小、图片格式、文件大小、宽高及总像素校验，并在前端上传时压缩图片。

实现后，未授权、超额、超限或无效图片请求必须在调用任何模型服务前被拒绝，避免模型费用损失和服务器资源被异常占用。

## 2. 范围与非目标

本需求包含：

- `POST /tryon` 的 Bearer Token 鉴权。
- 严格 CORS Origin 白名单。
- 每个调用方的速率限制、每日调用额度和并发限制。
- 前端设置中心增加后端访问 Token。
- 请求体大小限制。
- JPEG、PNG、WebP 图片真实性校验。
- 图片文件大小、宽高和总像素限制。
- 前端图片压缩和错误提示。
- 稳定、可识别的业务错误码。
- 对应的后端自动化测试。

本需求不包含：

- 异步任务、任务队列、任务进度、取消任务。
- Redis、数据库或多实例共享额度。
- 用户注册、OAuth、会员系统或付费计费。
- GitHub Actions、Prometheus、完整监控系统。
- 前端 `index.html` 模块化重构。
- 修改模型提示词、模型选择逻辑或 ComfyUI 工作流。

## 3. 当前依据

### 3.1 Evidence

| ID | 代码位置 | 当前观察 | 复核命令 |
|---|---|---|---|
| E-001 | `backend/internal/cmd/cmd.go` | 当前只注册响应和默认 CORS 中间件，没有访问鉴权和限流 | `rg -n "Middleware|CORSDefault" backend/internal/cmd/cmd.go` |
| E-002 | `backend/api/tryon/v1/tryon.go` | `/tryon` 可以直接接收两张 Base64 图片并触发生成 | `rg -n "PersonImage|GarmentImage|path:\"/tryon\"" backend/api/tryon/v1/tryon.go` |
| E-003 | `backend/internal/logic/tryon/gpt_image.go` | `decodeB64` 只检查 Base64 解码结果 | `rg -n "func decodeB64" backend/internal/logic/tryon/gpt_image.go` |
| E-004 | `backend/manifest/config/config.example.yaml` | 请求体限制配置未启用 | `rg -n "clientMaxBodySize" backend/manifest/config/config.example.yaml` |

### 3.2 Findings

| ID | 结论 | 依据 | 影响 |
|---|---|---|---|
| F-001 | 公开部署时，任何人都可能调用生成接口 | E-001、E-002 | 可能消耗模型额度或占满本地 GPU |
| F-002 | 合法 Base64 不等于合法、安全的图片 | E-003、E-004 | 可能造成内存、CPU、带宽浪费或模型调用失败 |

### 3.3 请求处理 Path

目标处理顺序如下：

1. 接收请求并生成请求 ID。
2. 校验 Origin。
3. 限制整个 HTTP 请求体大小。
4. 校验后端访问 Token。
5. 执行速率、每日额度和并发检查。
6. 校验两张图片的 Base64、真实 MIME、文件大小、宽高和像素数。
7. 调用现有 `DoTryonWithClientConfig` 及模型适配器。
8. 无论成功或失败，都释放并发占用。

## 4. 功能 1：接口鉴权、限流和费用保护

### 4.1 用户故事

- 作为服务部署者，我可以设置一个或多个后端访问 Token，阻止匿名用户调用模型。
- 作为服务部署者，我可以限制单个 Token 的调用频率和每日额度，避免费用失控。
- 作为服务部署者，我可以限制全局和单 Token 并发，避免服务器或 GPU 被占满。
- 作为前端用户，我可以在设置中心填写后端访问 Token，并正常调用 `/tryon`。
- 作为前端用户，当请求被拒绝时，我能看到明确的原因，而不是统一的“生成失败”。

### 4.2 FR-1：后端访问 Token

`POST /tryon` 必须支持以下认证头：

```http
Authorization: Bearer ${BACKEND_ACCESS_TOKEN}
```

要求：

- 后端访问 Token 与 Gemini、OpenAI、豆包等模型 API Key 完全独立。
- 支持配置多个有效 Token。
- Token 从环境变量读取，不写入仓库中的真实配置文件。
- Token 去除首尾空格后长度必须为 32 到 512 个字符。
- 内存中只保存 Token 的 SHA-256 摘要。
- 比较 Token 时使用恒定时间比较，避免普通字符串比较泄漏时序信息。
- 日志、错误响应和页面不得输出完整 Token。
- `authRequired: true` 且没有有效 Token 时，后端启动失败。
- 本地开发可明确设置 `authRequired: false`，但启动时必须输出安全警告。

环境变量格式：

```dotenv
TRYON_API_KEYS=owner-token-with-at-least-32-characters,second-owner-token-with-at-least-32-characters
```

### 4.3 FR-2：认证主体标识

后端需要为每个有效 Token 生成内部调用方标识 `principal_id`：

- 取 Token SHA-256 的前 16 个十六进制字符。
- `principal_id` 只用于限流、额度、并发和脱敏日志。
- 不向前端返回 `principal_id`。
- 不得使用 Token 明文作为 Map Key 或日志字段。

### 4.4 FR-3：CORS 白名单

替换无条件的 `CORSDefault()`，按配置精确匹配 Origin。

要求：

- 只允许 `security.trustedOrigins` 中的完整 Origin。
- 不允许 `*`。
- 不允许用字符串后缀模糊匹配域名。
- 允许方法：`POST, GET, OPTIONS`。
- 允许请求头：`Authorization, Content-Type, X-Request-ID`。
- 暴露响应头：`X-Request-ID, Retry-After`。
- `OPTIONS` 预检不进行 Token 校验，也不消耗调用额度。
- 非浏览器请求没有 `Origin` 时，仍然必须通过 Token 鉴权。
- 只有明确配置 `allowPrivateNetwork: true` 时，才返回 `Access-Control-Allow-Private-Network: true`。

默认允许的开发 Origin：

```yaml
trustedOrigins:
  - "https://paipiange.github.io"
  - "http://localhost:3000"
  - "http://127.0.0.1:3000"
```

### 4.5 FR-4：调用频率限制

对 `POST /tryon` 实施每个 `principal_id` 独立的令牌桶限流。

默认值：

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `requestsPerMinute` | 10 | 每个 Token 每分钟最多发起 10 次请求 |
| `burst` | 3 | 短时间突发容量 |
| 限流器闲置回收时间 | 30 分钟 | 防止 Map 无限增长 |

要求：

- 推荐使用 `golang.org/x/time/rate`。
- 限流发生在图片 Base64 解码和模型调用之前。
- 超限返回 HTTP `429`。
- 响应包含 `Retry-After`。
- 关闭鉴权时，限流键退化为客户端 IP。
- 默认不信任客户端任意传入的 `X-Forwarded-For`。
- 只有显式配置可信反向代理时才能读取代理转发 IP。

### 4.6 FR-5：每日调用额度

每个 `principal_id` 默认每天最多成功进入模型调用 100 次。

要求：

- 额度按 UTC 自然日重置。
- Token 无效、CORS 失败、请求体过大、图片校验失败不扣减每日额度。
- 通过所有图片校验并准备调用模型时，原子地扣减一次额度。
- 模型调用失败仍计入额度，因为已经消耗了外部模型或本地计算资源。
- 额度数据第一版保存在内存中，后端重启会重置；部署文档必须明确该限制。
- 达到每日额度返回 HTTP `429`，但使用独立业务错误码。

### 4.7 FR-6：并发限制

保留现有同步接口时，对正在执行的模型生成请求设置并发上限。

默认值：

| 配置 | 默认值 |
|---|---:|
| 全局最大并发 | 2 |
| 单个 Token 最大并发 | 1 |

要求：

- 只有通过鉴权、限流和图片校验的请求才尝试占用并发名额。
- 同时获取单 Token 和全局并发名额。
- 任一名额不足时立即返回，不排队等待。
- 成功、失败、超时、Context 取消和 panic 都必须释放名额。
- 并发控制不能影响 `OPTIONS` 和普通健康检查。

### 4.8 FR-7：前端设置中心

前端设置中心增加“后端访问 Token”字段。

要求：

- 字段类型为密码输入框。
- 与模型 API Key 分开显示，名称和帮助文本不得混淆。
- 保存到当前浏览器设置，字段建议为 `tryonSettings.backendToken`。
- 调用 `/tryon` 时增加 `Authorization` 请求头。
- 清除本地配置时同时清除后端访问 Token。
- 页面、控制台和错误弹窗不得显示 Token。
- 未填写 Token 且后端返回 `AUTH_REQUIRED` 时，提示用户打开设置中心填写后端访问 Token。

### 4.9 FR-8：错误响应

继续使用现有 GoFrame 响应结构：

```json
{
  "code": 40101,
  "message": "缺少后端访问凭证",
  "data": {
    "error_code": "AUTH_REQUIRED",
    "request_id": "01J..."
  }
}
```

错误定义：

| HTTP | 业务码 | `error_code` | 用户提示 |
|---:|---:|---|---|
| 401 | 40101 | `AUTH_REQUIRED` | 请在设置中心填写后端访问 Token |
| 401 | 40102 | `AUTH_INVALID` | 后端访问 Token 无效 |
| 403 | 40301 | `ORIGIN_NOT_ALLOWED` | 当前页面来源不允许访问后端 |
| 429 | 42901 | `RATE_LIMITED` | 请求过于频繁，请稍后重试 |
| 429 | 42902 | `DAILY_QUOTA_EXCEEDED` | 今日生成额度已用完 |
| 429 | 42903 | `CONCURRENCY_LIMITED` | 当前生成任务较多，请稍后重试 |

所有错误响应必须包含 `request_id`，不得包含 Token、模型 API Key、图片 Base64 或供应商原始敏感错误。

### 4.10 功能 1 配置

在 `backend/manifest/config/config.example.yaml` 增加：

```yaml
security:
  authRequired: true
  apiKeysEnv: "TRYON_API_KEYS"
  trustedOrigins:
    - "https://paipiange.github.io"
    - "http://localhost:3000"
    - "http://127.0.0.1:3000"
  allowPrivateNetwork: false
  rateLimit:
    requestsPerMinute: 10
    burst: 3
    limiterIdleMinutes: 30
    dailyRequestsPerPrincipal: 100
  concurrency:
    global: 2
    perPrincipal: 1
```

## 5. 功能 3：图片校验和资源限制

### 5.1 用户故事

- 作为服务部署者，我希望超大请求在 JSON 解析前被拒绝，避免占用过多内存。
- 作为服务部署者，我希望只有真实 JPEG、PNG、WebP 能进入模型调用。
- 作为服务部署者，我希望异常宽高或像素过大的图片被提前拒绝。
- 作为前端用户，我希望大图能够自动压缩，而不是上传后才收到模糊错误。
- 作为前端用户，我希望错误提示能说明是格式、大小还是尺寸问题。

### 5.2 FR-9：整个请求体限制

`POST /tryon` 的整个 JSON 请求体最大为 24 MiB。

要求：

- 在 GoFrame 绑定 JSON 请求参数之前限制 Body。
- 超限后停止继续读取请求体。
- 超限请求不得进入控制器、Base64 解码和模型调用。
- 返回 HTTP `413` 和业务错误 `REQUEST_TOO_LARGE`。
- 限制值必须可配置。

### 5.3 FR-10：Data URL 和 Base64 校验

两张图片字段继续兼容纯 Base64 和 Data URL。

要求：

- 空字符串直接拒绝。
- Data URL 只接受声明为 `image/jpeg`、`image/png` 或 `image/webp` 的 Base64 数据。
- 拒绝包含非 Base64 编码形式的 Data URL。
- 根据字符串长度预估解码大小，明显超限时不执行完整解码。
- Base64 解码失败返回 `INVALID_IMAGE_BASE64`。
- 不信任 Data URL 声明的 MIME，后续必须检查真实文件内容。

### 5.4 FR-11：图片文件大小

默认限制：

| 项目 | 最大值 |
|---|---:|
| 单张 Base64 编码字符串 | 12 MiB |
| 单张图片解码后字节数 | 8 MiB |
| 两张图片解码后合计 | 16 MiB |

要求：

- 人像和服装图分别校验。
- 单图超限返回 `IMAGE_FILE_TOO_LARGE`。
- 两张图片合计超限也返回 `IMAGE_FILE_TOO_LARGE`，消息说明总大小超限。
- 文件大小校验必须在任何完整图像像素解码之前完成。

### 5.5 FR-12：真实图片格式

仅允许：

- `image/jpeg`
- `image/png`
- `image/webp`

要求：

- 使用 `http.DetectContentType` 或等价方式检查真实内容。
- MIME 声明与真实内容不一致时拒绝。
- 使用 `image.DecodeConfig` 验证图片头可以被正确解析。
- WebP 通过 `golang.org/x/image/webp` 注册解码支持。
- 禁止 SVG、GIF、BMP、TIFF、ICO 和任意文本或可执行文件。
- 合法 Base64 但内容不是图片时返回 `UNSUPPORTED_IMAGE_TYPE`。

### 5.6 FR-13：图片尺寸和总像素

默认限制：

| 项目 | 最小值 | 最大值 |
|---|---:|---:|
| 宽度 | 256 px | 8192 px |
| 高度 | 256 px | 8192 px |
| 总像素 | 不单独设置最小值 | 20,000,000 px |

要求：

- 宽高不在范围内返回 `IMAGE_DIMENSIONS_INVALID`。
- 宽高均在范围内但乘积超过 2000 万时返回 `IMAGE_PIXEL_LIMIT_EXCEEDED`。
- 计算 `width * height` 时必须防止整数溢出。
- 只读取图片配置和头部，不需要在校验阶段完整解码全部像素。
- 人像和服装图使用相同限制。

### 5.7 FR-14：统一验证结果

后端增加统一图片验证结果：

```go
type ValidatedImage struct {
    Data   []byte
    MIME   string
    Width  int
    Height int
    SHA256 string
}
```

统一验证入口：

```go
func ValidateImage(input string, limits ImageLimits) (*ValidatedImage, error)
```

要求：

- 所有模型适配器只能接收已经验证过的图片数据。
- 不得在多个模型适配器中各自重复无上限解码用户输入。
- 迁移期间如果适配器仍需要 Base64，应由 `ValidatedImage.Data` 重新编码。
- 日志只允许记录 MIME、宽高、字节数和最多 16 位 SHA-256 前缀。
- 不记录图片内容和完整图片哈希。

### 5.8 FR-15：前端图片压缩

用户上传人像后，前端在发送请求前处理图片。

要求：

1. 只允许选择 JPEG、PNG 或 WebP。
2. 原文件超过 12 MiB 时先尝试压缩，而不是立即上传。
3. 使用浏览器图片解码能力读取图片方向和尺寸。
4. 最长边缩放到不超过 2048 像素，小图不放大。
5. 默认输出 WebP，质量为 `0.90`。
6. 浏览器不支持 WebP 编码时输出 JPEG，质量为 `0.90`。
7. 如果输出仍超过 8 MiB，逐步降低质量，最低不低于 `0.75`。
8. 降到最低质量仍超限时，提示用户更换图片。
9. 图库服装图发送到后端前也经过同一个尺寸检查函数。
10. 前端压缩不能替代后端校验。

### 5.9 FR-16：图片错误提示

| HTTP | 业务码 | `error_code` | 用户提示 |
|---:|---:|---|---|
| 413 | 41301 | `REQUEST_TOO_LARGE` | 上传内容过大，请压缩图片后重试 |
| 422 | 42201 | `INVALID_IMAGE_BASE64` | 图片数据损坏，请重新选择图片 |
| 422 | 42202 | `UNSUPPORTED_IMAGE_TYPE` | 仅支持 JPEG、PNG 和 WebP |
| 422 | 42203 | `IMAGE_FILE_TOO_LARGE` | 图片文件过大，请压缩后重试 |
| 422 | 42204 | `IMAGE_DIMENSIONS_INVALID` | 图片尺寸不符合要求 |
| 422 | 42205 | `IMAGE_PIXEL_LIMIT_EXCEEDED` | 图片像素过高，请缩小后重试 |

前端根据 `error_code` 显示中英文提示，不依赖后端原始错误字符串做正则匹配。

### 5.10 功能 3 配置

在 `backend/manifest/config/config.example.yaml` 增加：

```yaml
upload:
  maxRequestBodyBytes: 25165824
  maxEncodedImageBytes: 12582912
  maxDecodedImageBytes: 8388608
  maxCombinedDecodedBytes: 16777216
  minWidth: 256
  minHeight: 256
  maxWidth: 8192
  maxHeight: 8192
  maxPixels: 20000000
  allowedMimeTypes:
    - "image/jpeg"
    - "image/png"
    - "image/webp"
```

## 6. 中间件和业务处理顺序

建议请求处理顺序：

1. 请求 ID。
2. panic 恢复。
3. CORS。
4. 请求体大小限制。
5. Bearer Token 鉴权。
6. 调用频率限制。
7. JSON 参数绑定。
8. 两张图片校验。
9. 每日额度检查并原子扣减。
10. 单 Token 和全局并发检查。
11. 调用现有同步模型生成逻辑。
12. 释放并发名额并返回结果。

说明：无效图片会消耗速率令牌，但不会消耗每日生成额度，也不会占用生成并发名额。

## 7. 代码影响范围

建议新增：

```text
backend/internal/security/
├── config.go
├── token_store.go
├── limiter.go
├── quota.go
└── concurrency.go

backend/internal/middleware/
├── request_id.go
├── cors.go
├── body_limit.go
├── auth.go
└── rate_limit.go

backend/internal/imagevalidate/
├── config.go
├── data_url.go
├── validate.go
└── validate_test.go
```

建议修改：

| 文件 | 修改内容 |
|---|---|
| `backend/internal/cmd/cmd.go` | 注册安全中间件并替换默认 CORS |
| `backend/api/tryon/v1/tryon.go` | 保持请求字段兼容，补充错误文档和限制说明 |
| `backend/internal/controller/tryon/tryon_v1_tryon.go` | 在模型调用前执行图片校验、额度和并发控制 |
| `backend/internal/logic/tryon/tryon.go` | 接收已验证图片，避免重复无上限解码 |
| `backend/manifest/config/config.example.yaml` | 增加 `security` 和 `upload` 配置 |
| `backend/.env.example` | 增加 `TRYON_API_KEYS` 示例 |
| `index.html` | 增加后端 Token、图片压缩、错误码提示和 Authorization 头 |
| `README.md` | 增加安全配置和图片限制说明 |
| `backend/DEPLOY.md` | 增加公开部署必需配置说明 |

不要直接编辑带有 `DO NOT EDIT` 标记的自动生成文件；如果 API 定义需要重新生成，应使用项目已有 GoFrame 生成流程。

## 8. 测试需求

### 8.1 接口保护测试

- 缺少 Token 返回 `40101`。
- 错误 Token 返回 `40102`。
- 正确 Token 可以进入业务处理。
- Token 明文不会出现在日志和错误响应中。
- 非白名单 Origin 返回 `40301`。
- 白名单 OPTIONS 成功且不消耗额度。
- 超过速率限制返回 `42901` 和 `Retry-After`。
- 超过每日额度返回 `42902`。
- 超过单 Token 并发返回 `42903`。
- 超过全局并发返回 `42903`。
- 模型成功、失败、超时和 panic 后并发名额均被释放。

### 8.2 图片校验测试

- 合法 JPEG、PNG、WebP 可以通过。
- 纯文本编码成 Base64 后返回 `42202`。
- Data URL 声明格式与真实格式不一致时返回 `42202`。
- Base64 损坏返回 `42201`。
- 整个请求体超限返回 `41301`。
- 单张图片字节数超限返回 `42203`。
- 两张图片合计超限返回 `42203`。
- 图片宽高过小或过大返回 `42204`。
- 总像素超过限制返回 `42205`。
- 畸形图片头不会造成 panic。
- `ValidateImage` 增加 Go Fuzz 测试，任意输入不得 panic。
- 所有图片校验失败场景都不会调用模型适配器。

## 9. 验收标准

功能 1 验收：

- [ ] `authRequired: true` 时，匿名用户无法调用 `/tryon`。
- [ ] 前端能够保存后端 Token 并通过 Authorization 头调用接口。
- [ ] CORS 只允许配置的精确 Origin。
- [ ] 每 Token 速率、每日额度和并发限制均生效。
- [ ] 全局并发限制生效。
- [ ] Token、模型 Key 和图片内容不会进入日志或错误响应。

功能 3 验收：

- [ ] 超过 24 MiB 的请求在 JSON 绑定前被拒绝。
- [ ] 后端只接受真实 JPEG、PNG 和 WebP。
- [ ] 单图大小、合计大小、宽高和总像素限制均生效。
- [ ] 所有无效图片都在模型调用前被拒绝。
- [ ] 前端可以压缩大图并显示明确错误原因。
- [ ] 合法的现有试衣流程保持可用。

最终验证命令：

```powershell
Set-Location backend
$unformatted = gofmt -l .
if ($unformatted) { throw "需要 gofmt: $unformatted" }
go vet ./...
go test ./...
```

## 10. 交付要求

实现 AI 完成任务后必须同时交付：

1. 后端鉴权、CORS、限流、额度和并发保护代码。
2. 后端图片校验和请求体限制代码。
3. 前端后端 Token 设置、Authorization 请求头和图片压缩功能。
4. 所有新增功能的自动化测试。
5. 更新后的 `config.example.yaml`、`.env.example`、README 和部署文档。
6. 一份变更文件列表和测试结果摘要。

禁止在本需求范围内顺带实现异步任务、CI、监控平台或前端整体重构。
