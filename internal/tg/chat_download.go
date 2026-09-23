package tg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
)

// ChatDownloadParams 频道历史下载参数。
type ChatDownloadParams struct {
	ChatID         int64
	Username       string // 可选，加速解析
	ChatTitle      string
	FromMessageID  int    // chat_batch：起始 message id（含）
	Count          int    // chat_batch：连续 message id 数量
	Mode           string // "batch" | "continue" | "ids"
	AfterMessageID int    // chat_continue：已下载最大 id（不含）
	LastMessageID  int    // chat_continue：对话最新 id，0 表示未知
	MaxMedia       int    // continue 单次上限，默认 5000
	MessageIDs     []int  // mode=ids：仅下载这些消息
	GroupAlbum     bool
}

// DownloadChat 按 chat_batch / chat_continue 扫描历史并下载媒体。
// 返回 scanEnd：本批应推进的扫描水位；0 表示无推进。
func (m *Manager) DownloadChat(ctx context.Context, opt DownloadOptions, p ChatDownloadParams) (int, error) {
	if p.ChatID == 0 {
		return 0, fmt.Errorf("缺少 chat_id")
	}
	if opt.OutDir == "" {
		opt.OutDir = m.Cfg.DownloadDir
	}
	if opt.Threads <= 0 {
		opt.Threads = m.Cfg.Threads
		if opt.Threads <= 0 {
			opt.Threads = 4
		}
	}
	if opt.Threads > 8 {
		opt.Threads = 8
	}
	if opt.Template == "" {
		opt.Template = m.Cfg.Template
	}
	if opt.Template == "" {
		opt.Template = DefaultFileTemplate
	}
	if p.MaxMedia <= 0 {
		p.MaxMedia = 100
	}
	if p.Mode == "batch" {
		if p.FromMessageID <= 0 {
			return 0, fmt.Errorf("请指定起始 message id")
		}
		if p.Count <= 0 {
			p.Count = 50
		}
		if p.Count > 5000 {
			p.Count = 5000
		}
	}

	scanEnd := 0
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		return m.withAPI(ctx, client, func(ctx context.Context, api *tg.Client) error {
		peer, err := resolveChatPeer(ctx, api, p.ChatID, p.Username)
		if err != nil {
			return err
		}
		chatName := p.ChatTitle
		if chatName == "" {
			chatName = peer.VisibleName()
		}
		chatID := peer.ID()

		jobs, end, err := collectChatMedia(ctx, api, peer, p)
		if err != nil {
			return err
		}
		scanEnd = end
		slog.Info("chat download jobs", "chat", chatID, "mode", p.Mode, "jobs", len(jobs), "scanEnd", end)

		if len(jobs) == 0 {
			if opt.OnProgress != nil {
				opt.OnProgress(0, 0, "无需下载")
			}
			return nil
		}

		dl := downloader.NewDownloader()
		if err := os.MkdirAll(opt.OutDir, 0o755); err != nil {
			return err
		}

		total := len(jobs)
		done := 0
		if opt.OnProgress != nil {
			opt.OnProgress(done, total, "准备下载")
		}

		for _, msg := range jobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if opt.OnItem != nil {
				opt.OnItem(chatID, msg.ID, "downloading", "", "", "")
			}
			file, ok := messages.Elem{Msg: msg}.File()
			if !ok {
				done++
				if opt.OnItem != nil {
					opt.OnItem(chatID, msg.ID, "skipped", "", "", "无媒体")
				}
				if opt.OnProgress != nil {
					opt.OnProgress(done, total, fmt.Sprintf("跳过无媒体 msg=%d", msg.ID))
				}
				continue
			}

			size := mediaSize(msg)
			if opt.SkipSame && opt.Exists != nil && size > 0 {
				if exists, path, err := opt.Exists(chatID, msg.ID, size); err == nil && exists {
					if opt.OnItem != nil {
						opt.OnItem(chatID, msg.ID, "skipped", file.Name, path, "")
					}
					if opt.OnFile != nil {
						_ = opt.OnFile(chatID, msg.ID, file.Name, size, path, file.MIMEType)
					}
					done++
					if opt.OnProgress != nil {
						opt.OnProgress(done, total, file.Name+" (已存在)")
					}
					continue
				}
			}

			rawName := file.Name
			if opt.RewriteExt {
				rawName = rewriteExtByMIME(rawName, file.MIMEType)
			}
			name := renderFileName(opt.Template, chatID, msg.ID, msg, rawName, size)
			chatDir := filepath.Join(opt.OutDir, chatFolderName(chatID, chatName))
			if err := os.MkdirAll(chatDir, 0o755); err != nil {
				return err
			}
			path := uniquePath(filepath.Join(chatDir, name))
			threads := tutil.BestThreads(size, opt.Threads)
			if threads < 1 {
				threads = 1
			}

			slog.Info("downloading", "file", name, "path", path, "size", size, "threads", threads)
			fileCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			_, err := dl.Download(api, file.Location).WithThreads(threads).ToPath(fileCtx, path)
			cancel()
			if err != nil {
				_ = os.Remove(path)
				if opt.OnItem != nil {
					opt.OnItem(chatID, msg.ID, "failed", name, "", err.Error())
				}
				done++
				if opt.OnProgress != nil {
					opt.OnProgress(done, total, name+" 失败")
				}
				continue
			}
			if size == 0 {
				if st, err := os.Stat(path); err == nil {
					size = st.Size()
				}
			}
			if opt.OnFile != nil {
				if err := opt.OnFile(chatID, msg.ID, name, size, path, file.MIMEType); err != nil {
					slog.Warn("index media", "err", err)
				}
			}
			if opt.OnItem != nil {
				opt.OnItem(chatID, msg.ID, "done", name, path, "")
			}
			done++
			if opt.OnProgress != nil {
				opt.OnProgress(done, total, name)
			}
			slog.Info("downloaded", "file", name, "path", path)
		}
		return nil
		})
	})
	return scanEnd, err
}

