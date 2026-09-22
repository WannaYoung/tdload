package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"tdload/internal/config"
	"tdload/internal/db"
	"tdload/internal/progress"
	"tdload/internal/tg"
)

type Worker struct {
	Cfg *config.Config
	DB  *db.DB
	TG  *tg.Manager
	Hub *progress.Hub

	queue   chan int64
	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	wg      sync.WaitGroup
}

func New(cfg *config.Config, database *db.DB, tgMgr *tg.Manager, hub *progress.Hub) *Worker {
	return &Worker{
		Cfg:     cfg,
		DB:      database,
		TG:      tgMgr,
		Hub:     hub,
		queue:   make(chan int64, 256),
		cancels: map[int64]context.CancelFunc{},
	}
}

func (w *Worker) Start(ctx context.Context) {
	// Telegram session 同一时间只能跑 1 个下载连接，多槽位只会排队占着 running
	slots := 1
	for i := 0; i < slots; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop(ctx)
		}()
	}
	go func() {
		// 启动时把卡住的 running 改回 queued
		if _, err := w.DB.SQL.ExecContext(ctx, `UPDATE tasks SET status='queued', error='进程重启，重新入队' WHERE status='running'`); err != nil {
			slog.Warn("reset running tasks", "err", err)
		}
		ids, err := w.DB.ListQueuedTaskIDs(ctx)
		if err != nil {
			slog.Warn("list queued", "err", err)
			return
		}
		for _, id := range ids {
			_ = w.DB.UpdateTaskStatus(ctx, id, "queued", "")
			w.Enqueue(id)
		}
	}()
}

func (w *Worker) Stop() {
	w.mu.Lock()
	for id, cancel := range w.cancels {
		cancel()
		delete(w.cancels, id)
	}
	w.mu.Unlock()
	close(w.queue)
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		slog.Warn("worker drain timeout")
	}
}

func (w *Worker) Enqueue(id int64) {
	select {
	case w.queue <- id:
	default:
		go func() { w.queue <- id }()
	}
}

func (w *Worker) Pause(id int64) {
	w.mu.Lock()
	if cancel, ok := w.cancels[id]; ok {
		cancel()
	}
	w.mu.Unlock()
	_ = w.DB.UpdateTaskStatus(context.Background(), id, "paused", "用户暂停")
	w.Hub.Publish(progress.Event{TaskID: id, Status: "paused", Phase: "paused"})
}

func (w *Worker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-w.queue:
			if !ok {
				return
			}
			w.runOne(ctx, id)
		}
	}
}

