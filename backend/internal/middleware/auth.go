package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"

	"backend/internal/security"
)

type AuthMiddleware struct {
	tokenStore *security.TokenStore
}

func NewAuthMiddleware(store *security.TokenStore) *AuthMiddleware {
	return &AuthMiddleware{tokenStore: store}
}

func (m *AuthMiddleware) Handler(r *ghttp.Request) {
	if r.Method == http.MethodOptions {
		r.Middleware.Next()
		return
	}
	if r.URL.Path != "/tryon" {
		r.Middleware.Next()
		return
	}

	authHeader := r.Header.Get("Authorization")
	var token string
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token = strings.TrimSpace(authHeader[7:])
	}

	if m.tokenStore.IsAuthRequired() {
		if token == "" {
			WriteAPIError(r, security.ErrAuthRequired)
			return
		}
		principalID, ok := m.tokenStore.ValidateToken(token)
		if !ok {
			WriteAPIError(r, security.ErrAuthInvalid)
			return
		}
		r.SetCtxVar(CtxPrincipalID, principalID)
		ctx := context.WithValue(r.Context(), CtxPrincipalID, principalID)
		r.SetCtx(ctx)
	} else {
		// 未开启鉴权模式
		principalID := ""
		if token != "" {
			if pid, ok := m.tokenStore.ValidateToken(token); ok {
				principalID = pid
			}
		}
		if principalID == "" {
			// 回退到客户端 IP 作为限流标识
			principalID = "ip:" + r.GetClientIp()
		}
		r.SetCtxVar(CtxPrincipalID, principalID)
		ctx := context.WithValue(r.Context(), CtxPrincipalID, principalID)
		r.SetCtx(ctx)
	}

	r.Middleware.Next()
}
