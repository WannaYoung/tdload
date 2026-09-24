package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tdload/internal/db"
	"tdload/internal/tg"
)

func jsonDecodeOptional(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}

func channelListItem(s *Server, r *http.Request, d db.TGDialog) map[string]any {
	lastDL, _ := s.DB.GetScanCursor(r.Context(), db.DefaultTGAccountID, d.ChatID)
	caughtUp := d.LastMessageID > 0 && lastDL >= d.LastMessageID
	busy, _, _ := s.DB.HasActiveChannelTask(r.Context(), d.ChatID)
	status := "idle"
	if busy {
		status = "running"
	} else if caughtUp {
		status = "caught_up"
	}
	return map[string]any{
		"chatId":                  d.ChatID,
		"title":                   d.Title,
		"username":                d.Username,
		"kind":                    d.Kind,
		"messageCount":            d.MessageCount,
		"downloadedCount":         d.DownloadedCount,
		"lastMessageId":           d.LastMessageID,
		"lastDownloadedMessageId": lastDL,
		"scanCursor":              lastDL,
		"caughtUp":                caughtUp,
		"status":                  status,
		"syncedAt":                d.SyncedAt,
		"isCustom":                d.IsCustom,
		"custom":                  d.IsCustom,
	}
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	_ = s.DB.RefreshDialogDownloadCounts(r.Context(), db.DefaultTGAccountID)
	items, total, err := s.DB.ListTGDialogs(r.Context(), db.DefaultTGAccountID, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, d := range items {
		views = append(views, channelListItem(s, r, d))
	}
	writeOK(w, map[string]any{
		"items": views, "total": total, "page": page, "pageSize": pageSize,
	})
}

func (s *Server) handleAddCustomChannel(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 未初始化")
		return
	}
	var body struct {
		Chat     string `json:"chat"`
		ChatID   int64  `json:"chatId"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	chatID, username, errMsg := parseChannelRef(body.ChatID, body.Chat, body.Username)
	if errMsg != "" {
		writeErr(w, http.StatusBadRequest, errMsg)
		return
	}
	if existing, err := s.DB.FindTGDialogMatch(r.Context(), db.DefaultTGAccountID, chatID, username); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if existing != nil {
		writeErr(w, http.StatusConflict, "该频道已存在，不能重复添加")
		return
	}
	info, lastMsgID, err := s.TG.FetchChatSnapshot(r.Context(), chatID, username)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if info == nil || info.ChatID == 0 {
		writeErr(w, http.StatusBadRequest, "无法解析频道")
		return
	}
	if existing, err := s.DB.FindTGDialogMatch(r.Context(), db.DefaultTGAccountID, info.ChatID, info.Username); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if existing != nil {
		writeErr(w, http.StatusConflict, "该频道已存在，不能重复添加")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	dialog := db.TGDialog{
		TGAccountID:   db.DefaultTGAccountID,
		ChatID:        info.ChatID,
		Title:         info.Title,
		Username:      info.Username,
		Kind:          "custom",
		MessageCount:  -1,
		LastMessageID: lastMsgID,
		SyncedAt:      now,
		IsCustom:      true,
	}
	if err := s.DB.UpsertCustomDialog(r.Context(), db.DefaultTGAccountID, dialog); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.UpsertChatLabel(r.Context(), info.ChatID, info.Title, info.Username)
	if maxID, err := s.DB.MaxDownloadedMessageID(r.Context(), info.ChatID); err == nil && maxID > 0 {
		_ = s.DB.SetScanCursor(r.Context(), db.DefaultTGAccountID, info.ChatID, maxID)
	}
	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, info.ChatID)
	if err != nil || d == nil {
		writeErr(w, http.StatusInternalServerError, "写入后读取失败")
		return
	}
	writeOK(w, channelListItem(s, r, *d))
}

func (s *Server) handleDeleteCustomChannel(w http.ResponseWriter, r *http.Request) {
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "频道不存在")
		return
	}
	if !d.IsCustom {
		writeErr(w, http.StatusBadRequest, "仅可删除自定义频道")
		return
	}
	if err := s.DB.DeleteCustomDialog(r.Context(), db.DefaultTGAccountID, chatID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true, "chatId": chatID})
}

func (s *Server) handleSyncCustomChannel(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 未初始化")
		return
	}
	chatID, err := pathID(r, "chatId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 chatId")
		return
	}
	d, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, chatID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "频道不存在")
		return
	}
	info, lastMsgID, err := s.TG.FetchChatSnapshot(r.Context(), d.ChatID, d.Username)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	title := info.Title
	if title == "" {
		title = d.Title
	}
	username := info.Username
	if username == "" {
		username = d.Username
	}
	if lastMsgID <= 0 {
		lastMsgID = d.LastMessageID
	}
	if err := s.DB.UpdateDialogMeta(r.Context(), db.DefaultTGAccountID, d.ChatID, title, username, lastMsgID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.UpsertChatLabel(r.Context(), d.ChatID, title, username)
	updated, err := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, d.ChatID)
	if err != nil || updated == nil {
		writeErr(w, http.StatusInternalServerError, "更新后读取失败")
		return
	}
	writeOK(w, channelListItem(s, r, *updated))
}

func (s *Server) handleGetChannel(w http.ResponseWriter, r *http.Request) {
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
	lastDL, _ := s.DB.MaxDownloadedMessageID(r.Context(), chatID)
	writeOK(w, map[string]any{
		"chatId":                  d.ChatID,
		"title":                   d.Title,
		"username":                d.Username,
		"kind":                    d.Kind,
		"messageCount":            d.MessageCount,
		"downloadedCount":         d.DownloadedCount,
		"lastMessageId":           d.LastMessageID,
		"lastDownloadedMessageId": lastDL,
		"syncedAt":                d.SyncedAt,
		"isCustom":                d.IsCustom,
		"custom":                  d.IsCustom,
	})
}

func (s *Server) handleTGSync(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 未初始化")
		return
	}
	var body struct {
		Scope string `json:"scope"` // dialogs | saved | all
	}
	_ = jsonDecodeOptional(r, &body)
	scope := body.Scope
	if scope == "" {
		scope = "all"
	}
	switch scope {
	case "dialogs":
		n, err := s.TG.SyncDialogs(r.Context())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		sum, _ := s.TG.TGSummary(r.Context())
		writeOK(w, map[string]any{"syncedDialogs": n, "summary": sum})
	case "saved":
		selfID, err := s.TG.SelfUserID(r.Context())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		n, err := s.TG.SyncSaved(r.Context(), selfID, nil)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		sum, _ := s.TG.TGSummary(r.Context())
		favID := tg.FavoritesChatID(selfID)
		downloaded, _ := s.DB.SavedDownloadedCount(r.Context(), favID)
		writeOK(w, map[string]any{"syncedSaved": n, "savedDownloaded": downloaded, "summary": sum, "favoritesChatId": favID})
	default:
		sum, err := s.TG.SyncAll(r.Context())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		selfID, _ := s.TG.SelfUserID(r.Context())
		favID := tg.FavoritesChatID(selfID)
		downloaded, _ := s.DB.SavedDownloadedCount(r.Context(), favID)
		writeOK(w, map[string]any{"summary": sum, "savedDownloaded": downloaded, "favoritesChatId": favID})
	}
}

func (s *Server) handleTGSummary(w http.ResponseWriter, r *http.Request) {
	if s.TG == nil {
		writeErr(w, http.StatusServiceUnavailable, "Telegram 未初始化")
		return
	}
	sum, err := s.TG.TGSummary(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	selfID := int64(0)
	if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
		selfID = acc.UserID
	}
	favID := tg.FavoritesChatID(selfID)
	if selfID == 0 {
		if id, err := s.TG.SelfUserID(r.Context()); err == nil {
			favID = tg.FavoritesChatID(id)
		}
	}
	downloaded, _ := s.DB.SavedDownloadedCount(r.Context(), favID)
	writeOK(w, map[string]any{
		"dialogCount":     sum.DialogCount,
		"savedCount":      sum.SavedCount,
		"dialogsSyncedAt": sum.DialogsAt,
		"savedSyncedAt":   sum.SavedAt,
		"savedDownloaded": downloaded,
		"favoritesChatId": favID,
	})
}
