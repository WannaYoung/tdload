package worker

import (
	"context"
	"encoding/json"

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
	counts, err := w.DB.TaskItemCounts(context.Background(), taskID)
	if err != nil {
		return
	}
	total := counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
	done := counts.Done + counts.Skipped
	_ = w.DB.UpdateTaskProgress(context.Background(), taskID, done, total, 0, 0, 0)
	w.Hub.Publish(progress.Event{
		Type: "task_progress", Kind: "channel", TaskID: taskID, Phase: "downloading", Status: "running",
		Done: done, Total: total,
		ItemCounts: progress.ItemCounts{
			Pending: counts.Pending, Downloading: counts.Downloading, Done: counts.Done,
			Skipped: counts.Skipped, Failed: counts.Failed,
		},
	})
}

func parseTaskOptions(jsonStr string) map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(jsonStr), &m)
	return m
}
