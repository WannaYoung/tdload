package tg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"

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
	return len(dbRows), nil
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

func (m *Manager) SyncSaved(ctx context.Context, selfUserID int64) (int, error) {
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
				return nil
			})
	})
	if err != nil {
		return 0, err
	}
	if err := m.DB.ReplaceSavedMessagesCache(ctx, defaultAccountID, ids, srcChat); err != nil {
		return 0, err
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
	return m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		peer := &tg.InputPeerSelf{}
		done := 0
		total := len(messageIDs)
		for _, mid := range messageIDs {
			msg, err := tutil.GetSingleMessage(ctx, api, peer, mid)
			if err != nil {
				if opt.OnItem != nil {
					opt.OnItem(favoritesChatID, mid, "failed", "", "", err.Error())
				}
				continue
			}
			dl := downloader.NewDownloader()
			outRoot := filepath.Join(opt.OutDir, FavoritesFolderName)
			if err := os.MkdirAll(outRoot, 0o755); err != nil {
				return err
			}
			elem := messages.Elem{Msg: msg}
			file, ok := elem.File()
			if !ok {
				if opt.OnItem != nil {
					opt.OnItem(favoritesChatID, mid, "skipped", "", "", "无媒体")
				}
				done++
				if opt.OnProgress != nil {
					opt.OnProgress(done, total, fmt.Sprintf("msg %d 无媒体", mid))
				}
				continue
			}
			if opt.OnItem != nil {
				opt.OnItem(favoritesChatID, mid, "downloading", file.Name, "", "")
			}
			size := mediaSize(msg)
			if opt.SkipSame && opt.Exists != nil && size > 0 {
				if exists, path, err := opt.Exists(favoritesChatID, mid, size); err == nil && exists {
					if opt.OnItem != nil {
						opt.OnItem(favoritesChatID, mid, "skipped", file.Name, path, "")
					}
					if opt.OnFile != nil {
						_ = opt.OnFile(favoritesChatID, mid, file.Name, size, path, file.MIMEType)
					}
					done++
					if opt.OnProgress != nil {
						opt.OnProgress(done, total, file.Name+" (已存在)")
					}
					continue
				}
			}
			name := file.Name
			if name == "" {
				name = fmt.Sprintf("%d_%d", favoritesChatID, mid)
			}
			path := filepath.Join(outRoot, safeFileName(name))
			_, err = dl.Download(api, file.Location).ToPath(ctx, path)
			if err != nil {
				if opt.OnItem != nil {
					opt.OnItem(favoritesChatID, mid, "failed", name, "", err.Error())
				}
				done++
				if opt.OnProgress != nil {
					opt.OnProgress(done, total, name+" 失败")
				}
				continue
			}
			if opt.OnFile != nil {
				_ = opt.OnFile(favoritesChatID, mid, name, size, path, file.MIMEType)
			}
			if opt.OnItem != nil {
				opt.OnItem(favoritesChatID, mid, "done", name, path, "")
			}
			done++
			if opt.OnProgress != nil {
				opt.OnProgress(done, total, name)
			}
		}
		return nil
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
	if _, err := m.SyncSaved(ctx, selfID); err != nil {
		return nil, err
	}
	return m.TGSummary(ctx)
}
