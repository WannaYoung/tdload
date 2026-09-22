package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	writeOK(w, map[string]any{"items": taskItemViews(items), "total": total, "page": page, "pageSize": pageSize})
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

func taskItemViews(items []db.TaskItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id": it.ID, "taskId": it.TaskID, "chatId": it.ChatID, "messageId": it.MessageID,
			"fileName": it.FileName, "size": it.Size, "status": it.Status,
			"localPath": it.LocalPath, "error": it.Error,
		})
	}
	return out
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
	default:
		if t.Source == "saved_all" {
			v["kind"] = "saved"
		} else {
			v["kind"] = "message"
		}
	}
	var opt map[string]any
	_ = json.Unmarshal([]byte(t.OptionsJSON), &opt)
	if chatID, ok := opt["chatId"].(float64); ok {
		v["chatId"] = int64(chatID)
	}
	return v
}
