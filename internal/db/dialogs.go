package db

import (
	"context"
	"database/sql"
	"fmt"
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

func (d *DB) LibraryChatFilters(ctx context.Context, accountID int64, favoritesChatID int64) ([]struct {
	ChatID int64
	Title  string
}, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT chat_id, COUNT(1) AS c FROM media_index GROUP BY chat_id ORDER BY c DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		ChatID int64
		Title  string
	}
	var raw []row
	for rows.Next() {
		var chatID int64
		var c int
		if err := rows.Scan(&chatID, &c); err != nil {
			return nil, err
		}
		title := ""
		if chatID == favoritesChatID {
			title = "我的收藏"
		} else {
			dlg, _ := d.GetTGDialog(ctx, accountID, chatID)
			if dlg != nil {
				title = dlg.Title
			}
			if title == "" {
				title = formatChatID(chatID)
			}
		}
		raw = append(raw, row{ChatID: chatID, Title: title})
	}
	out := make([]struct {
		ChatID int64
		Title  string
	}, len(raw))
	for i, r := range raw {
		out[i].ChatID = r.ChatID
		out[i].Title = r.Title
	}
	return out, rows.Err()
}

func formatChatID(id int64) string {
	return fmt.Sprintf("%d", id)
}
