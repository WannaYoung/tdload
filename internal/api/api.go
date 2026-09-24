package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"tdload/internal/auth"
	"tdload/internal/config"
	"tdload/internal/db"
	"tdload/internal/progress"
	"tdload/internal/tg"
	"tdload/internal/worker"
)

type Server struct {
	Cfg    *config.Config
	DB     *db.DB
	TG     *tg.Manager
	Hub    *progress.Hub
	Worker *worker.Worker
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	s.Register(mux)
	return mux
}

// Register 把 API 挂到已有 mux（路径以 /api 开头）。
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/auth/me", s.withAuth(s.handleMe))
	mux.HandleFunc("POST /api/auth/ticket", s.withAuth(s.handleTicket))
	mux.HandleFunc("GET /api/dashboard", s.withAuth(s.handleDashboard))
	mux.HandleFunc("GET /api/settings", s.withAuth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", s.withAuth(s.handlePutSettings))
	mux.HandleFunc("GET /api/tg/status", s.withAuth(s.handleTGStatus))
	mux.HandleFunc("POST /api/tg/login/send_code", s.withAuth(s.handleTGSendCode))
	mux.HandleFunc("POST /api/tg/login/sign_in", s.withAuth(s.handleTGSignIn))
	mux.HandleFunc("POST /api/tg/logout", s.withAuth(s.handleTGLogout))
	mux.HandleFunc("POST /api/tg/credentials/desktop", s.withAuth(s.handleTGUseDesktop))
	mux.HandleFunc("POST /api/tg/sync", s.withAuth(s.handleTGSync))
	mux.HandleFunc("GET /api/tg/summary", s.withAuth(s.handleTGSummary))
	mux.HandleFunc("GET /api/channels", s.withAuth(s.handleListChannels))
	mux.HandleFunc("POST /api/channels", s.withAuth(s.handleAddCustomChannel))
	mux.HandleFunc("GET /api/channels/{chatId}", s.withAuth(s.handleGetChannel))
	mux.HandleFunc("DELETE /api/channels/{chatId}", s.withAuth(s.handleDeleteCustomChannel))
	mux.HandleFunc("POST /api/channels/{chatId}/sync", s.withAuth(s.handleSyncCustomChannel))
	mux.HandleFunc("GET /api/channels/{chatId}/download", s.withAuth(s.handleChannelDownload))
	mux.HandleFunc("POST /api/channels/{chatId}/continue", s.withAuth(s.handleChannelContinue))
	mux.HandleFunc("POST /api/channels/{chatId}/scan-cursor", s.withAuth(s.handleChannelScanCursor))
	mux.HandleFunc("DELETE /api/channels/{chatId}/batches/completed", s.withAuth(s.handleClearChannelCompletedBatches))
	mux.HandleFunc("GET /api/tasks", s.withAuth(s.handleListTasks))
	mux.HandleFunc("POST /api/tasks", s.withAuth(s.handleCreateTasks))
	mux.HandleFunc("GET /api/tasks/{id}", s.withAuth(s.handleGetTask))
	mux.HandleFunc("POST /api/tasks/{id}/pause", s.withAuth(s.handlePauseTask))
	mux.HandleFunc("POST /api/tasks/{id}/retry", s.withAuth(s.handleRetryTask))
	mux.HandleFunc("DELETE /api/tasks/{id}", s.withAuth(s.handleDeleteTask))
	mux.HandleFunc("DELETE /api/tasks/completed", s.withAuth(s.handleClearCompleted))
	mux.HandleFunc("GET /api/tasks/items", s.withAuth(s.handleListTaskItemsByKind))
	mux.HandleFunc("DELETE /api/tasks/items/completed", s.withAuth(s.handleClearCompletedItems))
	mux.HandleFunc("DELETE /api/tasks/items/{itemId}", s.withAuth(s.handleDeleteTaskItem))
	mux.HandleFunc("GET /api/tasks/{id}/items", s.withAuth(s.handleListTaskItems))
	mux.HandleFunc("POST /api/tasks/{id}/resume", s.withAuth(s.handleResumeTask))
	mux.HandleFunc("POST /api/tasks/{id}/cancel", s.withAuth(s.handleCancelTask))
	mux.HandleFunc("POST /api/tasks/{id}/retry-failed", s.withAuth(s.handleRetryFailedTask))
	mux.HandleFunc("POST /api/tasks/pause-all", s.withAuth(s.handlePauseAllTasks))
	mux.HandleFunc("POST /api/tasks/start-all", s.withAuth(s.handleStartAllTasks))
	mux.HandleFunc("GET /api/library/filters", s.withAuth(s.handleLibraryFilters))
	mux.HandleFunc("GET /api/library", s.withAuth(s.handleListLibrary))
	mux.HandleFunc("POST /api/library/sync", s.withAuth(s.handleLibrarySync))
	mux.HandleFunc("DELETE /api/library/{id}", s.withAuth(s.handleDeleteLibrary))
	mux.HandleFunc("GET /api/library/{id}/file", s.withFileAuth(s.handleLibraryFile))
	mux.HandleFunc("GET /api/library/{id}/thumb", s.withFileAuth(s.handleLibraryThumb))
	mux.HandleFunc("GET /api/watch", s.withAuth(s.handleListWatch))
	mux.HandleFunc("GET /api/watch/candidates", s.withAuth(s.handleWatchCandidates))
	mux.HandleFunc("POST /api/watch", s.withAuth(s.handleCreateWatch))
	mux.HandleFunc("DELETE /api/watch/{id}", s.withAuth(s.handleDeleteWatch))
	mux.HandleFunc("GET /api/about", s.withAuth(s.handleAbout))
	mux.HandleFunc("GET /api/events", s.withSSEAuth(s.handleEvents))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]any{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		// 兼容 xtools 前端字段名
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	user := strings.TrimSpace(body.Username)
	if user == "" {
		user = strings.TrimSpace(body.Email)
	}
	if user == "" || body.Password == "" {
		writeErr(w, http.StatusBadRequest, "请输入用户名和密码")
		return
	}
	row, err := s.DB.FindUserByUsername(r.Context(), user)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if !auth.VerifyPassword(body.Password, row.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	u := auth.User{ID: row.ID, Username: row.Username, Role: row.Role}
	tok, err := auth.IssueToken(s.Cfg.JWTSecret, u)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	writeOK(w, map[string]any{
		"token": tok,
		"user":  u,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	writeOK(w, u)
}

func (s *Server) handleTicket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	u := userFrom(r.Context())
	tok, exp, err := auth.IssueTicket(s.Cfg.JWTSecret, u, body.Kind, 15*time.Minute)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w, map[string]any{"token": tok, "expiresAt": exp, "kind": body.Kind})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	queued, running, failed, paused, done, media, err := s.DB.DashboardCounts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	msgActive, savedActive, channelActive, _ := s.DB.DashboardActiveByKind(r.Context())
	failMsg, failSaved, failChannel, failWatch, _ := s.DB.DashboardFailedByKind(r.Context())
	tgTotal, tgActive, tgExpired, _ := s.DB.TGAccountStatus(r.Context())
	diskUsed, diskAvail, diskTotal := diskUsage(s.Cfg.DownloadDir)

	dialogCount, _ := s.DB.DialogCount(r.Context(), db.DefaultTGAccountID)
	savedCount, _ := s.DB.SavedMessageCount(r.Context(), db.DefaultTGAccountID)
	favID := int64(0)
	if acc, _ := s.DB.GetTGAccount(r.Context(), db.DefaultTGAccountID); acc != nil {
		favID = acc.UserID
	}
	savedDownloaded := 0
	if favID > 0 {
		savedDownloaded, _ = s.DB.SavedDownloadedCount(r.Context(), favID)
	}
	dialogsAt, savedAt, _ := s.DB.LastSyncedAt(r.Context(), db.DefaultTGAccountID)
	watchCount := 0
	if watches, err := s.DB.ListWatchedChats(r.Context(), db.DefaultTGAccountID); err == nil {
		watchCount = len(watches)
	}

	writeOK(w, map[string]any{
		"tgAccounts":           tgTotal,
		"tgActive":             tgActive,
		"tgExpired":            tgExpired,
		"tasksQueued":          queued,
		"tasksRunning":         running,
		"tasksFailed":          failed,
		"tasksFailedMessage":   failMsg,
		"tasksFailedSaved":     failSaved,
		"tasksFailedChannel":   failChannel,
		"tasksFailedWatch":     failWatch,
		"tasksPaused":          paused,
		"tasksDone":            done,
		"tasksMessageActive":   msgActive,
		"tasksSavedActive":     savedActive,
		"tasksChannelActive":   channelActive,
		"media":                media,
		"downloadDir":          s.Cfg.DownloadDir,
		"diskUsed":             diskUsed,
		"diskAvailable":        diskAvail,
		"diskTotal":            diskTotal,
		"tgConfigured":         s.Cfg.AppID > 0 && s.Cfg.AppHash != "",
		"watchEnabled":         watchCount > 0,
		"watchCount":           watchCount,
		"watchIntervalMinutes": s.Cfg.ClampWatchInterval(),
		"dialogCount":          dialogCount,
		"savedCount":           savedCount,
		"savedDownloaded":      savedDownloaded,
		"dialogsSyncedAt":      dialogsAt,
		"savedSyncedAt":        savedAt,
		"proxyConfigured":      strings.TrimSpace(s.Cfg.Proxy) != "",
		"threads":              s.Cfg.Threads,
		"concurrency":          s.Cfg.Concurrency,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeOK(w, settingsView(s.Cfg))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "无效请求体")
		return
	}
	applySettings(s.Cfg, body)
	if err := s.Cfg.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存配置失败")
		return
	}
	writeOK(w, settingsView(s.Cfg))
}

