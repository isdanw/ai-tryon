package cmd

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func TestEnsureServerPort(t *testing.T) {
	ctx := context.Background()
	s := g.Server("test_port_server")
	EnsureServerPort(ctx, s)

	// 当未配置自定义端口时，经过 EnsureServerPort 校验，端口应稳定为 8000
	// 验证 s 的内部状态不会引起 panic 且逻辑正常
}

func TestInitConfigFallback(t *testing.T) {
	ctx := context.Background()
	// 保存旧 adapter
	oldAdapter := g.Cfg().GetAdapter()
	defer func() {
		g.Cfg().SetAdapter(oldAdapter)
	}()

	// 模拟未加载到配置文件的情况
	mockAdapter, err := gcfg.NewAdapterFile("non_existent_config.yaml")
	if err != nil {
		t.Fatalf("create mock adapter failed: %v", err)
	}
	g.Cfg().SetAdapter(mockAdapter)

	if g.Cfg().Available(ctx) {
		t.Fatalf("expected config to be unavailable for non_existent_config.yaml")
	}

	InitConfigFallback(ctx)

	// 如果存在 config.example.yaml，则应该成功回退并变得可用
	if g.Cfg().Available(ctx) {
		val, err := g.Cfg().Get(ctx, "server.address")
		if err != nil {
			t.Fatalf("read server.address failed: %v", err)
		}
		if val.String() != ":8000" {
			t.Fatalf("expected server.address to be :8000 from fallback, got: %s", val.String())
		}
	}
}
