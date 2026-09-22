package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"tdload/internal/db"
	"tdload/internal/progress"
	"tdload/internal/tg"
)

func (w *Worker) downloadOpts(taskID int64, outSubdir string) tg.DownloadOptions {
	return tg.DownloadOptions{
		OutDir:     w.Cfg.DownloadDir,
		OutSubdir:  outSubdir,
		Threads:    w.Cfg.Threads,
		Template:   w.Cfg.Template,
		GroupAlbum: w.Cfg.GroupAlbum,
		SkipSame:   w.Cfg.SkipSame,
		RewriteExt: w.Cfg.RewriteExt,
		Exists: func(chatID int64, messageID int, size int64) (bool, string, error) {
			return w.DB.MediaExists(context.Background(), chatID, messageID, size)
		},
		OnFile: func(chatID int64, messageID int, fileName string, size int64, path, mime string) error {
			bg := context.Background()
			_, _ = w.DB.InsertTaskItem(bg, &db.TaskItem{
				TaskID: taskID, ChatID: chatID, MessageID: messageID,
				FileName: fileName, Size: size, Status: "done", LocalPath: path,
			})
			return w.DB.UpsertMedia(bg, chatID, messageID, fileName, size, path, mime)
		},
		OnItem: func(chatID int64, messageID int, status, fileName, localPath, errMsg string) {
			bg := context.Background()
			_, _ = w.DB.InsertTaskItem(bg, &db.TaskItem{
				TaskID: taskID, ChatID: chatID, MessageID: messageID,
				FileName: fileName, Status: status, LocalPath: localPath, Error: errMsg,
			})
			w.publishItemProgress(taskID, chatID, messageID, status)
		},
		OnProgress: func(doneFiles, totalFiles int, fileName string) {
			bg := context.Background()
			_ = w.DB.UpdateTaskProgress(bg, taskID, doneFiles, totalFiles, 0, 0, 0)
			w.Hub.Publish(progress.Event{
				Type: "task_progress", TaskID: taskID, Phase: "downloading", Status: "running",
				Done: doneFiles, Total: totalFiles, Title: fileName,
			})
		},
	}
}

func (w *Worker) publishItemProgress(taskID int64, chatID int64, messageID int, status string) {
	w.Hub.Publish(progress.Event{
		Type: "task_item_progress", TaskID: taskID, ChatID: chatID, MessageID: messageID, Status: status,
	})
}

func (w *Worker) publishChannelProgress(taskID int64) {
	w.publishTaskCounts(taskID, "channel")
}

func (w *Worker) publishTaskCounts(taskID int64, kind string) {
	counts, err := w.DB.TaskItemCounts(context.Background(), taskID)
	if err != nil {
		return
	}
	total := counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
	done := counts.Done + counts.Skipped
	_ = w.DB.UpdateTaskProgress(context.Background(), taskID, done, total, 0, 0, 0)
	w.Hub.Publish(progress.Event{
		Type: "task_progress", Kind: kind, TaskID: taskID, Phase: "downloading", Status: "running",
		Done: done, Total: total,
		ItemCounts: progress.ItemCounts{
			Pending: counts.Pending, Downloading: counts.Downloading, Done: counts.Done,
			Skipped: counts.Skipped, Failed: counts.Failed,
		},
	})
}

func (w *Worker) channelDownloadOpts(taskID int64) tg.DownloadOptions {
	return tg.DownloadOptions{
		OutDir:     w.Cfg.DownloadDir,
		Threads:    w.Cfg.Threads,
		Template:   w.Cfg.Template,
		GroupAlbum: w.Cfg.GroupAlbum,
		SkipSame:   w.Cfg.SkipSame,
		RewriteExt: w.Cfg.RewriteExt,
		Exists: func(chatID int64, messageID int, size int64) (bool, string, error) {
			return w.DB.MediaExists(context.Background(), chatID, messageID, size)
		},
		OnFile: func(chatID int64, messageID int, fileName string, size int64, path, mime string) error {
			return w.DB.UpsertMedia(context.Background(), chatID, messageID, fileName, size, path, mime)
		},
		OnItem: func(chatID int64, messageID int, status, fileName, localPath, errMsg string) {
			bg := context.Background()
			_, _ = w.DB.InsertTaskItem(bg, &db.TaskItem{
				TaskID: taskID, ChatID: chatID, MessageID: messageID,
				FileName: fileName, Status: status, LocalPath: localPath, Error: errMsg,
			})
			w.publishChannelProgress(taskID)
		},
	}
}