func settingsView(c *config.Config) map[string]any {
	return map[string]any{
		"bind":         c.Bind,
		"downloadDir":  c.DownloadDir,
		"webDir":       c.WebDir,
		"dbPath":       c.DBPath,
		"sessionDir":   c.SessionDir,
		"appId":        c.AppID,
		"appHashSet":   c.AppHash != "",
		"usingDesktopPreset": c.AppID == config.DesktopAppID && c.AppHash == config.DesktopAppHash,
		"threads":      c.Threads,
		"concurrency":  c.Concurrency,
		"skipSame":     c.SkipSame,
		"groupAlbum":   c.GroupAlbum,
		"rewriteExt":   c.RewriteExt,
		"takeout":      c.Takeout,
		"noImage":      c.NoImage,
		"watchIntervalMinutes": c.ClampWatchInterval(),
		"template":     c.Template,
		"proxy":        c.Proxy,
	}
}

func applySettings(c *config.Config, body map[string]any) {
	// downloadDir / dbPath / sessionDir 等路径仅通过配置文件或环境固定，不在设置页修改
	if v, ok := body["proxy"].(string); ok {
		c.Proxy = v
	}
	if v, ok := body["template"].(string); ok {
		c.Template = v
	}
	if v, ok := asInt(body["threads"]); ok && v > 0 {
		c.Threads = v
	}
	if v, ok := asInt(body["concurrency"]); ok && v > 0 {
		if v > 16 {
			v = 16
		}
		c.Concurrency = v
	}
	if v, ok := body["skipSame"].(bool); ok {
		c.SkipSame = v
	}
	if v, ok := body["groupAlbum"].(bool); ok {
		c.GroupAlbum = v
	}
	if v, ok := body["rewriteExt"].(bool); ok {
		c.RewriteExt = v
	}
	if v, ok := body["takeout"].(bool); ok {
		c.Takeout = v
	}
	if v, ok := body["noImage"].(bool); ok {
		c.NoImage = v
	}
	if v, ok := asInt(body["watchIntervalMinutes"]); ok {
		c.WatchIntervalMinutes = v
		c.ClampWatchInterval()
	}
	if v, ok := asInt(body["appId"]); ok && v > 0 {
		c.AppID = v
	}
	if v, ok := body["appHash"].(string); ok && strings.TrimSpace(v) != "" {
		c.AppHash = strings.TrimSpace(v)
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
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
		if claims.Typ != auth.TypSession && claims.Typ != auth.TypSSE && claims.Typ != auth.TypMedia {
			writeErr(w, http.StatusUnauthorized, "无效令牌类型")
			return
		}
		// 常规 API 需要 session；ticket 仅允许后续扩展的流式接口使用
		if claims.Typ != auth.TypSession {
			writeErr(w, http.StatusUnauthorized, "请使用会话令牌")
			return
		}
		u := auth.User{ID: claims.Sub, Username: claims.User, Role: claims.Role}
		ctx := context.WithValue(r.Context(), userKey, u)
		next(w, r.WithContext(ctx))
	}
}

func userFrom(ctx context.Context) auth.User {
	u, _ := ctx.Value(userKey).(auth.User)
	return u
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": msg})
}
