package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"tdload/internal/auth"

	_ "modernc.org/sqlite"
)

type DB struct {
	SQL *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	d := &DB{SQL: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.SQL.Close()
}

func (d *DB) migrate() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tg_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  label TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  user_id INTEGER NOT NULL DEFAULT 0,
  session_file TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  proxy TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  tg_account_id INTEGER,
  options_json TEXT NOT NULL DEFAULT '{}',
  total_bytes INTEGER NOT NULL DEFAULT 0,
  done_bytes INTEGER NOT NULL DEFAULT 0,
  total_files INTEGER NOT NULL DEFAULT 0,
  done_files INTEGER NOT NULL DEFAULT 0,
  speed_bps INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT
);

CREATE TABLE IF NOT EXISTS task_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL,
  chat_id INTEGER NOT NULL DEFAULT 0,
  message_id INTEGER NOT NULL DEFAULT 0,
  file_name TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  local_path TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  UNIQUE(task_id, chat_id, message_id),
  FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_index (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL,
  file_name TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  local_path TEXT NOT NULL DEFAULT '',
  mime TEXT NOT NULL DEFAULT '',
  downloaded_at TEXT NOT NULL,
  UNIQUE(chat_id, message_id, size)
);

CREATE TABLE IF NOT EXISTS task_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL,
  level TEXT NOT NULL DEFAULT 'info',
  message TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS watched_chats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tg_account_id INTEGER NOT NULL,
  chat_id INTEGER NOT NULL,
  chat_title TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_message_id INTEGER NOT NULL DEFAULT 0,
  filter_json TEXT NOT NULL DEFAULT '{}',
  last_run_at TEXT NOT NULL DEFAULT '',
  next_run_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(tg_account_id, chat_id)
);

CREATE TABLE IF NOT EXISTS tg_dialogs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tg_account_id INTEGER NOT NULL,
  chat_id INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT -1,
  downloaded_count INTEGER NOT NULL DEFAULT 0,
  last_message_id INTEGER NOT NULL DEFAULT 0,
  synced_at TEXT NOT NULL,
  UNIQUE(tg_account_id, chat_id)
);

CREATE TABLE IF NOT EXISTS saved_messages_cache (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tg_account_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL,
  source_chat_id INTEGER NOT NULL DEFAULT 0,
  synced_at TEXT NOT NULL,
  UNIQUE(tg_account_id, message_id)
);

CREATE TABLE IF NOT EXISTS chat_download_state (
  tg_account_id INTEGER NOT NULL,
  chat_id INTEGER NOT NULL,
  last_downloaded_message_id INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tg_account_id, chat_id)
);
`)
	if err != nil {
		return err
	}
	if err := d.ensureWatchColumns(); err != nil {
		return err
	}
	return d.ensureChatDownloadStateColumns()
}

func (d *DB) ensureChatDownloadStateColumns() error {
	cols := []struct{ name, ddl string }{
		{"batch_size", `ALTER TABLE chat_download_state ADD COLUMN batch_size INTEGER NOT NULL DEFAULT 0`},
	}
	for _, c := range cols {
		var n int
		err := d.SQL.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('chat_download_state') WHERE name=?`, c.name).Scan(&n)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if _, err := d.SQL.Exec(c.ddl); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) ensureWatchColumns() error {
	cols := []struct{ name, ddl string }{
		{"last_run_at", `ALTER TABLE watched_chats ADD COLUMN last_run_at TEXT NOT NULL DEFAULT ''`},
		{"next_run_at", `ALTER TABLE watched_chats ADD COLUMN next_run_at TEXT NOT NULL DEFAULT ''`},
	}
	for _, c := range cols {
		var n int
		err := d.SQL.QueryRow(`SELECT COUNT(1) FROM pragma_table_info('watched_chats') WHERE name=?`, c.name).Scan(&n)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if _, err := d.SQL.Exec(c.ddl); err != nil {
			return err
		}
	}
	return nil
}

type UserRow struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
}

func (d *DB) EnsureAdmin(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return fmt.Errorf("ADMIN_USERNAME / ADMIN_PASSWORD 必填")
	}
	var n int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = d.SQL.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, 'admin', ?)`,
		username, hash, now,
	)
	return err
}

func (d *DB) FindUserByUsername(ctx context.Context, username string) (*UserRow, error) {
	row := d.SQL.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role FROM users WHERE username = ?`, username)
	var u UserRow
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) FindUserByID(ctx context.Context, id int64) (*UserRow, error) {
	row := d.SQL.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role FROM users WHERE id = ?`, id)
	var u UserRow
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) DashboardCounts(ctx context.Context) (queued, running, failed, paused, done, media int, err error) {
	err = d.SQL.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN status='queued' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='running' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='paused' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0)
FROM tasks`).Scan(&queued, &running, &failed, &paused, &done)
	if err != nil {
		return
	}
	err = d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM media_index`).Scan(&media)
	return
}

// DashboardActiveByKind 返回各业务线排队+下载中任务数（不含 paused）。
func (d *DB) DashboardActiveByKind(ctx context.Context) (message, saved, channel int, err error) {
	err = d.SQL.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN source='url' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN source IN ('saved_all','watch_saved') THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN source IN ('chat_continue','chat_batch','chat_range','watch') THEN 1 ELSE 0 END), 0)
FROM tasks
WHERE status IN ('queued','running')`).Scan(&message, &saved, &channel)
	return
}

func (d *DB) TGAccountStatus(ctx context.Context) (total, active, expired int, err error) {
	err = d.SQL.QueryRowContext(ctx, `
SELECT
  COUNT(1),
  COALESCE(SUM(CASE WHEN status='active' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status IN ('expired','error') THEN 1 ELSE 0 END), 0)
FROM tg_accounts`).Scan(&total, &active, &expired)
	return
}

type TGAccount struct {
	ID          int64
	Label       string
	Phone       string
	UserID      int64
	SessionFile string
	Status      string
	Proxy       string
	LastError   string
}

func (d *DB) GetTGAccount(ctx context.Context, id int64) (*TGAccount, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, label, phone, user_id, session_file, status, proxy, last_error
FROM tg_accounts WHERE id = ?`, id)
	var a TGAccount
	err := row.Scan(&a.ID, &a.Label, &a.Phone, &a.UserID, &a.SessionFile, &a.Status, &a.Proxy, &a.LastError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *DB) UpsertTGAccount(ctx context.Context, a *TGAccount) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO tg_accounts (id, label, phone, user_id, session_file, status, proxy, last_error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  label=excluded.label,
  phone=CASE WHEN excluded.phone != '' THEN excluded.phone ELSE tg_accounts.phone END,
  user_id=CASE WHEN excluded.user_id != 0 THEN excluded.user_id ELSE tg_accounts.user_id END,
  session_file=CASE WHEN excluded.session_file != '' THEN excluded.session_file ELSE tg_accounts.session_file END,
  status=excluded.status,
  proxy=CASE WHEN excluded.proxy != '' THEN excluded.proxy ELSE tg_accounts.proxy END,
  last_error=excluded.last_error,
  updated_at=excluded.updated_at
`, a.ID, a.Label, a.Phone, a.UserID, a.SessionFile, a.Status, a.Proxy, a.LastError, now, now)
	return err
}

func dirOf(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return "."
	}
	return path[:i]
}
