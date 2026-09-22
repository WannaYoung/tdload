package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"tdload/internal/db"
	"tdload/internal/tg"
)

func (s *Server) handleListWatch(w http.ResponseWriter, r *http.Request) {
	favID := s.favoritesChatID(r)
	items, err := s.DB.ListWatchedChats(r.Context(), db.DefaultTGAccountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, it := range items {
		isFav := favID > 0 && it.ChatID == favID
		title := it.ChatTitle
		if isFav {
			title = "我的收藏"
		} else if title == "" {
			if d, _ := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, it.ChatID); d != nil {
				title = d.Title
			}
		}
		downloaded, _ := s.DB.MediaCountForChat(r.Context(), it.ChatID)
		latest := 0
		if isFav {
			latest, _ = s.DB.MaxSavedMessageID(r.Context(), db.DefaultTGAccountID)
		} else if d, _ := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, it.ChatID); d != nil {
			latest = d.LastMessageID
		}
		views = append(views, map[string]any{
			"id":              it.ID,
			"chatId":          it.ChatID,
			"chatTitle":       title,
			"isFavorites":     isFav,
			"enabled":         it.Enabled,
			"lastMessageId":   latest,
			"cursorMessageId": it.LastMessageID,
			"downloadedCount": downloaded,
			"lastRunAt":       it.LastRunAt,
			"nextRunAt":       it.NextRunAt,
			"createdAt":       it.CreatedAt,
		})
	}
	// 我的收藏置顶，其余按创建时间
	sort.SliceStable(views, func(i, j int) bool {
		ai, _ := views[i]["isFavorites"].(bool)
		aj, _ := views[j]["isFavorites"].(bool)
		if ai != aj {
			return ai
		}
		return false
	})
	writeOK(w, map[string]any{
		"items":                  views,
		"watchIntervalMinutes":   s.Cfg.ClampWatchInterval(),
		"favoritesChatId":        favID,
	})
}

func (s *Server) handleWatchCandidates(w http.ResponseWriter, r *http.Request) {
	favID := s.favoritesChatID(r)
	watched, err := s.DB.WatchedChatIDs(r.Context(), db.DefaultTGAccountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type opt struct {
		ChatID   int64  `json:"chatId"`
		Title    string `json:"title"`
		Kind     string `json:"kind"`
		Username string `json:"username"`
	}
	out := make([]opt, 0)
	if favID > 0 {
		if _, ok := watched[favID]; !ok {
			out = append(out, opt{ChatID: favID, Title: "我的收藏", Kind: "saved"})
		}
	}
	dialogs, _, err := s.DB.ListTGDialogs(r.Context(), db.DefaultTGAccountID, 5000, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, d := range dialogs {
		if _, ok := watched[d.ChatID]; ok {
			continue
		}
		if favID > 0 && d.ChatID == favID {
			continue
		}
		title := d.Title
		if title == "" {
			title = formatInt64(d.ChatID)
		}
		out = append(out, opt{ChatID: d.ChatID, Title: title, Kind: d.Kind, Username: d.Username})
	}
	writeOK(w, map[string]any{"items": out, "favoritesChatId": favID})
}

func (s *Server) handleCreateWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChatID int64 `json:"chatId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ChatID == 0 {
		writeErr(w, http.StatusBadRequest, "请选择频道")
		return
	}
	if existing, _ := s.DB.GetWatchedByChatID(r.Context(), db.DefaultTGAccountID, body.ChatID); existing != nil {
		writeErr(w, http.StatusConflict, "该频道已在监听列表中")
		return
	}

	favID := s.favoritesChatID(r)
	isFav := favID > 0 && body.ChatID == favID
	title := ""
	latest := 0
	if isFav {
		title = "我的收藏"
		latest, _ = s.DB.MaxSavedMessageID(r.Context(), db.DefaultTGAccountID)
		if latest == 0 && s.TG != nil {
			selfID := int64(0)
			if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
				selfID = acc.UserID
			}
			if selfID == 0 {
				if id, err := s.TG.SelfUserID(r.Context()); err == nil {
					selfID = id
				}
			}
			if selfID > 0 {
				_, _ = s.TG.SyncSaved(r.Context(), selfID)
				latest, _ = s.DB.MaxSavedMessageID(r.Context(), db.DefaultTGAccountID)
			}
		}
	} else {
		d, _ := s.DB.GetTGDialog(r.Context(), db.DefaultTGAccountID, body.ChatID)
		if d == nil {
			writeErr(w, http.StatusBadRequest, "频道不存在，请先在 Telegram 页同步对话")
			return
		}
		title = d.Title
		if title == "" {
			title = d.Username
		}
		if title == "" {
			title = formatInt64(body.ChatID)
		}
		latest = d.LastMessageID
	}

	interval := time.Duration(s.Cfg.ClampWatchInterval()) * time.Minute
	nextRun := time.Now().UTC().Add(interval).Format(time.RFC3339)
	row, err := s.DB.InsertWatchedChat(r.Context(), db.DefaultTGAccountID, body.ChatID, title, latest, nextRun, "{}")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	downloaded, _ := s.DB.MediaCountForChat(r.Context(), body.ChatID)
	writeOK(w, map[string]any{
		"id":              row.ID,
		"chatId":          row.ChatID,
		"chatTitle":       title,
		"isFavorites":     isFav,
		"lastMessageId":   latest,
		"downloadedCount": downloaded,
		"nextRunAt":       nextRun,
	})
}

func (s *Server) handleDeleteWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	row, _ := s.DB.GetWatchedChat(r.Context(), id)
	if row == nil {
		writeErr(w, http.StatusNotFound, "监听不存在")
		return
	}
	if err := s.DB.DeleteWatchedChat(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (s *Server) favoritesChatID(r *http.Request) int64 {
	if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil && acc.UserID > 0 {
		return tg.FavoritesChatID(acc.UserID)
	}
	if s.TG != nil {
		if id, err := s.TG.SelfUserID(r.Context()); err == nil && id > 0 {
			return tg.FavoritesChatID(id)
		}
	}
	return 0
}

func formatInt64(n int64) string {
	return jsonNumber(n)
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
