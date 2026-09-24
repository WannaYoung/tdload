package db

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"
)

const DefaultTGAccountID int64 = 1

type TGDialog struct {
	ID              int64
	TGAccountID     int64
	ChatID          int64
	Title           string
	Username        string
	Kind            string
	MessageCount    int
	DownloadedCount int
	LastMessageID   int
	SyncedAt        string
	IsCustom        bool
}

func (d *DB) ReplaceTGDialogs(ctx context.Context, accountID int64, rows []TGDialog) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 只清非自定义对话；自定义频道在用户未加入前保留。
	if _, err := tx.ExecContext(ctx, `DELETE FROM tg_dialogs WHERE tg_account_id=? AND is_custom=0`, accountID); err != nil {
		return err
	}
	for _, r := range rows {
		downloaded, err := d.countDownloadedMessages(ctx, tx, r.ChatID)
		if err != nil {
			return err
		}
		// 若同 chat_id 已是自定义行（用户后来加入），UPSERT 合并并清 is_custom。
		_, err = tx.ExecContext(ctx, `
INSERT INTO tg_dialogs (tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
ON CONFLICT(tg_account_id, chat_id) DO UPDATE SET
  title=excluded.title,
  username=excluded.username,
  kind=excluded.kind,
  message_count=excluded.message_count,
  downloaded_count=excluded.downloaded_count,
  last_message_id=excluded.last_message_id,
  synced_at=excluded.synced_at,
  is_custom=0`,
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
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom
FROM tg_dialogs WHERE tg_account_id=? ORDER BY is_custom ASC, title COLLATE NOCASE LIMIT ? OFFSET ?`,
		accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []TGDialog
	for rows.Next() {
		var r TGDialog
		var isCustom int
		if err := rows.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
			&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt, &isCustom); err != nil {
			return nil, 0, err
		}
		r.IsCustom = isCustom != 0
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (d *DB) GetTGDialog(ctx context.Context, accountID, chatID int64) (*TGDialog, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom
FROM tg_dialogs WHERE tg_account_id=? AND chat_id=?`, accountID, chatID)
	var r TGDialog
	var isCustom int
	err := row.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
		&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt, &isCustom)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsCustom = isCustom != 0
	return &r, nil
}

