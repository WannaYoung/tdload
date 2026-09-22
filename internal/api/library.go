package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tdload/internal/db"
	"tdload/internal/tg"
)

func (s *Server) handleLibraryFilters(w http.ResponseWriter, r *http.Request) {
	selfID := int64(0)
	if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
		selfID = acc.UserID
	}
	favID := tg.FavoritesChatID(selfID)
	chats, err := s.DB.LibraryChatFilters(r.Context(), db.DefaultTGAccountID, favID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := []map[string]any{
		{"key": "saved", "chatId": favID, "title": "我的收藏"},
	}
	seen := map[int64]struct{}{favID: {}}
	for _, c := range chats {
		if _, ok := seen[c.ChatID]; ok {
			continue
		}
		seen[c.ChatID] = struct{}{}
		items = append(items, map[string]any{"key": strconv.FormatInt(c.ChatID, 10), "chatId": c.ChatID, "title": c.Title})
	}
	writeOK(w, map[string]any{"items": items})
}

func (s *Server) handleListLibrary(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	mediaType := r.URL.Query().Get("mediaType")
	if mediaType == "" {
		mediaType = "all"
	}
	q := r.URL.Query().Get("q")
	var chatFilter *int64
	if ch := r.URL.Query().Get("chat"); ch != "" && ch != "all" {
		if ch == "saved" {
			selfID := int64(0)
			if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
				selfID = acc.UserID
			}
			id := tg.FavoritesChatID(selfID)
			chatFilter = &id
		} else if id, err := strconv.ParseInt(ch, 10, 64); err == nil {
			chatFilter = &id
		}
	}
	rows, total, err := s.DB.ListMedia(r.Context(), chatFilter, mediaType, q, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		kind := "file"
		if strings.HasPrefix(m.Mime, "image/") {
			kind = "image"
		} else if strings.HasPrefix(m.Mime, "video/") {
			kind = "video"
		}
		items = append(items, map[string]any{
			"id": m.ID, "chatId": m.ChatID, "messageId": m.MessageID,
			"fileName": m.FileName, "size": m.Size, "mime": m.Mime,
			"localPath": m.LocalPath, "downloadedAt": m.DownloadedAt, "mediaKind": kind,
		})
	}
	writeOK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func (s *Server) handleLibraryFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	var path string
	err = s.DB.SQL.QueryRowContext(r.Context(), `SELECT local_path FROM media_index WHERE id=?`, id).Scan(&path)
	if err != nil || path == "" {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	path = filepath.Clean(path)
	root := filepath.Clean(s.Cfg.DownloadDir)
	if !strings.HasPrefix(path, root) {
		// 允许相对路径落在 download_dir 下
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
	}
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "文件未找到")
		return
	}
	http.ServeFile(w, r, path)
}
