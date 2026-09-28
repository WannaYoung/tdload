package tg

import (
	"context"
	"log/slog"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/middlewares/takeout"
)

func chainMiddlewares(invoker tg.Invoker, chain ...telegram.Middleware) tg.Invoker {
	if len(chain) == 0 {
		return invoker
	}
	for i := len(chain) - 1; i >= 0; i-- {
		invoker = chain[i].Handle(invoker)
	}
	return invoker
}

// withAPI 使用普通会话执行回调。
// 注意：takeout 会话下 messages.getHistory / 取单条消息会返回空结果（count=0），
// 因此解析、扫历史、取消息必须走普通 API；文件下载请用 withTakeout。
func (m *Manager) withAPI(ctx context.Context, client *telegram.Client, fn func(context.Context, *tg.Client) error) error {
	return fn(ctx, client.API())
}

// withTakeout 在启用配置时用 takeout 会话执行（适合 upload.getFile 等下载以降 FloodWait），
// 初始化失败则回退普通会话。
func (m *Manager) withTakeout(ctx context.Context, client *telegram.Client, fn func(context.Context, *tg.Client) error) error {
	api := client.API()
	if m.Cfg == nil || !m.Cfg.Takeout {
		return fn(ctx, api)
	}
	sid, err := takeout.Takeout(ctx, api.Invoker())
	if err != nil {
		slog.Warn("takeout init failed, continue without", "err", err)
		return fn(ctx, api)
	}
	wrapped := tg.NewClient(chainMiddlewares(api.Invoker(), takeout.Middleware(sid)))
	slog.Info("takeout session started", "id", sid)
	defer func() {
		if err := takeout.UnTakeout(context.Background(), wrapped.Invoker()); err != nil {
			slog.Warn("takeout finish", "err", err)
		}
	}()
	return fn(ctx, wrapped)
}
