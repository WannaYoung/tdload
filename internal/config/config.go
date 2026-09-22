package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Telegram Desktop 公开凭证（与 iyear/tdl AppDesktop 相同），默认启用。
const (
	DesktopAppID   = 2040
	DesktopAppHash = "b18441a1ff607e10a989891a5462e627"
)

type Config struct {
	Bind         string `yaml:"bind"`
	DownloadDir  string `yaml:"download_dir"`
	WebDir       string `yaml:"web_dir"`
	DBPath       string `yaml:"db_path"`
	SessionDir   string `yaml:"session_dir"`
	AppID        int    `yaml:"app_id"`
	AppHash      string `yaml:"app_hash"`
	Threads      int    `yaml:"threads"`
	Concurrency  int    `yaml:"concurrency"`
	SkipSame     bool   `yaml:"skip_same"`
	GroupAlbum   bool   `yaml:"group_album"`
	RewriteExt   bool   `yaml:"rewrite_ext"`
	Takeout      bool   `yaml:"takeout"`
	NoImage      bool   `yaml:"no_image"` // 界面：资源库等不加载预览图
	WatchIntervalMinutes int `yaml:"watch_interval_minutes"` // 监听轮询间隔（分钟）
	Template     string `yaml:"template"`
	Proxy        string `yaml:"proxy"`
	JWTSecret    string `yaml:"jwt_secret"`
	path         string `yaml:"-"`
}

func Default() *Config {
	return &Config{
		Bind:        "0.0.0.0:3030",
		DownloadDir: "/tdload/downloads",
		WebDir:      "/app/web",
		DBPath:      "/tdload/config/tdload.db",
		SessionDir:  "/tdload/config/session",
		Threads:     8,
		Concurrency: 4,
		SkipSame:    true,
		GroupAlbum:  true,
		WatchIntervalMinutes: 30,
		Template:    "{{ .DialogID }}_{{ .MessageID }}_{{ .FileName }}",
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// 首次：从 example 语义创建空配置文件
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("create config: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		cfg.path = path
	}

	cfg.applyEnv()
	if err := cfg.ensureSecrets(); err != nil {
		return nil, err
	}
	if err := cfg.ensureDirs(); err != nil {
		return nil, err
	}
	cfg.ClampWatchInterval()
	return cfg, nil
}

func (c *Config) Path() string { return c.path }

// ClampWatchInterval 将监听间隔限制在 10–300 分钟。
func (c *Config) ClampWatchInterval() int {
	v := c.WatchIntervalMinutes
	if v <= 0 {
		v = 30
	}
	if v < 10 {
		v = 10
	}
	if v > 300 {
		v = 300
	}
	c.WatchIntervalMinutes = v
	return v
}

func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("config path empty")
	}
	if err := os.MkdirAll(dirOf(c.path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, out, 0o600)
}

func (c *Config) applyEnv() {
	if v := os.Getenv("BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("WEB_DIR"); v != "" {
		c.WebDir = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	if v := os.Getenv("TG_APP_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.AppID = n
		}
	}
	if v := os.Getenv("TG_APP_HASH"); v != "" {
		c.AppHash = v
	}
}

func (c *Config) ensureSecrets() error {
	changed := false
	if strings.TrimSpace(c.JWTSecret) == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		c.JWTSecret = hex.EncodeToString(b)
		changed = true
	}
	// 默认使用 Telegram Desktop 公开凭证（与 tdl 一致），免去 my.telegram.org
	if c.AppID == 0 || strings.TrimSpace(c.AppHash) == "" {
		c.AppID = DesktopAppID
		c.AppHash = DesktopAppHash
		changed = true
	}
	if changed {
		return c.Save()
	}
	return nil
}

func (c *Config) ensureDirs() error {
	for _, d := range []string{c.DownloadDir, c.SessionDir, dirOf(c.DBPath)} {
		if d == "" || d == "." {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func dirOf(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return "."
	}
	return path[:i]
}
