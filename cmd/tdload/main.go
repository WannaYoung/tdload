package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"tdload/internal/api"
	"tdload/internal/config"
	"tdload/internal/db"
	"tdload/internal/progress"
	"tdload/internal/static"
	"tdload/internal/tg"
	"tdload/internal/watcher"
	"tdload/internal/worker"
)

func main() {
	_ = godotenv.Load()

	cfgPath := envOr("CONFIG_PATH", "./config/config.yaml")
	if abs, err := filepath.Abs(cfgPath); err == nil {
		cfgPath = abs
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx := context.Background()
	if err := database.EnsureAdmin(ctx, os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")); err != nil {
		slog.Error("ensure admin", "err", err)
		os.Exit(1)
	}

	hub := progress.NewHub()
	tgMgr := tg.NewManager(cfg, database)
	wrk := worker.New(cfg, database, tgMgr, hub)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	wrk.Start(workerCtx)
	watch := watcher.New(cfg, database, tgMgr, wrk, hub)
	watch.Start(workerCtx)

	srvAPI := &api.Server{Cfg: cfg, DB: database, TG: tgMgr, Hub: hub, Worker: wrk}
	mux := http.NewServeMux()
	srvAPI.Register(mux)
	if static.Exists(cfg.WebDir) {
		mux.Handle("/", static.FileServer(cfg.WebDir))
		slog.Info("serving web", "dir", cfg.WebDir)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "web UI not built; run pnpm build in web/ or set WEB_DIR", http.StatusServiceUnavailable)
		})
		slog.Warn("web dir missing", "dir", cfg.WebDir)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Bind,
		Handler:           withLog(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("tdload listening", "addr", cfg.Bind, "config", cfgPath)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	workerCancel()
	wrk.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	slog.Info("shutdown complete")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if stringsHasPrefix(r.URL.Path, "/api/") {
			slog.Info("req", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).String())
		}
	})
}

func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
