package tg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
	"golang.org/x/sync/singleflight"

	"tdload/internal/library"
)

var videoThumbFlight singleflight.Group

// EnsureVideoThumb 确保视频封面缓存存在：已有则直接返回路径；否则向 Telegram 拉取 document thumb。
func (m *Manager) EnsureVideoThumb(ctx context.Context, chatID int64, messageID int, size int64, mime, fileName string) (string, error) {
	if m == nil || m.Cfg == nil {
		return "", fmt.Errorf("Telegram 未就绪")
	}
	downloadDir := m.Cfg.DownloadDir
	if downloadDir == "" || chatID == 0 || messageID <= 0 || size <= 0 {
		return "", fmt.Errorf("无效封面参数")
	}
	if library.DetectMediaKind(mime, fileName) != "video" {
		return "", fmt.Errorf("非视频")
	}
	cache := library.ThumbCachePath(downloadDir, chatID, messageID, size)
	if library.HasVideoThumb(downloadDir, chatID, messageID, size) {
		return cache, nil
	}

	key := library.ThumbKey(chatID, messageID, size)
	v, err, _ := videoThumbFlight.Do(key, func() (any, error) {
		if library.HasVideoThumb(downloadDir, chatID, messageID, size) {
			return cache, nil
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := m.fetchVideoThumb(fetchCtx, downloadDir, chatID, messageID, size, mime, fileName); err != nil {
			return "", err
		}
		if !library.HasVideoThumb(downloadDir, chatID, messageID, size) {
			return "", fmt.Errorf("无法获取视频封面")
		}
		return cache, nil
	})
	if err != nil {
		return "", err
	}
	path, _ := v.(string)
	if path == "" {
		return "", fmt.Errorf("无法获取视频封面")
	}
	return path, nil
}

func (m *Manager) fetchVideoThumb(ctx context.Context, downloadDir string, chatID int64, messageID int, size int64, mime, fileName string) error {
	username := ""
	if m.DB != nil {
		if dlg, err := m.DB.FindTGDialogMatch(ctx, defaultAccountID, chatID, ""); err == nil && dlg != nil {
			username = dlg.Username
		}
	}
	return m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		return m.withAPI(ctx, client, func(ctx context.Context, api *tg.Client) error {
			input, err := m.inputPeerForThumb(ctx, client, api, chatID, username)
			if err != nil {
				return err
			}
			msg, err := tutil.GetSingleMessage(ctx, api, input, messageID)
			if err != nil {
				return fmt.Errorf("获取消息: %w", err)
			}
			dl := downloader.NewDownloader()
			return saveVideoThumb(ctx, api, dl, downloadDir, chatID, messageID, size, mime, fileName, msg)
		})
	})
}

func (m *Manager) inputPeerForThumb(ctx context.Context, client *telegram.Client, api *tg.Client, chatID int64, username string) (tg.InputPeerClass, error) {
	self, err := client.Self(ctx)
	if err == nil && self != nil && FavoritesChatID(self.ID) == chatID {
		return &tg.InputPeerSelf{}, nil
	}
	peer, err := resolveChatPeer(ctx, api, chatID, username, true)
	if err != nil {
		return nil, err
	}
	return peer.InputPeer(), nil
}

// saveVideoThumb 下载 Telegram document 自带封面并写入资源库缩略图缓存。
// 非视频、无 thumbs、或已有缓存时直接跳过；失败返回 error。
func saveVideoThumb(ctx context.Context, api *tg.Client, dl *downloader.Downloader, downloadDir string, chatID int64, messageID int, size int64, mime, fileName string, msg *tg.Message) error {
	if downloadDir == "" || msg == nil || size <= 0 {
		return fmt.Errorf("无效封面参数")
	}
	if library.DetectMediaKind(mime, fileName) != "video" {
		return nil
	}
	if library.HasVideoThumb(downloadDir, chatID, messageID, size) {
		return nil
	}
	loc, ok := documentThumbLocation(msg)
	if !ok {
		return fmt.Errorf("消息无封面")
	}
	tmpDir := filepath.Join(downloadDir, ".tdload-thumbs")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(tmpDir, fmt.Sprintf(".tmp_%d_%d_%d.jpg", chatID, messageID, size))
	defer func() { _ = os.Remove(tmp) }()

	thumbCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := dl.Download(api, loc).WithThreads(1).ToPath(thumbCtx, tmp); err != nil {
		slog.Debug("video thumb download", "chat", chatID, "msg", messageID, "err", err)
		return fmt.Errorf("下载封面: %w", err)
	}
	if err := library.InstallVideoThumb(downloadDir, chatID, messageID, size, tmp); err != nil {
		slog.Debug("video thumb install", "chat", chatID, "msg", messageID, "err", err)
		return err
	}
	slog.Debug("video thumb saved", "chat", chatID, "msg", messageID, "size", size)
	return nil
}

// documentThumbLocation 选取 document 最大静态缩略图的下载位置。
func documentThumbLocation(msg *tg.Message) (tg.InputFileLocationClass, bool) {
	m, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, false
	}
	doc, ok := m.Document.AsNotEmpty()
	if !ok {
		return nil, false
	}
	thumbs, exists := doc.GetThumbs()
	if !exists || len(thumbs) == 0 {
		return nil, false
	}
	var (
		thumbType string
		thumbSize int
		found     bool
	)
	for _, t := range thumbs {
		switch p := t.(type) {
		case *tg.PhotoSize:
			if !found || p.Size > thumbSize {
				thumbType, thumbSize, found = p.Type, p.Size, true
			}
		case *tg.PhotoSizeProgressive:
			sz := 0
			if n := len(p.Sizes); n > 0 {
				sz = p.Sizes[n-1]
			}
			if !found || sz > thumbSize {
				thumbType, thumbSize, found = p.Type, sz, true
			}
		}
	}
	if !found || thumbType == "" {
		return nil, false
	}
	return &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     thumbType,
	}, true
}
