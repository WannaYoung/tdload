package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type WatchedChat struct {
	ID            int64
	TGAccountID   int64
	ChatID        int64
	ChatTitle     string
	Enabled       bool
	LastMessageID int
	FilterJSON    string
	LastRunAt     string
	NextRunAt     string
	CreatedAt     string
	UpdatedAt     string
}

func (d *DB) ListWatchedChats(ctx context.Context, accountID int64) ([]WatchedChat, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, tg_account_id, chat_id, chat_title, enabled, last_message_id, filter_json,
       last_run_at, next_run_at, created_at, updated_at
FROM watched_chats WHERE tg_account_id=?
ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WatchedChat
	for rows.Next() {
		var w WatchedChat
		var en int
		if err := rows.Scan(&w.ID, &w.TGAccountID, &w.ChatID, &w.ChatTitle, &en, &w.LastMessageID,
			&w.FilterJSON, &w.LastRunAt, &w.NextRunAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		w.Enabled = en != 0
		out = append(out, w)
	}
	return out, rows.Err()
}

func (d *DB) GetWatchedChat(ctx context.Context, id int64) (*WatchedChat, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, tg_account_id, chat_id, chat_title, enabled, last_message_id, filter_json,
       last_run_at, next_run_at, created_at, updated_at
FROM watched_chats WHERE id=?`, id)
	var w WatchedChat
	var en int
	err := row.Scan(&w.ID, &w.TGAccountID, &w.ChatID, &w.ChatTitle, &en, &w.LastMessageID,
		&w.FilterJSON, &w.LastRunAt, &w.NextRunAt, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.Enabled = en != 0
	return &w, nil
}

func (d *DB) GetWatchedByChatID(ctx context.Context, accountID, chatID int64) (*WatchedChat, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, tg_account_id, chat_id, chat_title, enabled, last_message_id, filter_json,
       last_run_at, next_run_at, created_at, updated_at
FROM watched_chats WHERE tg_account_id=? AND chat_id=?`, accountID, chatID)
	var w WatchedChat
	var en int
	err := row.Scan(&w.ID, &w.TGAccountID, &w.ChatID, &w.ChatTitle, &en, &w.LastMessageID,
		&w.FilterJSON, &w.LastRunAt, &w.NextRunAt, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.Enabled = en != 0
	return &w, nil
}

func (d *DB) InsertWatchedChat(ctx context.Context, accountID, chatID int64, title string, lastMessageID int, nextRunAt, filterJSON string) (*WatchedChat, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(filterJSON) == "" {
		filterJSON = "{}"
	}
	res, err := d.SQL.ExecContext(ctx, `
INSERT INTO watched_chats (
  tg_account_id, chat_id, chat_title, enabled, last_message_id, filter_json,
  last_run_at, next_run_at, created_at, updated_at
) VALUES (?, ?, ?, 1, ?, ?, '', ?, ?, ?)`,
		accountID, chatID, title, lastMessageID, filterJSON, nextRunAt, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return d.GetWatchedChat(ctx, id)
}

func (d *DB) DeleteWatchedChat(ctx context.Context, id int64) error {
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM watched_chats WHERE id=?`, id)
	return err
}

func (d *DB) UpdateWatchedSchedule(ctx context.Context, id int64, lastRunAt, nextRunAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
UPDATE watched_chats SET last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		lastRunAt, nextRunAt, now, id)
	return err
}

func (d *DB) AdvanceWatchedCursor(ctx context.Context, id int64, lastMessageID int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
UPDATE watched_chats SET last_message_id=CASE
  WHEN ? > last_message_id THEN ?
  ELSE last_message_id
END, updated_at=? WHERE id=?`, lastMessageID, lastMessageID, now, id)
	return err
}

func (d *DB) UpdateWatchedAfterRun(ctx context.Context, id int64, lastMessageID int, lastRunAt, nextRunAt string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
UPDATE watched_chats SET last_message_id=?, last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		lastMessageID, lastRunAt, nextRunAt, now, id)
	return err
}

func (d *DB) ListDueWatches(ctx context.Context, accountID int64, nowRFC3339 string) ([]WatchedChat, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, tg_account_id, chat_id, chat_title, enabled, last_message_id, filter_json,
       last_run_at, next_run_at, created_at, updated_at
FROM watched_chats
WHERE tg_account_id=? AND enabled=1 AND (next_run_at='' OR next_run_at<=?)
ORDER BY id ASC`, accountID, nowRFC3339)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WatchedChat
	for rows.Next() {
		var w WatchedChat
		var en int
		if err := rows.Scan(&w.ID, &w.TGAccountID, &w.ChatID, &w.ChatTitle, &en, &w.LastMessageID,
			&w.FilterJSON, &w.LastRunAt, &w.NextRunAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		w.Enabled = en != 0
		out = append(out, w)
	}
	return out, rows.Err()
}

func (d *DB) WatchedChatIDs(ctx context.Context, accountID int64) (map[int64]struct{}, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT chat_id FROM watched_chats WHERE tg_account_id=?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]struct{}{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (d *DB) HasActiveWatchTask(ctx context.Context, watchID int64) (bool, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `
SELECT COUNT(1) FROM tasks
WHERE status IN ('queued','running','paused')
  AND source IN ('watch','watch_saved')
  AND json_extract(options_json, '$.watchId') = ?`, watchID).Scan(&n)
	return n > 0, err
}

func (d *DB) MaxSavedMessageID(ctx context.Context, accountID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `
SELECT MAX(message_id) FROM saved_messages_cache WHERE tg_account_id=?`, accountID).Scan(&n)
	if err != nil || !n.Valid {
		return 0, err
	}
	return int(n.Int64), nil
}

func (d *DB) ListSavedMessageIDsInRange(ctx context.Context, accountID int64, fromID, toID int) ([]int, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT message_id FROM saved_messages_cache
WHERE tg_account_id=? AND message_id>=? AND message_id<=?
ORDER BY message_id`, accountID, fromID, toID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (d *DB) MediaCountForChat(ctx context.Context, chatID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT message_id) FROM media_index WHERE chat_id=?`, chatID).Scan(&n)
	return n, err
}
