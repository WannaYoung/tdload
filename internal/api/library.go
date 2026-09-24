package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tdload/internal/auth"
	"tdload/internal/db"
	"tdload/internal/library"
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
	// 多取一些再按磁盘存在过滤，避免坏索引占满一页
	fetch := pageSize * 3
	if fetch < 50 {
		fetch = 50
	}
	rows, totalAll, err := s.DB.ListMedia(r.Context(), chatFilter, mediaType, q, fetch, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		abs, err := s.resolveMediaPath(m.LocalPath)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		kind := library.DetectMediaKind(m.Mime, m.FileName)
		items = append(items, map[string]any{
			"id": m.ID, "chatId": m.ChatID, "messageId": m.MessageID,
			"fileName": m.FileName, "size": m.Size, "mime": m.Mime,
			"localPath": m.LocalPath, "downloadedAt": m.DownloadedAt, "mediaKind": kind,
		})
		if len(items) >= pageSize {
			break
		}
	}
	writeOK(w, map[string]any{
		"items": items, "total": totalAll, "page": page, "pageSize": pageSize,
		"available": len(items),
	})
}

func (s *Server) handleLibrarySync(w http.ResponseWriter, r *http.Request) {
	selfID := int64(0)
	if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
		selfID = acc.UserID
	}
	if selfID == 0 && s.TG != nil {
		if id, err := s.TG.SelfUserID(r.Context()); err == nil {
			selfID = id
		}
	}
	favID := tg.FavoritesChatID(selfID)
	res, err := library.SyncDisk(r.Context(), s.DB, s.Cfg.DownloadDir, favID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.RefreshDialogDownloadCounts(r.Context(), db.DefaultTGAccountID)
	writeOK(w, res)
}

func (s *Server) handleDeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	row, err := s.DB.GetMedia(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		writeErr(w, http.StatusNotFound, "索引不存在")
		return
	}
	deleteFile := r.URL.Query().Get("delete_file") == "1" || r.URL.Query().Get("deleteFile") == "1"
	if deleteFile {
		if abs, err := s.resolveMediaPath(row.LocalPath); err == nil {
			_ = os.Remove(abs)
		}
	}
	if err := s.DB.DeleteMedia(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.DB.SyncScanCursorForChat(r.Context(), db.DefaultTGAccountID, row.ChatID)
	_ = s.DB.RefreshDialogDownloadCounts(r.Context(), db.DefaultTGAccountID)
	writeOK(w, map[string]any{"ok": true, "deletedFile": deleteFile})
}

func (s *Server) handleLibraryFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	var localPath, mime string
	err = s.DB.SQL.QueryRowContext(r.Context(),
		`SELECT local_path, mime FROM media_index WHERE id=?`, id).Scan(&localPath, &mime)
	if err != nil || localPath == "" {
		writeErr(w, http.StatusNotFound, "索引不存在")
		return
	}
	path, err := s.resolveMediaPath(localPath)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "文件未找到（可能已被删除，索引已过期）")
		return
	}
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

// handleLibraryThumb 返回列表用缩略图（图片本地缩放；视频用 Telegram 封面缓存，缺失时按需补拉）。
func (s *Server) handleLibraryThumb(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "无效 id")
		return
	}
	row, err := s.DB.GetMedia(r.Context(), id)
	if err != nil || row == nil {
		writeErr(w, http.StatusNotFound, "索引不存在")
		return
	}
	path, err := s.resolveMediaPath(row.LocalPath)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "文件未找到")
		return
	}
	kind := library.DetectMediaKind(row.Mime, row.FileName)
	if kind != "image" && kind != "video" {
		writeErr(w, http.StatusUnsupportedMediaType, "不支持缩略图")
		return
	}
	thumb, err := library.EnsureThumb(s.Cfg.DownloadDir, path, kind, row.ChatID, row.MessageID, row.Size)
	if err != nil && kind == "video" && s.TG != nil {
		if p, ferr := s.TG.EnsureVideoThumb(r.Context(), row.ChatID, row.MessageID, row.Size, row.Mime, row.FileName); ferr == nil {
			thumb, err = p, nil
		}
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, thumb)
}

// resolveMediaPath 将 media_index.local_path 解析为绝对路径，并限制在 download_dir 下。
func (s *Server) resolveMediaPath(localPath string) (string, error) {
	root, err := filepath.Abs(s.Cfg.DownloadDir)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)

	var abs string
	if filepath.IsAbs(localPath) {
		abs = filepath.Clean(localPath)
	} else {
		abs, err = filepath.Abs(localPath)
		if err != nil {
			return "", err
		}
		abs = filepath.Clean(abs)
		// 若相对路径写成了 "downloads/xxx" 而 Abs 落到 cwd，仍校验在 root 下
	}
	sep := string(os.PathSeparator)
	if abs != root && !strings.HasPrefix(abs, root+sep) {
		// 再试：把相对路径直接拼到 download_dir（去掉重复的 downloads/ 前缀）
		rel := filepath.Clean(localPath)
		base := filepath.Base(root)
		if strings.HasPrefix(rel, base+sep) {
			rel = strings.TrimPrefix(rel, base+sep)
		}
		abs = filepath.Clean(filepath.Join(root, rel))
		if abs != root && !strings.HasPrefix(abs, root+sep) {
			return "", fmt.Errorf("路径不在下载目录内")
		}
	}
	return abs, nil
}

// withFileAuth 允许会话 JWT 或 media ticket（便于 <img src>）。
func (s *Server) withFileAuth(next http.HandlerFunc) http.HandlerFunc {
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
		if claims.Typ != auth.TypSession && claims.Typ != auth.TypMedia {
			writeErr(w, http.StatusUnauthorized, "无效令牌类型")
			return
		}
		next(w, r)
	}
}
