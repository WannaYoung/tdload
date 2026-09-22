package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Task struct {
	ID           int64
	Source       string
	Title        string
	Status       string
	TGAccountID  sql.NullInt64
	OptionsJSON  string
	TotalBytes   int64
	DoneBytes    int64
	TotalFiles   int
	DoneFiles    int
	SpeedBPS     int64
	Error        string
	CreatedAt    string
	StartedAt    sql.NullString
	FinishedAt   sql.NullString
	URLs         []string `json:"-"`
}

type TaskItem struct {
	ID        int64
	TaskID    int64
	ChatID    int64
	MessageID int
	FileName  string
	Size      int64
	Status    string
	LocalPath string
	Error     string
}

func (d *DB) CreateTask(ctx context.Context, source, title, optionsJSON string, totalFiles int) (*Task, error) {
	if title == "" {
		title = source
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO tasks (source, title, status, tg_account_id, options_json, total_files, created_at)
VALUES (?, ?, 'queued', 1, ?, ?, ?)`, source, title, optionsJSON, totalFiles, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetTask(ctx, id)
}

func (d *DB) CreateURLTask(ctx context.Context, urls []string, title string) (*Task, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("没有有效链接")
	}
	if title == "" {
		title = fmt.Sprintf("%d 条消息", len(urls))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	opt, _ := json.Marshal(map[string]any{"urls": urls})
	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO tasks (source, title, status, tg_account_id, options_json, total_files, created_at)
VALUES ('url', ?, 'queued', 1, ?, ?, ?)`, title, string(opt), len(urls), now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetTask(ctx, id)
}

func (d *DB) GetTask(ctx context.Context, id int64) (*Task, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, source, title, status, tg_account_id, options_json,
       total_bytes, done_bytes, total_files, done_files, speed_bps, error,
       created_at, started_at, finished_at
FROM tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if err != nil {
		return nil, err
	}
	t.URLs = parseURLs(t.OptionsJSON)
	return t, nil
}

func sourcesForKind(kind string) []string {
	switch kind {
	case "message":
		return []string{"url"}
	case "saved":
		return []string{"saved_all"}
	case "channel":
		return []string{"chat_continue", "chat_batch", "chat_range"}
	default:
		return nil
	}
}

func (d *DB) ListTasks(ctx context.Context, kind string, limit, offset int) ([]*Task, int, error) {
	if limit <= 0 {
		limit = 50
	}
	sources := sourcesForKind(kind)
	var total int
	var rows *sql.Rows
	var err error
	if len(sources) == 0 {
		if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM tasks`).Scan(&total); err != nil {
			return nil, 0, err
		}
		rows, err = d.SQL.QueryContext(ctx, `
SELECT id, source, title, status, tg_account_id, options_json,
       total_bytes, done_bytes, total_files, done_files, speed_bps, error,
       created_at, started_at, finished_at
FROM tasks ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	} else {
		q, args := inClause(`SELECT COUNT(1) FROM tasks WHERE source IN (`, sources)
		if err := d.SQL.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
		q, args = inClause(`
SELECT id, source, title, status, tg_account_id, options_json,
       total_bytes, done_bytes, total_files, done_files, speed_bps, error,
       created_at, started_at, finished_at
FROM tasks WHERE source IN (`, sources)
		args = append(args, limit, offset)
		rows, err = d.SQL.QueryContext(ctx, q+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		t.URLs = parseURLs(t.OptionsJSON)
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (d *DB) ListQueuedTaskIDs(ctx context.Context) ([]int64, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id FROM tasks WHERE status IN ('queued','running') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) UpdateTaskStatus(ctx context.Context, id int64, status, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	switch status {
	case "running":
		_, err := d.SQL.ExecContext(ctx, `
UPDATE tasks SET status=?, error=?, started_at=COALESCE(started_at, ?), finished_at=NULL WHERE id=?`,
			status, errMsg, now, id)
		return err
	case "done", "failed", "cancelled", "paused":
		_, err := d.SQL.ExecContext(ctx, `
UPDATE tasks SET status=?, error=?, finished_at=? WHERE id=?`, status, errMsg, now, id)
		return err
	default:
		_, err := d.SQL.ExecContext(ctx, `UPDATE tasks SET status=?, error=? WHERE id=?`, status, errMsg, id)
		return err
	}
}

func (d *DB) UpdateTaskProgress(ctx context.Context, id int64, doneFiles, totalFiles int, doneBytes, totalBytes, speed int64) error {
	_, err := d.SQL.ExecContext(ctx, `
UPDATE tasks SET done_files=?, total_files=?, done_bytes=?, total_bytes=?, speed_bps=? WHERE id=?`,
		doneFiles, totalFiles, doneBytes, totalBytes, speed, id)
	return err
}

func (d *DB) AddTaskLog(ctx context.Context, taskID int64, level, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx,
		`INSERT INTO task_logs (task_id, level, message, created_at) VALUES (?, ?, ?, ?)`,
		taskID, level, message, now)
	return err
}

func (d *DB) InsertTaskItem(ctx context.Context, item *TaskItem) (int64, error) {
	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO task_items (task_id, chat_id, message_id, file_name, size, status, local_path, error)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id, chat_id, message_id) DO UPDATE SET
  file_name=excluded.file_name, size=excluded.size, status=excluded.status,
  local_path=excluded.local_path, error=excluded.error`,
		item.TaskID, item.ChatID, item.MessageID, item.FileName, item.Size, item.Status, item.LocalPath, item.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpsertMedia(ctx context.Context, chatID int64, messageID int, fileName string, size int64, localPath, mime string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO media_index (chat_id, message_id, file_name, size, local_path, mime, downloaded_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chat_id, message_id, size) DO UPDATE SET
  file_name=excluded.file_name, local_path=excluded.local_path, mime=excluded.mime, downloaded_at=excluded.downloaded_at`,
		chatID, messageID, fileName, size, localPath, mime, now)
	return err
}

func (d *DB) MediaExists(ctx context.Context, chatID int64, messageID int, size int64) (bool, string, error) {
	var path string
	err := d.SQL.QueryRowContext(ctx, `
SELECT local_path FROM media_index WHERE chat_id=? AND message_id=? AND size=?`,
		chatID, messageID, size).Scan(&path)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, path, nil
}

func (d *DB) DeleteTask(ctx context.Context, id int64) error {
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM tasks WHERE id=?`, id)
	return err
}

func (d *DB) ClearCompletedTasks(ctx context.Context) error {
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM tasks WHERE status IN ('done','cancelled')`)
	return err
}

