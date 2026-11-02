package security

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
)

type Service struct {
	Config             *SecurityConfig
	TokenStore         *TokenStore
	RateLimiter        *RateLimiter
	DailyQuotaTracker  *DailyQuotaTracker
	ConcurrencyManager *ConcurrencyManager
}

var defaultService *Service

func InitService(ctx context.Context) (*Service, error) {
	cfg, err := LoadSecurityConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("加载安全配置失败: %w", err)
	}

	tokenStore, err := NewTokenStore(cfg.AuthRequired, cfg.APIKeysEnv)
	if err != nil {
		return nil, err
	}

	if !cfg.AuthRequired {
		g.Log().Warning(ctx, "【安全警告】security.authRequired 设置为 false，已关闭接口访问鉴权，任何客户端均可调用模型！")
	}

	s := &Service{
		Config:             cfg,
		TokenStore:         tokenStore,
		RateLimiter:        NewRateLimiter(cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst, cfg.RateLimit.LimiterIdleMinutes),
		DailyQuotaTracker:  NewDailyQuotaTracker(cfg.RateLimit.DailyRequestsPerPrincipal),
		ConcurrencyManager: NewConcurrencyManager(cfg.Concurrency.Global, cfg.Concurrency.PerPrincipal),
	}
	defaultService = s
	return s, nil
}

func GetService() *Service {
	if defaultService == nil {
		cfg := DefaultSecurityConfig()
		store, _ := NewTokenStore(false, "")
		defaultService = &Service{
			Config:             cfg,
			TokenStore:         store,
			RateLimiter:        NewRateLimiter(cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst, cfg.RateLimit.LimiterIdleMinutes),
			DailyQuotaTracker:  NewDailyQuotaTracker(cfg.RateLimit.DailyRequestsPerPrincipal),
			ConcurrencyManager: NewConcurrencyManager(cfg.Concurrency.Global, cfg.Concurrency.PerPrincipal),
		}
	}
	return defaultService
}