func (w *Worker) runChatTask(ctx context.Context, task *db.Task) error {
	optMap := parseTaskOptions(task.OptionsJSON)
	chatID, _ := optMap["chatId"].(float64)
	fromID, _ := optMap["fromMessageId"].(float64)
	count, _ := optMap["count"].(float64)
	toID, _ := optMap["toMessageId"].(float64)
	watchID, _ := optMap["watchId"].(float64)
	if chatID == 0 {
		return fmt.Errorf("任务缺少 chatId")
	}

	dialog, _ := w.DB.GetTGDialog(ctx, db.DefaultTGAccountID, int64(chatID))
	username, title := "", ""
	lastMsgID := 0
	if dialog != nil {
		username, title = dialog.Username, dialog.Title
		lastMsgID = dialog.LastMessageID
	}
	if title == "" {
		title = task.Title
	}

	mode := "batch"
	afterID := 0
	switch task.Source {
	case "chat_continue":
		mode = "continue"
		afterID, _ = w.DB.GetScanCursor(ctx, db.DefaultTGAccountID, int64(chatID))
	case "chat_batch", "chat_range", "watch":
		mode = "batch"
		if task.Source == "watch" && count <= 0 && toID >= fromID && fromID > 0 {
			count = toID - fromID + 1
		}
	}

	dlOpt := w.channelDownloadOpts(task.ID)
	maxMedia := int(count)
	if maxMedia <= 0 {
		maxMedia = 500
	}
	params := tg.ChatDownloadParams{
		ChatID:         int64(chatID),
		Username:       username,
		ChatTitle:      title,
		FromMessageID:  int(fromID),
		Count:          int(count),
		Mode:           mode,
		AfterMessageID: afterID,
		LastMessageID:  lastMsgID,
		MaxMedia:       maxMedia,
		GroupAlbum:     w.Cfg.GroupAlbum,
	}
	scanEnd, err := w.TG.DownloadChat(ctx, dlOpt, params)
	if err != nil {
		return err
	}
	// 仅历史补齐（频道下载）推进 chat_download_state；监听只推进 watched_chats，避免把扫描水位抬过未补完的历史。
	// 文件去重仍共用 media_index，两边互相 skip_same。
	if task.Source != "watch" {
		if scanEnd > 0 {
			_ = w.DB.UpsertChatDownloadState(ctx, db.DefaultTGAccountID, int64(chatID), scanEnd)
		} else if maxID, e := w.DB.MaxTaskItemMessageID(ctx, task.ID); e == nil && maxID > 0 {
			_ = w.DB.UpsertChatDownloadState(ctx, db.DefaultTGAccountID, int64(chatID), maxID)
		}
	}
	if task.Source == "watch" && watchID > 0 {
		cursor := int(toID)
		if cursor <= 0 {
			cursor = int(fromID) + int(count) - 1
		}
		if cursor > 0 {
			_ = w.DB.AdvanceWatchedCursor(ctx, int64(watchID), cursor)
		}
	}
	_ = w.DB.RefreshDialogDownloadCounts(ctx, db.DefaultTGAccountID)
	w.publishChannelProgress(task.ID)
	return nil
}

func (w *Worker) runWatchSavedTask(ctx context.Context, task *db.Task) error {
	optMap := parseTaskOptions(task.OptionsJSON)
	watchID, _ := optMap["watchId"].(float64)
	toID, _ := optMap["toMessageId"].(float64)
	chatID, _ := optMap["chatId"].(float64)
	rawIDs, _ := optMap["messageIds"].([]any)
	ids := make([]int, 0, len(rawIDs))
	for _, v := range rawIDs {
		switch n := v.(type) {
		case float64:
			ids = append(ids, int(n))
		case int:
			ids = append(ids, n)
		}
	}
	if len(ids) == 0 {
		if watchID > 0 && toID > 0 {
			_ = w.DB.AdvanceWatchedCursor(ctx, int64(watchID), int(toID))
		}
		return nil
	}
	favID := int64(chatID)
	if favID == 0 {
		selfID := int64(0)
		if acc, _ := w.DB.GetTGAccount(ctx, db.DefaultTGAccountID); acc != nil {
			selfID = acc.UserID
		}
		if selfID == 0 {
			if uid, err := w.TG.SelfUserID(ctx); err == nil {
				selfID = uid
			}
		}
		favID = tg.FavoritesChatID(selfID)
	}
	opt := w.downloadOpts(task.ID, tg.FavoritesFolderName)
	baseOnItem := opt.OnItem
	opt.OnItem = func(cID int64, messageID int, status, fileName, localPath, errMsg string) {
		if baseOnItem != nil {
			baseOnItem(cID, messageID, status, fileName, localPath, errMsg)
		}
		w.publishTaskCounts(task.ID, "saved")
	}
	opt.OnProgress = func(doneFiles, totalFiles int, fileName string) {
		w.publishTaskCounts(task.ID, "saved")
	}
	err := w.TG.DownloadSaved(ctx, opt, favID, ids)
	if err != nil {
		return err
	}
	if watchID > 0 && toID > 0 {
		_ = w.DB.AdvanceWatchedCursor(ctx, int64(watchID), int(toID))
	}
	w.publishTaskCounts(task.ID, "saved")
	return nil
}

func parseTaskOptions(jsonStr string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(jsonStr), &m)
	return m
}
