package tg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
	"golang.org/x/sync/errgroup"

	"tdload/internal/db"
)

// FavoritesFolderName 是收藏落盘子目录名。
const FavoritesFolderName = "我的收藏"

type DialogSyncRow struct {
	ChatID        int64
	Title         string
	Username      string
	Kind          string
	MessageCount  int
	LastMessageID int
}

type SyncSummary struct {
	DialogCount int `json:"dialogCount"`
	SavedCount  int `json:"savedCount"`
	DialogsAt   string `json:"dialogsSyncedAt,omitempty"`
	SavedAt     string `json:"savedSyncedAt,omitempty"`
}

func (m *Manager) SyncDialogs(ctx context.Context) (int, error) {
	var rows []DialogSyncRow
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		pm := peers.Options{}.Build(api)
		elems, err := dialogs.NewQueryBuilder(api).GetDialogs().BatchSize(100).Collect(ctx)
		if err != nil {
			return err
		}
		for _, el := range elems {
			if el.Deleted() {
				continue
			}
			p, err := pm.FromInputPeer(ctx, el.Peer)
			if err != nil {
				continue
			}
			kind, include := peerKind(p)
			if !include {
				continue
			}
			title := p.VisibleName()
			username, _ := p.Username()
			topID := 0
			if d, ok := el.Dialog.(*tg.Dialog); ok {
				topID = d.TopMessage
			}
			msgCount := -1
			rows = append(rows, DialogSyncRow{
				ChatID:        p.ID(),
				Title:         title,
				Username:      username,
				Kind:          kind,
				MessageCount:  msgCount,
				LastMessageID: topID,
			})
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	syncedAt := time.Now().UTC().Format(time.RFC3339)
	dbRows := make([]db.TGDialog, 0, len(rows))
	for _, r := range rows {
		dbRows = append(dbRows, db.TGDialog{
			TGAccountID:   defaultAccountID,
			ChatID:        r.ChatID,
			Title:         r.Title,
			Username:      r.Username,
			Kind:          r.Kind,
			MessageCount:  r.MessageCount,
			LastMessageID: r.LastMessageID,
			SyncedAt:      syncedAt,
		})
	}
	if err := m.DB.ReplaceTGDialogs(ctx, defaultAccountID, dbRows); err != nil {
		return 0, err
	}
	// 同步后刷新自定义频道的标题 / 最新消息 ID（不影响已加入对话）。
	if _, err := m.SyncCustomDialogs(ctx); err != nil {
		slog.Warn("sync custom dialogs", "err", err)
	}
	return len(dbRows), nil
}

// SyncCustomDialogs 刷新 is_custom=1 的频道元数据与最新消息 ID。
func (m *Manager) SyncCustomDialogs(ctx context.Context) (int, error) {
	rows, err := m.DB.ListCustomTGDialogs(ctx, defaultAccountID)
	if err != nil {
		return 0, err
	}
	ok := 0
	for _, r := range rows {
		info, lastID, err := m.FetchChatSnapshot(ctx, r.ChatID, r.Username)
		if err != nil {
			slog.Warn("sync custom dialog", "chatId", r.ChatID, "username", r.Username, "err", err)
			continue
		}
		title := info.Title
		if title == "" {
			title = r.Title
		}
		username := info.Username
		if username == "" {
			username = r.Username
		}
		if lastID <= 0 {
			lastID = r.LastMessageID
		}
		if err := m.DB.UpdateDialogMeta(ctx, defaultAccountID, r.ChatID, title, username, lastID); err != nil {
			slog.Warn("update custom dialog meta", "chatId", r.ChatID, "err", err)
			continue
		}
		_ = m.DB.UpsertChatLabel(ctx, r.ChatID, title, username)
		ok++
	}
	return ok, nil
}

func peerKind(p peers.Peer) (kind string, include bool) {
	switch v := p.(type) {
	case peers.Channel:
		raw := v.Raw()
		if raw.Broadcast {
			return "channel", true
		}
		if raw.Megagroup {
			return "supergroup", true
		}
		return "channel", true
	case peers.Chat:
		return "group", true
	default:
		return "user", false
	}
}

// SyncSaved 拉取 Saved Messages 全量 id 写入缓存。
// onProgress 可选：每累计若干条回调一次（最后一条总会回调），供收藏同步任务刷新「收藏数」。
func (m *Manager) SyncSaved(ctx context.Context, selfUserID int64, onProgress func(count int)) (int, error) {
	var ids []int
	var srcChat []int64
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		peer := &tg.InputPeerSelf{}
		return messages.NewQueryBuilder(api).GetHistory(peer).BatchSize(100).ForEach(ctx,
			func(ctx context.Context, elem messages.Elem) error {
				msg, ok := elem.Msg.(*tg.Message)
				if !ok {
					return nil
				}
				ids = append(ids, msg.ID)
				src := int64(0)
				if msg.PeerID != nil {
					switch p := msg.PeerID.(type) {
					case *tg.PeerChannel:
						src = p.ChannelID
					case *tg.PeerChat:
						src = p.ChatID
					case *tg.PeerUser:
						src = p.UserID
					}
				}
				srcChat = append(srcChat, src)
				if onProgress != nil && len(ids)%25 == 0 {
					onProgress(len(ids))
				}
				return nil
			})
	})
	if err != nil {
		return 0, err
	}
	if err := m.DB.ReplaceSavedMessagesCache(ctx, defaultAccountID, ids, srcChat); err != nil {
		return 0, err
	}
	if onProgress != nil {
		onProgress(len(ids))
	}
	_ = selfUserID
	return len(ids), nil
}

