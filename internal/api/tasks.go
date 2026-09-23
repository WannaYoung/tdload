package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tdload/internal/auth"
	"tdload/internal/db"
	"tdload/internal/tg"
)

func (s *Server) handleCreateTasks(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source        string   `json:"source"`
		URLs          []string `json:"urls"`
		Text          string   `json:"text"`
		Title         string   `json:"title"`
		ChatID        int64    `json:"chatId"`
		Chat          string   `json:"chat"`
		Username      string   `json:"username"`
		FromMessageID int      `json:"fromMessageId"`
		Count         int      `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	source := body.Source
	if source == "" {
		source = "url"
	}
	switch source {
	case "saved_all":
		n, _ := s.DB.SavedMessageCount(r.Context(), db.DefaultTGAccountID)
		if n == 0 {
			writeErr(w, http.StatusBadRequest, "收藏缓存为空，请先在 Telegram 页同步收藏")
			return
		}
		if ok, id, _ := s.DB.HasActiveSavedSyncTask(r.Context()); ok {
			writeErr(w, http.StatusConflict, fmt.Sprintf("已有进行中的收藏同步 #%d", id))
			return
		}
		opt, _ := json.Marshal(map[string]any{"outSubdir": "我的收藏"})
		task, err := s.DB.CreateTask(r.Context(), "saved_all", "收藏同步", string(opt), n)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.Worker != nil {
			s.Worker.Enqueue(task.ID)
		}
		writeOK(w, taskViewEnriched(s, r, task))
		return
	case "chat_batch":
		chatID, username, errMsg := parseChannelRef(body.ChatID, body.Chat, body.Username)
		if errMsg != "" {
			writeErr(w, http.StatusBadRequest, errMsg)
			return
		}
		if body.FromMessageID <= 0 {
			writeErr(w, http.StatusBadRequest, "请填写起始消息 ID")
			return
		}
		if body.Count <= 0 {
			body.Count = 100
		}
		if body.Count < 50 {
			body.Count = 50
		}
		if body.Count > 5000 {
			body.Count = 5000
		}
		chatTitle := ""
		// 尽量解析真实名称（公开频道 / 已加入均可）
		if s.TG != nil {
			if info, err := s.TG.ResolveChatInfo(r.Context(), chatID, username); err == nil && info != nil {
				chatID = info.ChatID
				if info.Username != "" {
					username = info.Username
				}
				chatTitle = info.Title
				_ = s.DB.UpsertChatLabel(r.Context(), info.ChatID, info.Title, info.Username)
			}
		}
		if chatTitle == "" {
			if d, _ := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID); d != nil {
				chatTitle = d.Title
				if chatTitle == "" {
					chatTitle = d.Username
				}
				if username == "" {
					username = d.Username
				}
			}
		}
		if chatTitle == "" {
			if label, uname, ok := s.DB.GetChatLabel(r.Context(), chatID); ok {
				chatTitle = label
				if username == "" {
					username = uname
				}
			}
		}
		if chatTitle == "" && username != "" {
			chatTitle = "@" + username
		}
		if chatTitle == "" {
			chatTitle = fmt.Sprintf("%d", chatID)
		}
		// 仅与同 chatId 的任务页频道下载互斥；不要求已同步、不写频道水位 / batch_size
		if chatID != 0 {
			if ok, id, _ := s.DB.HasActiveChatBatchTask(r.Context(), chatID); ok {
				writeErr(w, http.StatusConflict, fmt.Sprintf("该频道已有进行中的下载任务 #%d", id))
				return
			}
			if alt := tg.NormalizeChannelChatID(chatID); alt != chatID {
				if ok, id, _ := s.DB.HasActiveChatBatchTask(r.Context(), alt); ok {
					writeErr(w, http.StatusConflict, fmt.Sprintf("该频道已有进行中的下载任务 #%d", id))
					return
				}
			}
		}
		optMap := map[string]any{
			"chatId": chatID, "fromMessageId": body.FromMessageID, "count": body.Count,
			"chatTitle": chatTitle,
		}
		if username != "" {
			optMap["username"] = username
		}
		opt, _ := json.Marshal(optMap)
		title := body.Title
		if title == "" {
			title = fmt.Sprintf("%s · #%d–#%d", chatTitle, body.FromMessageID, body.FromMessageID+body.Count-1)
		}
		task, err := s.DB.CreateTask(r.Context(), "chat_batch", title, string(opt), body.Count)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.Worker != nil {
			s.Worker.Enqueue(task.ID)
		}
		writeOK(w, taskViewEnriched(s, r, task))
		return
	case "chat_continue":
		if body.ChatID == 0 {
			writeErr(w, http.StatusBadRequest, "请选择频道")
			return
		}
		d, _ := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, body.ChatID)
		if d == nil {
			writeErr(w, http.StatusNotFound, "频道不存在，请先在 Telegram 页同步对话")
			return
		}
		if ok, id, _ := s.DB.HasActiveChannelTask(r.Context(), body.ChatID); ok {
			writeErr(w, http.StatusConflict, fmt.Sprintf("该频道已有进行中的下载任务 #%d", id))
			return
		}
		if body.Count <= 0 {
			if n, _ := s.DB.GetChannelBatchSize(r.Context(), db.DefaultTGAccountID, body.ChatID); n > 0 {
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
		opt, _ := json.Marshal(map[string]any{
			"chatId": body.ChatID, "fromMessageId": body.FromMessageID, "count": body.Count,
		})
		title := body.Title
		if title == "" {
			name := d.Title
			if name == "" {
				name = d.Username
			}
			if name == "" {
				name = fmt.Sprintf("%d", body.ChatID)
			}
			title = name + " · 续下"
		}
		task, err := s.DB.CreateTask(r.Context(), "chat_continue", title, string(opt), 0)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = s.DB.SetChannelBatchSize(r.Context(), db.DefaultTGAccountID, body.ChatID, body.Count)
		if s.Worker != nil {
			s.Worker.Enqueue(task.ID)
		}
		writeOK(w, taskViewEnriched(s, r, task))
		return
	default:
		urls := body.URLs
		if body.Text != "" {
			for _, line := range strings.Split(body.Text, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					urls = append(urls, line)
				}
			}
		}
		urls = filterTGURLs(urls)
		if len(urls) == 0 {
			writeErr(w, http.StatusBadRequest, "请粘贴至少一条 t.me 消息链接")
			return
		}
		task, err := s.DB.CreateURLTask(r.Context(), urls, body.Title)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.Worker != nil {
			s.Worker.Enqueue(task.ID)
		}
		writeOK(w, taskViewEnriched(s, r, task))
	}
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		switch kind {
		case "channel":
			pageSize = 20
		default:
			pageSize = 50
		}
	}
	items, total, err := s.DB.ListTasks(r.Context(), kind, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, t := range items {
		views = append(views, taskViewEnriched(s, r, t))
	}
	writeOK(w, map[string]any{
		"items":    views,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	task, err := s.DB.GetTask(r.Context(), id)
	if err != nil || task == nil {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeOK(w, taskViewEnriched(s, r, task))
}

func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	if s.Worker != nil {
		s.Worker.Pause(id)
	} else {
		_ = s.DB.UpdateTaskStatus(r.Context(), id, "paused", "用户暂停")
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleRetryTask(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	if s.Worker != nil {
		s.Worker.Pause(id)
	}
	if err := s.DB.DeleteTask(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) handleClearCompleted(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	n, err := s.DB.ClearCompletedTasks(r.Context(), kind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true, "cleared": n})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "不支持 SSE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	if s.Hub == nil {
		return
	}
	ch, unsub := s.Hub.Subscribe(32)
	defer unsub()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", ev.JSON())
			flusher.Flush()
		}
	}
}

func taskView(t *db.Task) map[string]any {
	return map[string]any{
		"id":            t.ID,
		"source":        t.Source,
		"title":         t.Title,
		"status":        t.Status,
		"totalBytes":    t.TotalBytes,
		"doneBytes":     t.DoneBytes,
		"totalFiles":    t.TotalFiles,
		"doneFiles":     t.DoneFiles,
		"speedBps":      t.SpeedBPS,
		"error":         t.Error,
		"urls":          t.URLs,
		"createdAt":     t.CreatedAt,
		"startedAt":     nullString(t.StartedAt),
		"finishedAt":    nullString(t.FinishedAt),
		"progressDone":  t.DoneFiles,
		"progressTotal": t.TotalFiles,
	}
}

func nullString(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func filterTGURLs(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.Contains(u, "t.me/") {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// parseChannelRef 解析频道引用：支持 chatId、@username，或 chat 字段（数字 / 用户名）。
func parseChannelRef(chatID int64, chat, username string) (int64, string, string) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	chat = strings.TrimSpace(chat)
	if chat != "" {
		raw := strings.TrimPrefix(chat, "@")
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n != 0 {
			chatID = n
		} else if raw != "" {
			username = raw
		}
	}
	if chatID == 0 && username == "" {
		return 0, "", "请填写频道 ID 或 @用户名"
	}
	if chatID > 0 {
		chatID = tg.NormalizeChannelChatID(chatID)
	}
	return chatID, username, ""
}

func pathID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

// withSSEAuth 允许 session 或 sse ticket。
func (s *Server) withSSEAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearer(r)
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "未登录")
			return
		}
		claims, err := auth.Parse(s.Cfg.JWTSecret, tok)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "登录已失效")
			return
		}
		if claims.Typ != auth.TypSession && claims.Typ != auth.TypSSE {
			writeErr(w, http.StatusUnauthorized, "无效令牌类型")
			return
		}
		next(w, r)
	}
}
