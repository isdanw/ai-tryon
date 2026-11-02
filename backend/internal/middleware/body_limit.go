package middleware

import (
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"

	"backend/internal/security"
)

// testConnectionMaxBytes 诊断接口只需要几个短字段，64 KiB 足够。
const testConnectionMaxBytes int64 = 64 << 10

type BodyLimitMiddleware struct {
	maxBytes int64
}

func NewBodyLimitMiddleware(maxBytes int64) *BodyLimitMiddleware {
	if maxBytes <= 0 {
		maxBytes = 25165824 // 24 MiB
	}
	return &BodyLimitMiddleware{maxBytes: maxBytes}
}

func (m *BodyLimitMiddleware) Handler(r *ghttp.Request) {
	if r.Method != http.MethodPost || (r.URL.Path != "/tryon" && r.URL.Path != "/test-connection") {
		r.Middleware.Next()
		return
	}

	maxBytes := m.maxBytes
	if r.URL.Path == "/test-connection" {
		maxBytes = testConnectionMaxBytes
	}

	if r.ContentLength > maxBytes {
		WriteAPIError(r, security.ErrRequestTooLarge)
		return
	}

	// 限制读取流，防止 chunked 分块传输或未标明长度的超大请求
	r.Request.Body = http.MaxBytesReader(r.Response.RawWriter(), r.Request.Body, maxBytes)

	r.Middleware.Next()

	// 若在后续参数解析阶段触发了 MaxBytesError，拦截并转换为标准 41301 响应
	if err := r.GetError(); err != nil {
		if _, ok := err.(*http.MaxBytesError); ok || err.Error() == "http: request body too large" {
			r.SetError(nil)
			WriteAPIError(r, security.ErrRequestTooLarge)
			return
		}
	}
}
