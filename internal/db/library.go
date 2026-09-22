package db

import (
	"context"
	"database/sql"
	"strings"
)

type MediaRow struct {
	ID           int64
	ChatID       int64
	MessageID    int
	FileName     string
	Size         int64
	LocalPath    string
	Mime         string
	DownloadedAt string
}

func (d *DB) ListAllMedia(ctx context.Context) ([]MediaRow, error) {
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, chat_id, message_id, file_name, size, local_path, mime, downloaded_at
FROM media_index ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRow
	for rows.Next() {
		var r MediaRow
		if err := rows.Scan(&r.ID, &r.ChatID, &r.MessageID, &r.FileName, &r.Size, &r.LocalPath, &r.Mime, &r.DownloadedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetMedia(ctx context.Context, id int64) (*MediaRow, error) {
	row := d.SQL.QueryRowContext(ctx, `
SELECT id, chat_id, message_id, file_name, size, local_path, mime, downloaded_at
FROM media_index WHERE id=?`, id)
	var r MediaRow
	err := row.Scan(&r.ID, &r.ChatID, &r.MessageID, &r.FileName, &r.Size, &r.LocalPath, &r.Mime, &r.DownloadedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) DeleteMedia(ctx context.Context, id int64) error {
	_, err := d.SQL.ExecContext(ctx, `DELETE FROM media_index WHERE id=?`, id)
	return err
}

func (d *DB) ListMedia(ctx context.Context, chatID *int64, mediaType string, q string, limit, offset int) ([]MediaRow, int, error) {
	if limit <= 0 {
		limit = 50
	}
	where := []string{"1=1"}
	args := []any{}
	if chatID != nil {
		where = append(where, "chat_id=?")
		args = append(args, *chatID)
	}
	switch mediaType {
	case "image":
		where = append(where, "(mime LIKE 'image/%' OR lower(file_name) GLOB '*.jpg' OR lower(file_name) GLOB '*.jpeg' OR lower(file_name) GLOB '*.png' OR lower(file_name) GLOB '*.webp' OR lower(file_name) GLOB '*.gif')")
	case "video":
		where = append(where, "(mime LIKE 'video/%' OR lower(file_name) GLOB '*.mp4' OR lower(file_name) GLOB '*.mkv' OR lower(file_name) GLOB '*.mov' OR lower(file_name) GLOB '*.webm')")
	}
	if strings.TrimSpace(q) != "" {
		where = append(where, "file_name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(q)+"%")
	}
	wclause := strings.Join(where, " AND ")
	var total int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(1) FROM media_index WHERE `+wclause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := d.SQL.QueryContext(ctx, `
SELECT id, chat_id, message_id, file_name, size, local_path, mime, downloaded_at
FROM media_index WHERE `+wclause+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []MediaRow
	for rows.Next() {
		var r MediaRow
		if err := rows.Scan(&r.ID, &r.ChatID, &r.MessageID, &r.FileName, &r.Size, &r.LocalPath, &r.Mime, &r.DownloadedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}
