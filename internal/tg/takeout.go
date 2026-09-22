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

// withAPI 在启用 takeout 时包装 API，降低 FloodWait；失败则回退普通会话。
func (m *Manager) withAPI(ctx context.Context, client *telegram.Client, fn func(context.Context, *tg.Client) error) error {
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