func inClause(prefix string, sources []string) (string, []any) {
	q := prefix
	args := make([]any, len(sources))
	for i, s := range sources {
		if i > 0 {
			q += ","
		}
		q += "?"
		args[i] = s
	}
	q += ")"
	return q, args
}

type ItemCounts struct {
	Pending     int `json:"pending"`
	Downloading int `json:"downloading"`
	Done        int `json:"done"`
	Skipped     int `json:"skipped"`
	Failed      int `json:"failed"`
}

func (d *DB) TaskItemCounts(ctx context.Context, taskID int64) (ItemCounts, error) {
	var c ItemCounts
	err := d.SQL.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='downloading' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='skipped' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0)
FROM task_items WHERE task_id=?`, taskID).Scan(&c.Pending, &c.Downloading, &c.Done, &c.Skipped, &c.Failed)
	return c, err
}

func (d *DB) ListTaskItems(ctx context.Context, taskID int64, limit, offset int) ([]TaskItem, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM task_items WHERE task_id=?`, taskID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, task_id, chat_id, message_id, file_name, size, status, local_path, error
FROM task_items WHERE task_id=? ORDER BY id LIMIT ? OFFSET ?`, taskID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []TaskItem
	for rows.Next() {
		var it TaskItem
		if err := rows.Scan(&it.ID, &it.TaskID, &it.ChatID, &it.MessageID, &it.FileName, &it.Size, &it.Status, &it.LocalPath, &it.Error); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (d *DB) ListTaskItemsByKind(ctx context.Context, kind string, limit, offset int) ([]TaskItem, int, error) {
	sources := sourcesForKind(kind)
	if len(sources) == 0 {
		return nil, 0, fmt.Errorf("unknown kind")
	}
	if limit <= 0 {
		limit = 50
	}
	qCount, args := inClause(`SELECT COUNT(1) FROM task_items ti JOIN tasks t ON t.id=ti.task_id WHERE t.source IN (`, sources)
	var total int
	if err := d.SQL.QueryRowContext(ctx, qCount, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q, args := inClause(`
SELECT ti.id, ti.task_id, ti.chat_id, ti.message_id, ti.file_name, ti.size, ti.status, ti.local_path, ti.error
FROM task_items ti JOIN tasks t ON t.id=ti.task_id WHERE t.source IN (`, sources)
	args = append(args, limit, offset)
	rows, err := d.SQL.QueryContext(ctx, q+` ORDER BY ti.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []TaskItem
	for rows.Next() {
		var it TaskItem
		if err := rows.Scan(&it.ID, &it.TaskID, &it.ChatID, &it.MessageID, &it.FileName, &it.Size, &it.Status, &it.LocalPath, &it.Error); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (d *DB) RetryFailedTaskItems(ctx context.Context, taskID int64) (int64, error) {
	res, err := d.SQL.ExecContext(ctx, `
UPDATE task_items SET status='pending', error='' WHERE task_id=? AND status='failed'`, taskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (*Task, error) {
	var t Task
	err := row.Scan(
		&t.ID, &t.Source, &t.Title, &t.Status, &t.TGAccountID, &t.OptionsJSON,
		&t.TotalBytes, &t.DoneBytes, &t.TotalFiles, &t.DoneFiles, &t.SpeedBPS, &t.Error,
		&t.CreatedAt, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func parseURLs(optionsJSON string) []string {
	var m struct {
		URLs []string `json:"urls"`
	}
	_ = json.Unmarshal([]byte(optionsJSON), &m)
	out := make([]string, 0, len(m.URLs))
	for _, u := range m.URLs {
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, u)
		}
	}
	return out
}