func (m *Manager) TGSummary(ctx context.Context) (*SyncSummary, error) {
	dCount, _ := m.DB.DialogCount(ctx, defaultAccountID)
	sCount, _ := m.DB.SavedMessageCount(ctx, defaultAccountID)
	dAt, sAt, _ := m.DB.LastSyncedAt(ctx, defaultAccountID)
	return &SyncSummary{
		DialogCount: dCount,
		SavedCount:  sCount,
		DialogsAt:   dAt,
		SavedAt:     sAt,
	}, nil
}

func (m *Manager) SelfUserID(ctx context.Context) (int64, error) {
	var id int64
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		id = self.ID
		return nil
	})
	return id, err
}

func FavoritesChatID(selfUserID int64) int64 {
	return selfUserID
}

// DownloadSaved 下载 Saved Messages 中指定 message id 列表（落盘 OutSubdir）。
func (m *Manager) DownloadSaved(ctx context.Context, opt DownloadOptions, favoritesChatID int64, messageIDs []int) error {
	if len(messageIDs) == 0 {
		return fmt.Errorf("没有收藏消息")
	}
	opt.OutSubdir = FavoritesFolderName
	if opt.OutDir == "" {
		opt.OutDir = m.Cfg.DownloadDir
	}
	_, conc := resolveDownloadLimits(&opt, m.Cfg.Threads, m.Cfg.Concurrency)
	return m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		return m.withAPI(ctx, client, func(ctx context.Context, api *tg.Client) error {
			peer := &tg.InputPeerSelf{}
			total := len(messageIDs)
			var done atomic.Int32
			dl := downloader.NewDownloader()
			cb := &cbGuard{opt: &opt}
			outRoot := filepath.Join(opt.OutDir, FavoritesFolderName)
			if err := os.MkdirAll(outRoot, 0o755); err != nil {
				return err
			}

			type fileJob struct {
				msgID int
				name  string
				path  string
				size  int64
				mime  string
				loc   tg.InputFileLocationClass
			}
			var toFetch []fileJob

			for _, mid := range messageIDs {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				msg, err := tutil.GetSingleMessage(ctx, api, peer, mid)
				if err != nil {
					cb.item(favoritesChatID, mid, "failed", "", "", err.Error())
					continue
				}
				elem := messages.Elem{Msg: msg}
				file, ok := elem.File()
				if !ok {
					cb.item(favoritesChatID, mid, "skipped", "", "", "无媒体")
					bumpDone(&done, total, fmt.Sprintf("msg %d 无媒体", mid), cb)
					continue
				}
				if !MatchContentType(opt.ContentType, file.MIMEType, file.Name) {
					cb.item(favoritesChatID, mid, "skipped", file.Name, "", "类型不符")
					bumpDone(&done, total, file.Name+" (类型不符)", cb)
					continue
				}
				size := mediaSize(msg)
				if opt.SkipSame && opt.Exists != nil && size > 0 {
					if exists, path, err := opt.Exists(favoritesChatID, mid, size); err == nil && exists {
						cb.item(favoritesChatID, mid, "skipped", file.Name, path, "")
						_ = cb.file(favoritesChatID, mid, file.Name, size, path, file.MIMEType)
						bumpDone(&done, total, file.Name+" (已存在)", cb)
						continue
					}
				}
				name := file.Name
				if opt.RewriteExt {
					name = rewriteExtByMIME(name, file.MIMEType)
				}
				if name == "" {
					name = fmt.Sprintf("%d_%d", favoritesChatID, mid)
				}
				path, err := allocUniquePath(filepath.Join(outRoot, safeFileName(name)))
				if err != nil {
					cb.item(favoritesChatID, mid, "failed", name, "", err.Error())
					bumpDone(&done, total, name+" 失败", cb)
					continue
				}
				toFetch = append(toFetch, fileJob{
					msgID: mid, name: name, path: path, size: size, mime: file.MIMEType, loc: file.Location,
				})
			}

			eg, egCtx := errgroup.WithContext(ctx)
			eg.SetLimit(conc)
			for _, fj := range toFetch {
				fj := fj
				eg.Go(func() error {
					select {
					case <-egCtx.Done():
						_ = os.Remove(fj.path)
						return egCtx.Err()
					default:
					}
					cb.item(favoritesChatID, fj.msgID, "downloading", fj.name, "", "")
					threads := tutil.BestThreads(fj.size, opt.Threads)
					if threads < 1 {
						threads = 1
					}
					slog.Info("downloading", "file", fj.name, "path", fj.path, "size", fj.size, "threads", threads, "concurrency", conc)
					fileCtx, cancel := context.WithTimeout(egCtx, 30*time.Minute)
					_, err := dl.Download(api, fj.loc).WithThreads(threads).ToPath(fileCtx, fj.path)
					cancel()
					if err != nil {
						_ = os.Remove(fj.path)
						cb.item(favoritesChatID, fj.msgID, "failed", fj.name, "", err.Error())
						bumpDone(&done, total, fj.name+" 失败", cb)
						return nil
					}
					size := fj.size
					if size == 0 {
						if st, err := os.Stat(fj.path); err == nil {
							size = st.Size()
						}
					}
					_ = cb.file(favoritesChatID, fj.msgID, fj.name, size, fj.path, fj.mime)
					cb.item(favoritesChatID, fj.msgID, "done", fj.name, fj.path, "")
					bumpDone(&done, total, fj.name, cb)
					return nil
				})
			}
			return eg.Wait()
		})
	})
}

func (m *Manager) SyncAll(ctx context.Context) (*SyncSummary, error) {
	selfID, err := m.SelfUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取账号信息: %w", err)
	}
	if _, err := m.SyncDialogs(ctx); err != nil {
		return nil, err
	}
	if _, err := m.SyncSaved(ctx, selfID, nil); err != nil {
		return nil, err
	}
	return m.TGSummary(ctx)
}