func (w *Worker) runOne(parent context.Context, id int64) {
	task, err := w.DB.GetTask(parent, id)
	if err != nil || task == nil {
		return
	}
	if task.Status == "paused" || task.Status == "cancelled" || task.Status == "done" {
		return
	}

	ctx, cancel := context.WithTimeout(parent, 4*time.Hour)
	w.mu.Lock()
	w.cancels[id] = cancel
	w.mu.Unlock()
	defer func() {
		cancel()
		w.mu.Lock()
		delete(w.cancels, id)
		w.mu.Unlock()
	}()

	_ = w.DB.UpdateTaskStatus(ctx, id, "running", "")
	_ = w.DB.AddTaskLog(ctx, id, "info", "开始下载")
	slog.Info("task start", "id", id, "urls", task.URLs)
	w.Hub.Publish(progress.Event{
		TaskID: id, Phase: "running", Status: "running",
		Done: task.DoneFiles, Total: task.TotalFiles, Title: task.Title,
	})

	switch task.Source {
	case "saved_all":
		selfID := int64(0)
		if acc, _ := w.DB.GetTGAccount(context.Background(), db.DefaultTGAccountID); acc != nil {
			selfID = acc.UserID
		}
		if selfID == 0 {
			if uid, err := w.TG.SelfUserID(ctx); err == nil {
				selfID = uid
			}
		}
		favID := tg.FavoritesChatID(selfID)
		rows, err := w.DB.SQL.QueryContext(ctx, `SELECT message_id FROM saved_messages_cache WHERE tg_account_id=1 ORDER BY message_id`)
		if err != nil {
			_ = w.DB.UpdateTaskStatus(context.Background(), id, "failed", err.Error())
			return
		}
		var ids []int
		for rows.Next() {
			var mid int
			if err := rows.Scan(&mid); err == nil {
				ids = append(ids, mid)
			}
		}
		rows.Close()
		opt := w.downloadOpts(id, tg.FavoritesFolderName)
		opt.OnProgress = func(doneFiles, totalFiles int, fileName string) {
			bg := context.Background()
			_ = w.DB.UpdateTaskProgress(bg, id, doneFiles, len(ids), 0, 0, 0)
			w.publishItemProgress(id, favID, 0, "downloading")
			w.Hub.Publish(progress.Event{
				Type: "task_progress", TaskID: id, Phase: "downloading", Status: "running",
				Done: doneFiles, Total: len(ids), Title: fileName,
			})
		}
		err = w.TG.DownloadSaved(ctx, opt, favID, ids)
	case "chat_batch", "chat_continue", "chat_range":
		err = fmt.Errorf("频道批量下载 Worker 开发中，请先使用消息链接下载")
	default:
		opt := w.downloadOpts(id, "")
		opt.URLs = task.URLs
		opt.OnProgress = func(doneFiles, totalFiles int, fileName string) {
			bg := context.Background()
			_ = w.DB.UpdateTaskProgress(bg, id, doneFiles, totalFiles, 0, 0, 0)
			if fileName != "" {
				_ = w.DB.AddTaskLog(bg, id, "info", fileName)
			}
			w.Hub.Publish(progress.Event{
				Type: "task_item_progress", TaskID: id, Phase: "downloading", Status: "running",
				Done: doneFiles, Total: totalFiles, Title: fileName,
			})
			w.Hub.Publish(progress.Event{
				Type: "task_progress", TaskID: id, Phase: "downloading", Status: "running",
				Done: doneFiles, Total: totalFiles, Title: fileName,
			})
		}
		err = w.TG.DownloadURLs(ctx, opt)
	}

	if err != nil {
		interrupted := parent.Err() != nil || errors.Is(err, context.Canceled)
		if interrupted {
			cur, _ := w.DB.GetTask(context.Background(), id)
			if cur != nil && cur.Status == "paused" {
				return
			}
			_ = w.DB.UpdateTaskStatus(context.Background(), id, "queued", "中断，待重试")
			w.Hub.Publish(progress.Event{TaskID: id, Status: "queued", Phase: "queued", Error: "interrupted"})
			return
		}
		_ = w.DB.UpdateTaskStatus(context.Background(), id, "failed", err.Error())
		_ = w.DB.AddTaskLog(context.Background(), id, "error", err.Error())
		w.Hub.Publish(progress.Event{TaskID: id, Status: "failed", Phase: "failed", Error: err.Error()})
		slog.Error("task failed", "id", id, "err", err)
		return
	}

	final, _ := w.DB.GetTask(context.Background(), id)
	doneFiles, totalFiles := 0, 0
	if final != nil {
		doneFiles, totalFiles = final.DoneFiles, final.TotalFiles
	}
	if doneFiles == 0 {
		msg := "未写入任何文件"
		_ = w.DB.UpdateTaskStatus(context.Background(), id, "failed", msg)
		_ = w.DB.AddTaskLog(context.Background(), id, "error", msg)
		w.Hub.Publish(progress.Event{TaskID: id, Status: "failed", Phase: "failed", Error: msg})
		slog.Error("task failed", "id", id, "err", msg)
		return
	}
	_ = w.DB.UpdateTaskStatus(context.Background(), id, "done", "")
	_ = w.DB.AddTaskLog(context.Background(), id, "info", "完成")
	w.Hub.Publish(progress.Event{TaskID: id, Status: "done", Phase: "done", Done: doneFiles, Total: totalFiles})
	slog.Info("task done", "id", id)
}
