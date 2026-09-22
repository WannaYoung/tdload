package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tdload/internal/db"
	"tdload/internal/tg"
)

func jsonDecodeOptional(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
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
		lastDL, _ := s.DB.MaxDownloadedMessageID(r.Context(), d.ChatID)
		views = append(views, map[string]any{
			"chatId":                  d.ChatID,
			"title":                   d.Title,
			"username":                d.Username,
			"kind":                    d.Kind,
			"messageCount":            d.MessageCount,
			"downloadedCount":         d.DownloadedCount,
			"lastMessageId":           d.LastMessageID,
			"lastDownloadedMessageId": lastDL,
			"syncedAt":                d.SyncedAt,
		})
	}
	writeOK(w, map[string]any{
		"items": views, "total": total, "page": page, "pageSize": pageSize,
	})
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
		n, err := s.TG.SyncSaved(r.Context(), selfID)
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