func collectChatMedia(ctx context.Context, api *tg.Client, peer peers.Peer, p ChatDownloadParams) ([]*tg.Message, int, error) {
	input := peer.InputPeer()
	seen := map[int]struct{}{}
	var out []*tg.Message
	scanEnd := 0

	switch p.Mode {
	case "ids":
		for _, mid := range p.MessageIDs {
			if mid <= 0 {
				continue
			}
			if _, ok := seen[mid]; ok {
				continue
			}
			msg, err := tutil.GetSingleMessage(ctx, api, input, mid)
			if err != nil {
				slog.Warn("retry fetch message", "chat", peer.ID(), "msg", mid, "err", err)
				continue
			}
			seen[mid] = struct{}{}
			out = append(out, msg)
			if p.GroupAlbum {
				gctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				grouped, gerr := tutil.GetGroupedMessages(gctx, api, input, msg)
				cancel()
				if gerr == nil {
					for _, gm := range grouped {
						if gm == nil || gm.ID == msg.ID {
							continue
						}
						if _, ok := seen[gm.ID]; ok {
							continue
						}
						seen[gm.ID] = struct{}{}
						out = append(out, gm)
					}
				}
			}
		}
		return out, 0, nil
	case "batch":
		from := p.FromMessageID
		to := from + p.Count - 1
		chunk, err := fetchMediaInRange(ctx, api, input, from, to, seen)
		if err != nil {
			return nil, 0, err
		}
		out = chunk
		scanEnd = to
	default: // continue：从水位之后向前填充，避免只抓「最新 N 条」漏掉中间历史
		after := p.AfterMessageID
		maxMedia := p.MaxMedia
		if maxMedia <= 0 {
			maxMedia = 100
		}
		latest := p.LastMessageID
		from := after + 1
		if latest > 0 && from > latest {
			return nil, latest, nil
		}
		window := maxMedia * 8
		if window < 400 {
			window = 400
		}
		if window > 3000 {
			window = 3000
		}
		scanEnd = after
		for len(out) < maxMedia {
			if latest > 0 && from > latest {
				scanEnd = latest
				break
			}
			to := from + window - 1
			if latest > 0 && to > latest {
				to = latest
			}
			chunk, err := fetchMediaInRange(ctx, api, input, from, to, seen)
			if err != nil {
				return nil, scanEnd, err
			}
			out = append(out, chunk...)
			if len(out) >= maxMedia {
				sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
				out = out[:maxMedia]
				scanEnd = out[len(out)-1].ID
				break
			}
			scanEnd = to
			if latest > 0 && to >= latest {
				break
			}
			if to < from {
				break
			}
			from = to + 1
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if !p.GroupAlbum {
		return out, scanEnd, nil
	}
	expanded, err := expandAlbums(ctx, api, input, out)
	return expanded, scanEnd, err
}

func fetchMediaInRange(ctx context.Context, api *tg.Client, input tg.InputPeerClass, from, to int, seen map[int]struct{}) ([]*tg.Message, error) {
	if from <= 0 || to < from {
		return nil, nil
	}
	var out []*tg.Message
	iter := messages.NewQueryBuilder(api).GetHistory(input).BatchSize(100).OffsetID(to + 1).Iter()
	for iter.Next(ctx) {
		msg, ok := iter.Value().Msg.(*tg.Message)
		if !ok {
			continue
		}
		if msg.ID < from {
			break
		}
		if msg.ID > to {
			continue
		}
		if !tutil.FileExists(msg) {
			continue
		}
		if _, ok := seen[msg.ID]; ok {
			continue
		}
		seen[msg.ID] = struct{}{}
		out = append(out, msg)
	}
	if err := iter.Err(); err != nil {
		return nil, errors.Wrap(err, "扫描频道历史")
	}
	return out, nil
}

func expandAlbums(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, msgs []*tg.Message) ([]*tg.Message, error) {
	uniq := map[int]*tg.Message{}
	for _, msg := range msgs {
		uniq[msg.ID] = msg
		gctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		grouped, err := tutil.GetGroupedMessages(gctx, api, peer, msg)
		cancel()
		if err != nil || len(grouped) == 0 {
			continue
		}
		for _, gm := range grouped {
			if tutil.FileExists(gm) {
				uniq[gm.ID] = gm
			}
		}
	}
	final := make([]*tg.Message, 0, len(uniq))
	for _, m := range uniq {
		final = append(final, m)
	}
	sort.Slice(final, func(i, j int) bool { return final[i].ID < final[j].ID })
	return final, nil
}

func resolveChatPeer(ctx context.Context, api *tg.Client, chatID int64, username string) (peers.Peer, error) {
	manager := peers.Options{}.Build(api)
	if username = strings.TrimPrefix(strings.TrimSpace(username), "@"); username != "" {
		if p, err := manager.Resolve(ctx, username); err == nil {
			return p, nil
		}
	}
	// 先拉对话列表，写入 peers 缓存（含 access_hash）
	elems, err := dialogs.NewQueryBuilder(api).GetDialogs().BatchSize(100).Collect(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "获取对话列表")
	}
	plain := plainChannelID(chatID)
	for _, el := range elems {
		if el.Deleted() {
			continue
		}
		p, err := manager.FromInputPeer(ctx, el.Peer)
		if err != nil {
			continue
		}
		if p.ID() == chatID || p.ID() == plain {
			return p, nil
		}
		if ch, ok := p.(peers.Channel); ok {
			if channelMarkedID(ch.ID()) == chatID {
				return p, nil
			}
		}
	}
	if p, err := tutil.GetInputPeer(ctx, manager, strconv.FormatInt(plain, 10)); err == nil {
		return p, nil
	}
	if plain != chatID {
		if p, err := tutil.GetInputPeer(ctx, manager, strconv.FormatInt(chatID, 10)); err == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("找不到对话 %d，请先在 Telegram 页同步频道", chatID)
}

func channelMarkedID(channelID int64) int64 {
	return -(1_000_000_000_000 + channelID)
}

func plainChannelID(id int64) int64 {
	if id >= 0 {
		return id
	}
	s := strconv.FormatInt(id, 10)
	if strings.HasPrefix(s, "-100") {
		n, err := strconv.ParseInt(strings.TrimPrefix(s, "-100"), 10, 64)
		if err == nil {
			return n
		}
	}
	return id
}
