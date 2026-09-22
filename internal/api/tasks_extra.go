package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"tdload/internal/db"
)

func (s *Server) handleListTaskItems(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	items, total, err := s.DB.ListTaskItems(r.Context(), id, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"items": taskItemViews(items), "total": total, "page": page, "pageSize": pageSize})
}

func (s *Server) handleListTaskItemsByKind(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "message"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	items, total, err := s.DB.ListTaskItemsByKind(r.Context(), kind, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	doneCount, _ := s.DB.TaskItemKindDoneCount(r.Context(), kind)
	writeOK(w, map[string]any{
		"items": taskItemViews(items), "total": total, "doneCount": doneCount,
		"page": page, "pageSize": pageSize,
	})
}

func (s *Server) handleDeleteTaskItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "itemId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	if err := s.DB.DeleteTaskItem(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleClearCompletedItems(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "message"
	}
	n, err := s.DB.ClearCompletedTaskItemsByKind(r.Context(), kind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true, "cleared": n})
}

func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	_ = s.DB.UpdateTaskStatus(r.Context(), id, "queued", "")
	if s.Worker != nil {
		s.Worker.Enqueue(id)
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	if s.Worker != nil {
		s.Worker.Pause(id)
	}
	_ = s.DB.UpdateTaskStatus(r.Context(), id, "cancelled", "用户取消")
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleRetryFailedTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	n, err := s.DB.RetryFailedTaskItems(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeErr(w, http.StatusBadRequest, "没有失败项可重试")
		return
	}
	_ = s.DB.UpdateTaskStatus(r.Context(), id, "queued", "")
	if s.Worker != nil {
		s.Worker.Enqueue(id)
	}
	writeOK(w, map[string]any{"ok": true, "retried": n})
}

func (s *Server) handlePauseAllTasks(w http.ResponseWriter, r *http.Request) {
	ids, _ := s.DB.ListQueuedTaskIDs(r.Context())
	if s.Worker != nil {
		for _, id := range ids {
			s.Worker.Pause(id)
		}
	}
	n, err := s.DB.PauseAllActiveTasks(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true, "paused": n})
}

func (s *Server) handleStartAllTasks(w http.ResponseWriter, r *http.Request) {
	ids, err := s.DB.ListPausedTaskIDs(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, id := range ids {
		_ = s.DB.UpdateTaskStatus(r.Context(), id, "queued", "")
		if s.Worker != nil {
			s.Worker.Enqueue(id)
		}
	}
	writeOK(w, map[string]any{"ok": true, "started": len(ids)})
}

func taskItemViews(items []db.TaskItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id": it.ID, "taskId": it.TaskID, "chatId": it.ChatID, "messageId": it.MessageID,
			"fileName": it.FileName, "size": it.Size, "status": it.Status,
			"localPath": it.LocalPath, "error": it.Error,
			"chatTitle": it.ChatTitle,
			"mediaKind": mediaKindLabel(it.Mime, it.FileName),
		})
	}
	return out
}

func mediaKindLabel(mime, fileName string) string {
	lower := strings.ToLower(mime + " " + fileName)
	switch {
	case strings.HasPrefix(mime, "image/"),
		strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"),
		strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".webp"),
		strings.HasSuffix(lower, ".gif"), strings.HasSuffix(lower, ".bmp"):
		return "图片"
	case strings.HasPrefix(mime, "video/"),
		strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".mkv"),
		strings.HasSuffix(lower, ".mov"), strings.HasSuffix(lower, ".webm"),
		strings.HasSuffix(lower, ".avi"):
		return "视频"
	case strings.HasPrefix(mime, "audio/"),
		strings.HasSuffix(lower, ".mp3"), strings.HasSuffix(lower, ".m4a"),
		strings.HasSuffix(lower, ".ogg"), strings.HasSuffix(lower, ".flac"):
		return "音频"
	default:
		if fileName == "" {
			return "—"
		}
		return "文件"
	}
}

func taskViewEnriched(s *Server, r *http.Request, t *db.Task) map[string]any {
	v := taskView(t)
	switch t.Source {
	case "chat_continue", "chat_batch", "chat_range":
		v["kind"] = "channel"
		if counts, err := s.DB.TaskItemCounts(r.Context(), t.ID); err == nil {
			v["itemCounts"] = counts
			v["progressDone"] = counts.Done + counts.Skipped
			v["progressTotal"] = counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
		}
	case "saved_all":
		v["kind"] = "saved"
		if counts, err := s.DB.TaskItemCounts(r.Context(), t.ID); err == nil {
			total := counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
			if total == 0 {
				total = t.TotalFiles
			}
			v["itemCounts"] = counts
			v["progressDone"] = counts.Done + counts.Skipped
			v["progressTotal"] = total
		}
	default:
		v["kind"] = "message"
	}
	var opt map[string]any
	_ = json.Unmarshal([]byte(t.OptionsJSON), &opt)
	if chatID, ok := opt["chatId"].(float64); ok {
		v["chatId"] = int64(chatID)
	}
	return v
}
