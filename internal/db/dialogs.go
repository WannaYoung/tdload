package db

import (
	"context"
	"database/sql"
	"time"
)

const DefaultTGAccountID int64 = 1

type TGDialog struct {
	ID               int64
	TGAccountID      int64
	ChatID           int64
	Title            string
	Username         string
	Kind             string
	MessageCount     int
	DownloadedCount  int
	LastMessageID    int
	SyncedAt         string
}

func (d *DB) ReplaceTGDialogs(ctx context.Context, accountID int64, rows []TGDialog) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM tg_dialogs WHERE tg_account_id=?`, accountID); err != nil {
		return err
	}
	for _, r := range rows {
		downloaded, err := d.countDownloadedMessages(ctx, tx, r.ChatID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO tg_dialogs (tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			accountID, r.ChatID, r.Title, r.Username, r.Kind, r.MessageCount, downloaded, r.LastMessageID, r.SyncedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) countDownloadedMessages(ctx context.Context, q sqlQuerier, chatID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT message_id) FROM media_index WHERE chat_id=?`, chatID).Scan(&n)
	return n, err
}

type sqlQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (d *DB) RefreshDialogDownloadCounts(ctx context.Context, accountID int64) error {
	_, err := d.SQL.ExecContext(ctx, `
UPDATE tg_dialogs SET downloaded_count = (
  SELECT COUNT(DISTINCT message_id) FROM media_index WHERE media_index.chat_id = tg_dialogs.chat_id
) WHERE tg_account_id=?`, accountID)
	return err
}

func (d *DB) ListTGDialogs(ctx context.Context, accountID int64, limit, offset int) ([]TGDialog, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM tg_dialogs WHERE tg_account_id=?`, accountID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at
FROM tg_dialogs WHERE tg_account_id=? ORDER BY title COLLATE NOCASE LIMIT ? OFFSET ?`,
		accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []TGDialog
	for rows.Next() {
		var r TGDialog
		if err := rows.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
			&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (d *DB) GetTGDialog(ctx context.Context, accountID, chatID int64) (*TGDialog, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at
FROM tg_dialogs WHERE tg_account_id=? AND chat_id=?`, accountID, chatID)
	var r TGDialog
	err := row.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
		&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) DialogCount(ctx context.Context, accountID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM tg_dialogs WHERE tg_account_id=?`, accountID).Scan(&n)
	return n, err
}

func (d *DB) ReplaceSavedMessagesCache(ctx context.Context, accountID int64, messageIDs []int, sourceChatIDs []int64) error {
	if len(messageIDs) != len(sourceChatIDs) {
		return sql.ErrNoRows
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM saved_messages_cache WHERE tg_account_id=?`, accountID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i, mid := range messageIDs {
		_, err := tx.ExecContext(ctx, `
INSERT INTO saved_messages_cache (tg_account_id, message_id, source_chat_id, synced_at) VALUES (?, ?, ?, ?)`,
			accountID, mid, sourceChatIDs[i], now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) SavedMessageCount(ctx context.Context, accountID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM saved_messages_cache WHERE tg_account_id=?`, accountID).Scan(&n)
	return n, err
}

func (d *DB) SavedDownloadedCount(ctx context.Context, favoritesChatID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT message_id) FROM media_index WHERE chat_id=?`, favoritesChatID).Scan(&n)
	return n, err
}

func (d *DB) LastSyncedAt(ctx context.Context, accountID int64) (dialogsAt, savedAt string, err error) {
	err = d.SQL.QueryRowContext(ctx, `SELECT COALESCE(MAX(synced_at),'') FROM tg_dialogs WHERE tg_account_id=?`, accountID).Scan(&dialogsAt)
	if err != nil {
		return
	}
	err = d.SQL.QueryRowContext(ctx, `SELECT COALESCE(MAX(synced_at),'') FROM saved_messages_cache WHERE tg_account_id=?`, accountID).Scan(&savedAt)
	return
}

func (d *DB) MaxDownloadedMessageID(ctx context.Context, chatID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `SELECT MAX(message_id) FROM media_index WHERE chat_id=?`, chatID).Scan(&n)
	if err != nil || !n.Valid {
		return 0, err
	}
	return int(n.Int64), nil
}

// GetScanCursor 返回频道历史扫描水位：优先 chat_download_state，否则回退 media_index 最大 message id。
func (d *DB) GetScanCursor(ctx context.Context, accountID, chatID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `
SELECT last_downloaded_message_id FROM chat_download_state
WHERE tg_account_id=? AND chat_id=?`, accountID, chatID).Scan(&n)
	if err == nil && n.Valid && n.Int64 > 0 {
		return int(n.Int64), nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return d.MaxDownloadedMessageID(ctx, chatID)
}

func (d *DB) HasActiveChannelTask(ctx context.Context, chatID int64) (bool, int64, error) {
	var id int64
	err := d.SQL.QueryRowContext(ctx, `
SELECT id FROM tasks
WHERE status IN ('queued','running','paused')
  AND source IN ('chat_continue','chat_batch','chat_range')
  AND json_extract(options_json, '$.chatId') = ?
ORDER BY id DESC LIMIT 1`, chatID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, id, nil
}

func (d *DB) GetActiveChannelTask(ctx context.Context, chatID int64) (*Task, error) {
	ok, id, err := d.HasActiveChannelTask(ctx, chatID)
	if err != nil || !ok {
		return nil, err
	}
	return d.GetTask(ctx, id)
}

func (d *DB) ListChannelTasksForChat(ctx context.Context, chatID int64, limit int) ([]*Task, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, source, title, status, tg_account_id, options_json,
       total_bytes, done_bytes, total_files, done_files, speed_bps, error,
       created_at, started_at, finished_at
FROM tasks
WHERE source IN ('chat_continue','chat_batch','chat_range')
  AND json_extract(options_json, '$.chatId') = ?
ORDER BY id DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) CountFailedItemsForChatActive(ctx context.Context, chatID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `
SELECT COUNT(1) FROM task_items ti
JOIN tasks t ON t.id = ti.task_id
WHERE ti.status='failed'
  AND t.source IN ('chat_continue','chat_batch','chat_range')
  AND json_extract(t.options_json, '$.chatId') = ?`, chatID).Scan(&n)
	return n, err
}

func (d *DB) MaxTaskItemMessageID(ctx context.Context, taskID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `SELECT MAX(message_id) FROM task_items WHERE task_id=?`, taskID).Scan(&n)
	if err != nil || !n.Valid {
		return 0, err
	}
	return int(n.Int64), nil
}

func (d *DB) UpsertChatDownloadState(ctx context.Context, accountID, chatID int64, lastMessageID int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO chat_download_state (tg_account_id, chat_id, last_downloaded_message_id, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(tg_account_id, chat_id) DO UPDATE SET
  last_downloaded_message_id=CASE
    WHEN excluded.last_downloaded_message_id > chat_download_state.last_downloaded_message_id
    THEN excluded.last_downloaded_message_id
    ELSE chat_download_state.last_downloaded_message_id
  END,
  updated_at=excluded.updated_at`,
		accountID, chatID, lastMessageID, now)
	return err
}

