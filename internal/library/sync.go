package library

import (
	"context"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tdload/internal/db"
	"tdload/internal/tg"
)

type SyncResult struct {
	Kept            int `json:"kept"`
	Pruned          int `json:"pruned"`
	Imported        int `json:"imported"`
	CursorsUpdated  int `json:"cursorsUpdated"`
}

var fileNamePattern = regexp.MustCompile(`^(-?\d+)_(\d+)_(.+)$`)

// SyncDisk 清理失效索引并扫盘补入 media_index。
func SyncDisk(ctx context.Context, database *db.DB, downloadDir string, favoritesChatID int64) (*SyncResult, error) {
	root, err := filepath.Abs(downloadDir)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	if st, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(root, 0o755); mkErr != nil {
				return nil, mkErr
			}
		} else {
			return nil, err
		}
	} else if !st.IsDir() {
		return nil, fmt.Errorf("下载目录不是文件夹: %s", root)
	}

	rows, err := database.ListAllMedia(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]struct{}{}
	kept, pruned := 0, 0
	for _, m := range rows {
		abs, ok := resolveUnderRoot(root, m.LocalPath)
		if !ok {
			_ = database.DeleteMedia(ctx, m.ID)
			pruned++
			continue
		}
		if st, err := os.Stat(abs); err != nil || st.IsDir() {
			_ = database.DeleteMedia(ctx, m.ID)
			pruned++
			continue
		}
		known[abs] = struct{}{}
		kept++
	}

	imported := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(strings.ToLower(name), ".part") {
			return nil
		}
		abs := filepath.Clean(path)
		if _, ok := known[abs]; ok {
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil
		}
		chatID, messageID, fileName, ok := parseIndexedPath(rel, favoritesChatID)
		if !ok {
			return nil
		}
		st, err := os.Stat(abs)
		if err != nil || st.Size() <= 0 {
			return nil
		}
		mimeType := mime.TypeByExtension(filepath.Ext(fileName))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if err := database.UpsertMedia(ctx, chatID, messageID, fileName, st.Size(), abs, mimeType); err != nil {
			return nil
		}
		known[abs] = struct{}{}
		imported++
		return nil
	})
	if err != nil {
		return nil, err
	}
	cursors, err := database.RebuildScanCursorsFromMedia(ctx, db.DefaultTGAccountID, favoritesChatID)
	if err != nil {
		return nil, err
	}
	return &SyncResult{Kept: kept, Pruned: pruned, Imported: imported, CursorsUpdated: cursors}, nil
}

func parseIndexedPath(rel string, favoritesChatID int64) (chatID int64, messageID int, fileName string, ok bool) {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return 0, 0, "", false
	}
	folder := parts[0]
	base := parts[len(parts)-1]
	m := fileNamePattern.FindStringSubmatch(base)
	if m == nil {
		return 0, 0, "", false
	}
	msgID, err := strconv.Atoi(m[2])
	if err != nil || msgID <= 0 {
		return 0, 0, "", false
	}
	fileName = m[3]
	if folder == tg.FavoritesFolderName {
		if favoritesChatID <= 0 {
			return 0, 0, "", false
		}
		return favoritesChatID, msgID, fileName, true
	}
	// 目录名：{chatId}_{title}
	us := strings.IndexByte(folder, '_')
	idPart := folder
	if us > 0 {
		idPart = folder[:us]
	}
	cid, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil || cid == 0 {
		return 0, 0, "", false
	}
	return cid, msgID, fileName, true
}

func resolveUnderRoot(root, localPath string) (string, bool) {
	var abs string
	if filepath.IsAbs(localPath) {
		abs = filepath.Clean(localPath)
	} else {
		abs = filepath.Clean(filepath.Join(root, localPath))
		base := filepath.Base(root)
		rel := filepath.Clean(localPath)
		if strings.HasPrefix(rel, base+string(os.PathSeparator)) {
			abs = filepath.Clean(filepath.Join(root, strings.TrimPrefix(rel, base+string(os.PathSeparator))))
		}
	}
	sep := string(os.PathSeparator)
	if abs != root && !strings.HasPrefix(abs, root+sep) {
		return "", false
	}
	return abs, true
}

// NowRFC3339 便于测试。
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