// dialogChatIDVariants 生成同一频道可能出现的 chat_id 形式（带/不带 -100 前缀）。
func dialogChatIDVariants(chatID int64) []int64 {
	if chatID == 0 {
		return nil
	}
	seen := map[int64]struct{}{chatID: {}}
	out := []int64{chatID}
	plain := chatID
	if chatID < 0 {
		s := strconv.FormatInt(chatID, 10)
		if strings.HasPrefix(s, "-100") {
			if n, err := strconv.ParseInt(strings.TrimPrefix(s, "-100"), 10, 64); err == nil {
				plain = n
			}
		}
	}
	marked := chatID
	if chatID > 0 {
		marked = -(1_000_000_000_000 + chatID)
	} else if plain > 0 && plain != chatID {
		marked = -(1_000_000_000_000 + plain)
	}
	for _, id := range []int64{plain, marked} {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// FindTGDialogMatch 按 chat_id（兼容 -100 形式）或 username 查找已有对话。
func (d *DB) FindTGDialogMatch(ctx context.Context, accountID, chatID int64, username string) (*TGDialog, error) {
	for _, id := range dialogChatIDVariants(chatID) {
		row, err := d.GetTGDialog(ctx, accountID, id)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return row, nil
		}
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, nil
	}
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom
FROM tg_dialogs WHERE tg_account_id=? AND lower(username)=lower(?) LIMIT 1`, accountID, username)
	var r TGDialog
	var isCustom int
	err := row.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
		&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt, &isCustom)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsCustom = isCustom != 0
	return &r, nil
}

// ListCustomTGDialogs 列出账号下全部自定义频道。
func (d *DB) ListCustomTGDialogs(ctx context.Context, accountID int64) ([]TGDialog, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom
FROM tg_dialogs WHERE tg_account_id=? AND is_custom=1 ORDER BY title COLLATE NOCASE`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TGDialog
	for rows.Next() {
		var r TGDialog
		var isCustom int
		if err := rows.Scan(&r.ID, &r.TGAccountID, &r.ChatID, &r.Title, &r.Username, &r.Kind,
			&r.MessageCount, &r.DownloadedCount, &r.LastMessageID, &r.SyncedAt, &isCustom); err != nil {
			return nil, err
		}
		r.IsCustom = isCustom != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertCustomDialog 写入自定义频道（is_custom=1）；冲突时更新元数据并保持自定义标记。
func (d *DB) UpsertCustomDialog(ctx context.Context, accountID int64, dialog TGDialog) error {
	now := dialog.SyncedAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	kind := dialog.Kind
	if kind == "" {
		kind = "custom"
	}
	downloaded, err := d.countDownloadedMessages(ctx, d.SQL, dialog.ChatID)
	if err != nil {
		return err
	}
	_, err = d.SQL.ExecContext(ctx, `
INSERT INTO tg_dialogs (tg_account_id, chat_id, title, username, kind, message_count, downloaded_count, last_message_id, synced_at, is_custom)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(tg_account_id, chat_id) DO UPDATE SET
  title=excluded.title,
  username=excluded.username,
  kind=excluded.kind,
  message_count=excluded.message_count,
  downloaded_count=excluded.downloaded_count,
  last_message_id=excluded.last_message_id,
  synced_at=excluded.synced_at,
  is_custom=1`,
		accountID, dialog.ChatID, dialog.Title, dialog.Username, kind,
		dialog.MessageCount, downloaded, dialog.LastMessageID, now)
	return err
}

// DeleteCustomDialog 仅删除 is_custom=1 的行。
func (d *DB) DeleteCustomDialog(ctx context.Context, accountID, chatID int64) error {
	res, err := d.SQL.ExecContext(ctx, `
DELETE FROM tg_dialogs WHERE tg_account_id=? AND chat_id=? AND is_custom=1`, accountID, chatID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateDialogMeta 更新对话标题/用户名/最新消息 ID。
func (d *DB) UpdateDialogMeta(ctx context.Context, accountID, chatID int64, title, username string, lastMessageID int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	res, err := d.SQL.ExecContext(ctx, `
UPDATE tg_dialogs SET title=?, username=?, last_message_id=?, synced_at=?
WHERE tg_account_id=? AND chat_id=?`, title, username, lastMessageID, now, accountID, chatID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
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

// GetScanCursor 返回频道历史扫描水位。
// 若已有 chat_download_state 记录则以其为准（含 0，表示从未推进）；
// 仅在尚无状态记录时，回退为 media_index 最大 message id。
func (d *DB) GetScanCursor(ctx context.Context, accountID, chatID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `
SELECT last_downloaded_message_id FROM chat_download_state
WHERE tg_account_id=? AND chat_id=?`, accountID, chatID).Scan(&n)
	if err == nil && n.Valid {
		return int(n.Int64), nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return d.MaxDownloadedMessageID(ctx, chatID)
}

func (d *DB) HasActiveChannelTask(ctx context.Context, chatID int64) (bool, int64, error) {
	return d.hasActiveChannelTaskBySources(ctx, chatID, "chat_continue", "chat_range")
}

// HasActiveChatBatchTask 任务页频道下载进行中任务。
func (d *DB) HasActiveChatBatchTask(ctx context.Context, chatID int64) (bool, int64, error) {
	return d.hasActiveChannelTaskBySources(ctx, chatID, "chat_batch")
}

func (d *DB) hasActiveChannelTaskBySources(ctx context.Context, chatID int64, sources ...string) (bool, int64, error) {
	if len(sources) == 0 {
		return false, 0, nil
	}
	placeholders := make([]string, len(sources))
	args := make([]any, 0, len(sources)+1)
	for i, s := range sources {
		placeholders[i] = "?"
		args = append(args, s)
	}
	args = append(args, chatID)
	var id int64
	err := d.SQL.QueryRowContext(ctx, `
SELECT id FROM tasks
WHERE status IN ('queued','running','paused')
  AND source IN (`+strings.Join(placeholders, ",")+`)
  AND json_extract(options_json, '$.chatId') = ?
ORDER BY id DESC LIMIT 1`, args...).Scan(&id)
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
WHERE source IN ('chat_continue','chat_range')
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

// ClearCompletedChannelTasksForChat 清除该频道页续下的已结束批次（不影响任务页 chat_batch）。
func (d *DB) ClearCompletedChannelTasksForChat(ctx context.Context, chatID int64) (int64, error) {
	res, err := d.SQL.ExecContext(ctx, `
DELETE FROM tasks
WHERE status IN ('done','cancelled','failed')
  AND source IN ('chat_continue','chat_range')
  AND json_extract(options_json, '$.chatId') = ?`, chatID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) CountFailedItemsForChatActive(ctx context.Context, chatID int64) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `
SELECT COUNT(1) FROM task_items ti
JOIN tasks t ON t.id = ti.task_id
WHERE ti.status='failed'
  AND t.source IN ('chat_continue','chat_range')
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

func (d *DB) MinTaskItemMessageID(ctx context.Context, taskID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `SELECT MIN(message_id) FROM task_items WHERE task_id=?`, taskID).Scan(&n)
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

// SetScanCursor 强制写入扫描水位（允许上调或下调）。
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

// GetChannelBatchSize 返回该频道已保存的每批条数；未设置时返回 0。
func (d *DB) GetChannelBatchSize(ctx context.Context, accountID, chatID int64) (int, error) {
	var n sql.NullInt64
	err := d.SQL.QueryRowContext(ctx, `
SELECT batch_size FROM chat_download_state WHERE tg_account_id=? AND chat_id=?`, accountID, chatID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !n.Valid || n.Int64 <= 0 {
		return 0, nil
	}
	return int(n.Int64), nil
}

// SetChannelBatchSize 保存频道每批条数（成功下载后记忆）。
func (d *DB) SetChannelBatchSize(ctx context.Context, accountID, chatID int64, batchSize int) error {
	if batchSize <= 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO chat_download_state (tg_account_id, chat_id, last_downloaded_message_id, batch_size, updated_at)
VALUES (?, ?, 0, ?, ?)
ON CONFLICT(tg_account_id, chat_id) DO UPDATE SET
  batch_size=excluded.batch_size,
  updated_at=excluded.updated_at`,
		accountID, chatID, batchSize, now)
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
       COALESCE(
         NULLIF(d.title, ''),
         NULLIF(l.title, ''),
         CASE WHEN l.username != '' THEN '@' || l.username END,
         CAST(m.chat_id AS TEXT)
       ) AS title
FROM (
  SELECT chat_id, COUNT(1) AS c FROM media_index GROUP BY chat_id
) m
LEFT JOIN tg_dialogs d ON d.chat_id = m.chat_id AND d.tg_account_id = ?
LEFT JOIN chat_labels l ON l.chat_id = m.chat_id
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