// SetScanCursor 强制写入扫描水位（允许下调，供扫盘同步对齐磁盘）。
func (d *DB) SetScanCursor(ctx context.Context, accountID, chatID int64, lastMessageID int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if lastMessageID < 0 {
		lastMessageID = 0
	}
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO chat_download_state (tg_account_id, chat_id, last_downloaded_message_id, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(tg_account_id, chat_id) DO UPDATE SET
  last_downloaded_message_id=excluded.last_downloaded_message_id,
  updated_at=excluded.updated_at`,
		accountID, chatID, lastMessageID, now)
	return err
}

// RebuildScanCursorsFromMedia 按 media_index 各 chat 的最大 message_id 重建水位；无文件的频道水位归零。
// skipChatID 一般为收藏 chat_id，不参与频道水位。
func (d *DB) RebuildScanCursorsFromMedia(ctx context.Context, accountID, skipChatID int64) (int, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT chat_id, COALESCE(MAX(message_id), 0) FROM media_index GROUP BY chat_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	maxByChat := map[int64]int{}
	for rows.Next() {
		var chatID int64
		var maxID int
		if err := rows.Scan(&chatID, &maxID); err != nil {
			return 0, err
		}
		if skipChatID > 0 && chatID == skipChatID {
			continue
		}
		maxByChat[chatID] = maxID
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	stateRows, err := d.SQL.QueryContext(ctx, `
SELECT chat_id FROM chat_download_state WHERE tg_account_id=?`, accountID)
	if err != nil {
		return 0, err
	}
	defer stateRows.Close()
	for stateRows.Next() {
		var chatID int64
		if err := stateRows.Scan(&chatID); err != nil {
			return 0, err
		}
		if skipChatID > 0 && chatID == skipChatID {
			continue
		}
		if _, ok := maxByChat[chatID]; !ok {
			maxByChat[chatID] = 0
		}
	}
	if err := stateRows.Err(); err != nil {
		return 0, err
	}

	n := 0
	for chatID, maxID := range maxByChat {
		if err := d.SetScanCursor(ctx, accountID, chatID, maxID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// SyncScanCursorForChat 将单个频道水位对齐为 media_index 最大 message_id（无则 0）。
func (d *DB) SyncScanCursorForChat(ctx context.Context, accountID, chatID int64) error {
	maxID, err := d.MaxDownloadedMessageID(ctx, chatID)
	if err != nil {
		return err
	}
	return d.SetScanCursor(ctx, accountID, chatID, maxID)
}

func (d *DB) LibraryChatFilters(ctx context.Context, accountID int64, favoritesChatID int64) ([]struct {
	ChatID int64
	Title  string
}, error) {
	// 单连接 SQLite：禁止在未关闭的 rows 上再 Query（会死锁）
	rows, err := d.SQL.QueryContext(ctx, `
SELECT m.chat_id,
       COALESCE(NULLIF(d.title, ''), CAST(m.chat_id AS TEXT)) AS title
FROM (
  SELECT chat_id, COUNT(1) AS c FROM media_index GROUP BY chat_id
) m
LEFT JOIN tg_dialogs d ON d.chat_id = m.chat_id AND d.tg_account_id = ?
ORDER BY m.c DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]struct {
		ChatID int64
		Title  string
	}, 0)
	for rows.Next() {
		var chatID int64
		var title string
		if err := rows.Scan(&chatID, &title); err != nil {
			return nil, err
		}
		if chatID == favoritesChatID {
			title = "我的收藏"
		}
		out = append(out, struct {
			ChatID int64
			Title  string
		}{ChatID: chatID, Title: title})
	}
	return out, rows.Err()
}
