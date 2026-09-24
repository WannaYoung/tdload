package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	ids, err := s.DB.ListFailedTaskItemMessageIDs(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "没有失败项可重试")
		return
	}
	if err := s.DB.SetTaskRetryMessageIDs(r.Context(), id, ids); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	case "saved_all", "watch_saved":
		v["kind"] = "saved"
		if counts, err := s.DB.TaskItemCounts(r.Context(), t.ID); err == nil {
			itemTotal := counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
			total := t.TotalFiles
			if total < itemTotal {
				total = itemTotal
			}
			v["itemCounts"] = counts
			v["progressDone"] = counts.Done + counts.Skipped
			v["progressTotal"] = total
		}
	case "chat_continue", "chat_batch", "chat_range", "watch":
		v["kind"] = "channel"
		if counts, err := s.DB.TaskItemCounts(r.Context(), t.ID); err == nil {
			itemTotal := counts.Pending + counts.Downloading + counts.Done + counts.Skipped + counts.Failed
			total := t.TotalFiles
			if total < itemTotal {
				total = itemTotal
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
	var chatID int64
	if id, ok := opt["chatId"].(float64); ok {
		chatID = int64(id)
		v["chatId"] = chatID
	}
	username := ""
	if u, ok := opt["username"].(string); ok && strings.TrimSpace(u) != "" {
		username = strings.TrimPrefix(strings.TrimSpace(u), "@")
		v["username"] = username
	}
	chatTitle := ""
	if ct, ok := opt["chatTitle"].(string); ok {
		chatTitle = strings.TrimSpace(ct)
	}
	// 优先用对话 / 标签里的真实名称，避免展示 @username
	if resolved := s.resolveChatDisplayName(r.Context(), chatID, username, chatTitle); resolved != "" {
		chatTitle = resolved
		v["chatTitle"] = chatTitle
		if t.Source == "chat_continue" || t.Source == "chat_batch" || t.Source == "chat_range" || t.Source == "watch" {
			v["title"] = rewriteTaskTitleWithChatName(t.Title, chatTitle, username)
		}
	} else if chatTitle != "" {
		v["chatTitle"] = strings.TrimPrefix(chatTitle, "@")
	}
	if phase, ok := opt["phase"].(string); ok && strings.TrimSpace(phase) != "" {
		v["phase"] = strings.TrimSpace(phase)
	}
	if raw, ok := opt["fromMessageId"]; ok {
		if n, ok := asInt(raw); ok && n >= 0 {
			v["fromMessageId"] = n
		}
	} else if t.Source == "chat_continue" || t.Source == "chat_batch" || t.Source == "chat_range" || t.Source == "watch" {
		// 旧批次未写入水位时，用本批最早消息 id 近似展示
		if minID, err := s.DB.MinTaskItemMessageID(r.Context(), t.ID); err == nil && minID > 0 {
			v["fromMessageId"] = minID
		}
	}
	if raw, ok := opt["count"]; ok {
		if n, ok := asInt(raw); ok && n > 0 {
			v["count"] = n
		}
	}
	return v
}

// resolveChatDisplayName 返回频道真实标题（不含 @username）。
func (s *Server) resolveChatDisplayName(ctx context.Context, chatID int64, username, fallback string) string {
	isBad := func(name string) bool {
		name = strings.TrimSpace(name)
		return name == "" || strings.HasPrefix(name, "@")
	}
	if chatID != 0 {
		if d, _ := s.DB.GetTGDialog(ctx, db.DefaultTGAccountID, chatID); d != nil && !isBad(d.Title) {
			return strings.TrimSpace(d.Title)
		}
		if label, _, ok := s.DB.GetChatLabel(ctx, chatID); ok && !isBad(label) {
			return strings.TrimSpace(label)
		}
		if d, _ := s.DB.GetTGDialog(ctx, db.DefaultTGAccountID, chatID); d != nil && strings.TrimSpace(d.Title) != "" {
			return strings.TrimPrefix(strings.TrimSpace(d.Title), "@")
		}
	}
	fallback = strings.TrimSpace(strings.TrimPrefix(fallback, "@"))
	if fallback != "" {
		return fallback
	}
	uname := strings.TrimPrefix(strings.TrimSpace(username), "@")
	if uname != "" {
		return uname
	}
	if chatID != 0 {
		return fmt.Sprintf("%d", chatID)
	}
	return ""
}

// rewriteTaskTitleWithChatName 把标题里的 @用户名 / 纯用户名换成真实频道名。
func rewriteTaskTitleWithChatName(title, chatName, username string) string {
	title = strings.TrimSpace(title)
	chatName = strings.TrimSpace(chatName)
	if title == "" || chatName == "" {
		return title
	}
	uname := strings.TrimPrefix(strings.TrimSpace(username), "@")
	shouldReplace := func(head string) bool {
		head = strings.TrimSpace(head)
		if head == "" {
			return true
		}
		if strings.HasPrefix(head, "@") {
			return true
		}
		bare := strings.TrimPrefix(head, "@")
		if uname != "" && strings.EqualFold(bare, uname) && !strings.EqualFold(chatName, bare) {
			return true
		}
		if _, err := strconv.ParseInt(head, 10, 64); err == nil {
			return true
		}
		return false
	}
	if strings.HasPrefix(title, "监听 · ") {
		rest := strings.TrimPrefix(title, "监听 · ")
		parts := strings.SplitN(rest, " · ", 2)
		if len(parts) == 2 && shouldReplace(parts[0]) {
			return "监听 · " + chatName + " · " + parts[1]
		}
		if len(parts) == 1 && shouldReplace(parts[0]) {
			return "监听 · " + chatName
		}
		return title
	}
	parts := strings.SplitN(title, " · ", 2)
	if shouldReplace(parts[0]) {
		if len(parts) == 2 {
			return chatName + " · " + parts[1]
		}
		return chatName
	}
	return title
}
