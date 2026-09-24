package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"tdload/internal/db"
	"tdload/internal/tg"
)

func (s *Server) handleChannelDownload(w http.ResponseWriter, r *http.Request) {
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	_ = s.DB.RefreshDialogDownloadCounts(r.Context(), db.DefaultTGAccountID)
	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID)
	if err != nil || d == nil {
		writeErr(w, http.StatusNotFound, "频道不存在，请先在 Telegram 页同步")
		return
	}
	cursor, _ := s.DB.GetScanCursor(r.Context(), db.DefaultTGAccountID, chatID)
	localMax, _ := s.DB.MaxDownloadedMessageID(r.Context(), chatID)
	batchSize, _ := s.DB.GetChannelBatchSize(r.Context(), db.DefaultTGAccountID, chatID)
	if batchSize <= 0 {
		batchSize = 100
	}
	failedCount, _ := s.DB.CountFailedItemsForChatActive(r.Context(), chatID)
	caughtUp := d.LastMessageID > 0 && cursor >= d.LastMessageID

	var activeView map[string]any
	var activeID int64
	if active, _ := s.DB.GetActiveChannelTask(r.Context(), chatID); active != nil {
		activeID = active.ID
		activeView = taskViewEnriched(s, r, active)
	}

	history, _ := s.DB.ListChannelTasksForChat(r.Context(), chatID, 15)
	historyViews := make([]map[string]any, 0, len(history))
	for _, t := range history {
		if t.ID == activeID {
			continue
		}
		historyViews = append(historyViews, taskViewEnriched(s, r, t))
	}

	status := "idle"
	if activeView != nil {
		status = "running"
		if st, _ := activeView["status"].(string); st != "" {
			switch st {
			case "paused":
				status = "paused"
			case "queued":
				status = "queued"
			}
		}
	} else if caughtUp {
		status = "caught_up"
	} else if failedCount > 0 {
		status = "has_failed"
	}

	writeOK(w, map[string]any{
		"chatId":            d.ChatID,
		"title":             d.Title,
		"username":          d.Username,
		"kind":              d.Kind,
		"downloadedCount":   d.DownloadedCount,
		"lastMessageId":     d.LastMessageID,
		"scanCursor":        cursor,
		"localMaxMessageId": localMax,
		"caughtUp":          caughtUp,
		"failedCount":       failedCount,
		"status":            status,
		"syncedAt":          d.SyncedAt,
		"isCustom":          d.IsCustom,
		"custom":            d.IsCustom,
		"activeTask":        activeView,
		"recentBatches":     historyViews,
		"defaultBatchSize":  batchSize,
	})
}

func (s *Server) handleChannelContinue(w http.ResponseWriter, r *http.Request) {
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	var body struct {
		Count       int    `json:"count"`
		ContentType string `json:"contentType"`
	}
	_ = jsonDecodeOptional(r, &body)
	if body.Count <= 0 {
		if n, _ := s.DB.GetChannelBatchSize(r.Context(), db.DefaultTGAccountID, chatID); n > 0 {
			body.Count = n
		} else {
			body.Count = 100
		}
	}
	if body.Count < 50 {
		body.Count = 50
	}
	if body.Count > 5000 {
		body.Count = 5000
	}
	contentType := tg.NormalizeContentType(body.ContentType)

	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID)
	if err != nil || d == nil {
		writeErr(w, http.StatusNotFound, "频道不存在，请先在 Telegram 页同步")
		return
	}
	if ok, id, _ := s.DB.HasActiveChannelTask(r.Context(), chatID); ok {
		writeErr(w, http.StatusConflict, fmt.Sprintf("该频道已有进行中的下载任务 #%d", id))
		return
	}
	cursor, _ := s.DB.GetScanCursor(r.Context(), db.DefaultTGAccountID, chatID)
	if d.LastMessageID > 0 && cursor >= d.LastMessageID {
		writeErr(w, http.StatusBadRequest, "已追平最新消息，可在「监听」中跟踪增量")
		return
	}

	opt, _ := json.Marshal(map[string]any{
		"chatId": chatID, "count": body.Count, "phase": "listing", "chatTitle": d.Title,
		"fromMessageId": cursor, // 点击继续时的当前水位
		"contentType":   contentType,
	})
	title := d.Title
	if title == "" || strings.HasPrefix(strings.TrimSpace(title), "@") {
		if d.Username != "" && (title == "" || strings.EqualFold(strings.TrimPrefix(title, "@"), d.Username)) {
			// 仍无更好标题时先用不带 @ 的用户名；展示层会再解析
			title = d.Username
		}
	}
	if title == "" {
		title = fmt.Sprintf("%d", chatID)
	}
	title = fmt.Sprintf("%s · 继续下载", title)
	task, err := s.DB.CreateTask(r.Context(), "chat_continue", title, string(opt), body.Count)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.SetChannelBatchSize(r.Context(), db.DefaultTGAccountID, chatID, body.Count)
	if s.Worker != nil {
		s.Worker.Enqueue(task.ID)
	}
	writeOK(w, taskViewEnriched(s, r, task))
}

// handleChannelScanCursor 调整频道历史扫描水位。
// body: { "mode": "set", "messageId": N } 或 { "mode": "align" }（对齐本地 media_index 最大 id）
func (s *Server) handleChannelScanCursor(w http.ResponseWriter, r *http.Request) {
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	var body struct {
		Mode      string `json:"mode"`
		MessageID int    `json:"messageId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID)
	if err != nil || d == nil {
		writeErr(w, http.StatusNotFound, "频道不存在，请先在 Telegram 页同步")
		return
	}

	localMax, _ := s.DB.MaxDownloadedMessageID(r.Context(), chatID)
	mode := body.Mode
	if mode == "" {
		if body.MessageID > 0 {
			mode = "set"
		} else {
			mode = "align"
		}
	}
	switch mode {
	case "align":
		if err := s.DB.SyncScanCursorForChat(r.Context(), db.DefaultTGAccountID, chatID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "set":
		if body.MessageID < 1 {
			writeErr(w, http.StatusBadRequest, "messageId 最小为 1")
			return
		}
		if err := s.DB.SetScanCursor(r.Context(), db.DefaultTGAccountID, chatID, body.MessageID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "mode 应为 set 或 align")
		return
	}

	cursor, _ := s.DB.GetScanCursor(r.Context(), db.DefaultTGAccountID, chatID)
	caughtUp := d.LastMessageID > 0 && cursor >= d.LastMessageID
	writeOK(w, map[string]any{
		"chatId":            chatID,
		"scanCursor":        cursor,
		"localMaxMessageId": localMax,
		"lastMessageId":     d.LastMessageID,
		"caughtUp":          caughtUp,
	})
}

func (s *Server) handleClearChannelCompletedBatches(w http.ResponseWriter, r *http.Request) {
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	n, err := s.DB.ClearCompletedChannelTasksForChat(r.Context(), chatID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true, "cleared": n})
}
