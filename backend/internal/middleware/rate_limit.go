package middleware

import (
	"fmt"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"

	"backend/internal/security"
)

type RateLimitMiddleware struct {
	limiter *security.RateLimiter
}

func NewRateLimitMiddleware(limiter *security.RateLimiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{limiter: limiter}
}

func (m *RateLimitMiddleware) Handler(r *ghttp.Request) {
	if r.Method == http.MethodOptions {
		r.Middleware.Next()
		return
	}
	if r.URL.Path != "/tryon" {
		r.Middleware.Next()
		return
	}

	principalID := r.GetCtxVar(CtxPrincipalID).String()
	if principalID == "" {
		principalID = "anonymous"
	}

	ok, retryAfter := m.limiter.Allow(principalID)
	if !ok {
		r.Response.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		WriteAPIError(r, security.ErrRateLimited)
		return
	}

	r.Middleware.Next()
}
