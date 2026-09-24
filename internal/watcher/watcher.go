package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"tdload/internal/config"
	"tdload/internal/db"
	"tdload/internal/progress"
	"tdload/internal/tg"
	"tdload/internal/worker"
)

// Watcher 按间隔扫描 watched_chats，将增量媒体入队为 watch / watch_saved 任务。
type Watcher struct {
	Cfg    *config.Config
	DB     *db.DB
	TG     *tg.Manager
	Worker *worker.Worker
	Hub    *progress.Hub
}

func New(cfg *config.Config, database *db.DB, tgMgr *tg.Manager, wrk *worker.Worker, hub *progress.Hub) *Watcher {
	return &Watcher{Cfg: cfg, DB: database, TG: tgMgr, Worker: wrk, Hub: hub}
}

func (w *Watcher) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		w.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				w.tick(ctx)
			}
		}
	}()
}

func (w *Watcher) tick(ctx context.Context) {
	if w.TG == nil || w.Worker == nil {
		return
	}
	now := time.Now().UTC()
	due, err := w.DB.ListDueWatches(ctx, db.DefaultTGAccountID, now.Format(time.RFC3339))
	if err != nil || len(due) == 0 {
		return
	}

	favID := w.favoritesChatID(ctx)
	needDialogs, needSaved := false, false
	for _, item := range due {
		if favID > 0 && item.ChatID == favID {
			needSaved = true
		} else {
			needDialogs = true
		}
	}
	if needDialogs {
		if _, err := w.TG.SyncDialogs(ctx); err != nil {
			slog.Warn("watch sync dialogs", "err", err)
		}
	}
	if needSaved {
		selfID := int64(0)
		if acc, _ := w.DB.GetTGAccount(ctx, db.DefaultTGAccountID); acc != nil {
			selfID = acc.UserID
		}
		if selfID == 0 {
			if id, err := w.TG.SelfUserID(ctx); err == nil {
				selfID = id
			}
		}
		if selfID > 0 {
			if _, err := w.TG.SyncSaved(ctx, selfID, nil); err != nil {
				slog.Warn("watch sync saved", "err", err)
			}
			favID = tg.FavoritesChatID(selfID)
		}
	}

	interval := time.Duration(w.Cfg.ClampWatchInterval()) * time.Minute
	for _, item := range due {
		if err := w.runOne(ctx, item, favID, now, interval); err != nil {
			slog.Warn("watch run", "id", item.ID, "chat", item.ChatID, "err", err)
		}
	}
}

func (w *Watcher) favoritesChatID(ctx context.Context) int64 {
	if acc, _ := w.DB.GetTGAccount(ctx, db.DefaultTGAccountID); acc != nil && acc.UserID > 0 {
		return tg.FavoritesChatID(acc.UserID)
	}
	if id, err := w.TG.SelfUserID(ctx); err == nil && id > 0 {
		return tg.FavoritesChatID(id)
	}
	return 0
}

func (w *Watcher) runOne(ctx context.Context, item db.WatchedChat, favID int64, now time.Time, interval time.Duration) error {
	busy, err := w.DB.HasActiveWatchTask(ctx, item.ID)
	if err != nil {
		return err
	}
	if busy {
		return nil
	}

	isFav := favID > 0 && item.ChatID == favID
	latest := 0
	if isFav {
		latest, _ = w.DB.MaxSavedMessageID(ctx, db.DefaultTGAccountID)
	} else if d, _ := w.DB.GetTGDialog(ctx, db.DefaultTGAccountID, item.ChatID); d != nil {
		latest = d.LastMessageID
	}

	from := item.LastMessageID
	if from <= 0 {
		from = 1
	}
	to := latest
	lastRun := now.Format(time.RFC3339)
	nextRun := now.Add(interval).Format(time.RFC3339)

	// 尚无新水位可读：只推进调度时间
	if to <= 0 {
		return w.DB.UpdateWatchedSchedule(ctx, item.ID, lastRun, nextRun)
	}
	if to < from {
		return w.DB.UpdateWatchedSchedule(ctx, item.ID, lastRun, nextRun)
	}

	if isFav {
		ids, err := w.DB.ListSavedMessageIDsInRange(ctx, db.DefaultTGAccountID, from, to)
		if err != nil {
			return err
		}
		if err := w.DB.UpdateWatchedSchedule(ctx, item.ID, lastRun, nextRun); err != nil {
			return err
		}
		// 无候选消息：直接推进水位
		if len(ids) == 0 {
			return w.DB.AdvanceWatchedCursor(ctx, item.ID, to)
		}
		optMap := map[string]any{
			"watchId": item.ID, "chatId": item.ChatID, "fromMessageId": from, "toMessageId": to, "messageIds": ids,
			"contentType": parseWatchContentType(item.FilterJSON),
		}
		opt, _ := json.Marshal(optMap)
		title := fmt.Sprintf("监听 · 我的收藏 · #%d–#%d", from, to)
		task, err := w.DB.CreateTask(ctx, "watch_saved", title, string(opt), len(ids))
		if err != nil {
			return err
		}
		w.Worker.Enqueue(task.ID)
		w.publishHit(item, task.ID, from, to, len(ids))
		slog.Info("watch enqueued", "watchId", item.ID, "kind", "saved", "from", from, "to", to, "task", task.ID)
		return nil
	}

	count := to - from + 1
	if count > 5000 {
		count = 5000
		to = from + count - 1
	}
	if err := w.DB.UpdateWatchedSchedule(ctx, item.ID, lastRun, nextRun); err != nil {
		return err
	}
	optMap := map[string]any{
		"watchId": item.ID, "chatId": item.ChatID, "fromMessageId": from, "toMessageId": to, "count": count,
		"contentType": parseWatchContentType(item.FilterJSON),
	}
	opt, _ := json.Marshal(optMap)
	title := item.ChatTitle
	if title == "" {
		title = fmt.Sprintf("%d", item.ChatID)
	}
	title = fmt.Sprintf("监听 · %s · #%d–#%d", title, from, to)
	task, err := w.DB.CreateTask(ctx, "watch", title, string(opt), count)
	if err != nil {
		return err
	}
	w.Worker.Enqueue(task.ID)
	w.publishHit(item, task.ID, from, to, count)
	slog.Info("watch enqueued", "watchId", item.ID, "kind", "channel", "from", from, "to", to, "task", task.ID)
	return nil
}

func (w *Watcher) publishHit(item db.WatchedChat, taskID int64, from, to, count int) {
	if w.Hub == nil {
		return
	}
	w.Hub.Publish(progress.Event{
		Type: "watch_hit", TaskID: taskID, ChatID: item.ChatID, Phase: "enqueued",
		Done: from, Total: to, Title: item.ChatTitle, Status: "queued",
		Error: fmt.Sprintf("%d", count),
	})
}

func parseWatchContentType(filterJSON string) string {
	var f struct {
		ContentType string `json:"contentType"`
	}
	_ = json.Unmarshal([]byte(filterJSON), &f)
	return tg.NormalizeContentType(f.ContentType)
}
