package db

import (
	"context"
	"strings"
	"time"
)

// UpsertChatLabel 缓存已解析的频道/群显示名（不依赖 tg_dialogs，同步对话不会清掉）。
func (d *DB) UpsertChatLabel(ctx context.Context, chatID int64, title, username string) error {
	if chatID == 0 {
		return nil
	}
	title = strings.TrimSpace(title)
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if title == "" && username == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.SQL.ExecContext(ctx, `
INSERT INTO chat_labels (chat_id, title, username, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(chat_id) DO UPDATE SET
  title=CASE WHEN excluded.title != '' THEN excluded.title ELSE chat_labels.title END,
  username=CASE WHEN excluded.username != '' THEN excluded.username ELSE chat_labels.username END,
  updated_at=excluded.updated_at`,
		chatID, title, username, now)
	return err
}

func (d *DB) GetChatLabel(ctx context.Context, chatID int64) (title, username string, ok bool) {
	err := d.SQL.QueryRowContext(ctx, `
SELECT title, username FROM chat_labels WHERE chat_id=?`, chatID).Scan(&title, &username)
	if err != nil {
		return "", "", false
	}
	return title, username, true
}
