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
	"sync/atomic"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/iyear/tdl/core/util/tutil"
	"golang.org/x/sync/errgroup"
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
	if p.ChatID == 0 && strings.TrimSpace(p.Username) == "" {
		return 0, fmt.Errorf("缺少 chat_id 或用户名")
	}
	if opt.OutDir == "" {
		opt.OutDir = m.Cfg.DownloadDir
	}
	_, conc := resolveDownloadLimits(&opt, m.Cfg.Threads, m.Cfg.Concurrency)
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
		peer, err := resolveChatPeer(ctx, api, p.ChatID, p.Username, true)
		if err != nil {
			return friendlyChatAccessErr(err)
		}
		info := peerChatInfo(peer)
		chatName := info.Title
		if chatName == "" {
			chatName = strings.TrimSpace(p.ChatTitle)
		}
		chatID := info.ChatID
		if opt.OnResolved != nil && (info.Title != "" || info.Username != "" || info.ChatID != 0) {
			opt.OnResolved(info)
		}

		jobs, end, err := collectChatMedia(ctx, api, peer, p, opt.OnScanProgress)
		if err != nil {
			return friendlyChatAccessErr(err)
		}
		scanEnd = end
		slog.Info("chat download jobs", "chat", chatID, "mode", p.Mode, "jobs", len(jobs), "scanEnd", end)

		if opt.OnReadyToDownload != nil {
			opt.OnReadyToDownload(len(jobs))
		} else if opt.OnScanProgress != nil {
			opt.OnScanProgress(len(jobs))
		}

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
		var done atomic.Int32
		cb := &cbGuard{opt: &opt}
		if opt.OnProgress != nil {
			opt.OnProgress(0, total, "准备下载")
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

		for _, msg := range jobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			file, ok := messages.Elem{Msg: msg}.File()
			if !ok {
				cb.item(chatID, msg.ID, "skipped", "", "", "无媒体")
				bumpDone(&done, total, fmt.Sprintf("跳过无媒体 msg=%d", msg.ID), cb)
				continue
			}
			if !MatchContentType(opt.ContentType, file.MIMEType, file.Name) {
				cb.item(chatID, msg.ID, "skipped", file.Name, "", "类型不符")
				bumpDone(&done, total, file.Name+" (类型不符)", cb)
				continue
			}

			size := mediaSize(msg)
			if opt.SkipSame && opt.Exists != nil && size > 0 {
				if exists, path, err := opt.Exists(chatID, msg.ID, size); err == nil && exists {
					cb.item(chatID, msg.ID, "skipped", file.Name, path, "")
					_ = cb.file(chatID, msg.ID, file.Name, size, path, file.MIMEType)
					bumpDone(&done, total, file.Name+" (已存在)", cb)
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
			path, err := allocUniquePath(filepath.Join(chatDir, name))
			if err != nil {
				cb.item(chatID, msg.ID, "failed", name, "", err.Error())
				bumpDone(&done, total, name+" 失败", cb)
				continue
			}
			toFetch = append(toFetch, fileJob{
				msgID: msg.ID, name: name, path: path, size: size, mime: file.MIMEType, loc: file.Location,
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
				cb.item(chatID, fj.msgID, "downloading", fj.name, "", "")
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
					cb.item(chatID, fj.msgID, "failed", fj.name, "", err.Error())
					bumpDone(&done, total, fj.name+" 失败", cb)
					return nil
				}
				size := fj.size
				if size == 0 {
					if st, err := os.Stat(fj.path); err == nil {
						size = st.Size()
					}
				}
				if err := cb.file(chatID, fj.msgID, fj.name, size, fj.path, fj.mime); err != nil {
					slog.Warn("index media", "err", err)
				}
				cb.item(chatID, fj.msgID, "done", fj.name, fj.path, "")
				bumpDone(&done, total, fj.name, cb)
				slog.Info("downloaded", "file", fj.name, "path", fj.path)
				return nil
			})
		}
		return eg.Wait()
		})
	})
	return scanEnd, err
}

func collectChatMedia(ctx context.Context, api *tg.Client, peer peers.Peer, p ChatDownloadParams, onProgress func(found int)) ([]*tg.Message, int, error) {
	input := peer.InputPeer()
	seen := map[int]struct{}{}
	var out []*tg.Message
	scanEnd := 0
	report := func() {
		if onProgress != nil {
			onProgress(len(out))
		}
	}

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
			report()
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
					report()
				}
			}
		}
		return out, 0, nil
	case "batch":
		from := p.FromMessageID
		to := from + p.Count - 1
		chunk, err := fetchMediaInRange(ctx, api, input, from, to, seen, 0, func(n int) {
			if onProgress != nil {
				onProgress(len(out) + n)
			}
		})
		if err != nil {
			return nil, 0, err
		}
		out = chunk
		scanEnd = to
		report()
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
		capReport := func(n int) {
			if onProgress == nil {
				return
			}
			if n > maxMedia {
				n = maxMedia
			}
			onProgress(n)
		}
		for len(out) < maxMedia {
			if latest > 0 && from > latest {
				scanEnd = latest
				break
			}
			to := from + window - 1
			if latest > 0 && to > latest {
				to = latest
			}
			base := len(out)
			remain := maxMedia - base
			chunk, err := fetchMediaInRange(ctx, api, input, from, to, seen, remain, func(n int) {
				capReport(base + n)
			})
			if err != nil {
				return nil, scanEnd, err
			}
			out = append(out, chunk...)
			capReport(len(out))
			if len(out) >= maxMedia {
				sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
				out = out[:maxMedia]
				scanEnd = out[len(out)-1].ID
				capReport(len(out))
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
		report()
		return out, scanEnd, nil
	}
	expanded, err := expandAlbums(ctx, api, input, out)
	if err != nil {
		return nil, scanEnd, err
	}
	out = expanded
	report()
	return out, scanEnd, nil
}

func fetchMediaInRange(ctx context.Context, api *tg.Client, input tg.InputPeerClass, from, to int, seen map[int]struct{}, limit int, onFound func(n int)) ([]*tg.Message, error) {
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
		if onFound != nil {
			onFound(len(out))
		}
		if limit > 0 && len(out) >= limit {
			break
		}
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

func peerChatInfo(peer peers.Peer) ChatInfo {
	info := ChatInfo{
		ChatID: peer.ID(),
		Title:  strings.TrimSpace(peer.VisibleName()),
		Kind:   "channel",
	}
	if u, ok := peer.Username(); ok {
		info.Username = strings.TrimPrefix(strings.TrimSpace(u), "@")
	}
	switch p := peer.(type) {
	case peers.Chat:
		info.Kind = "group"
		_ = p
	case peers.Channel:
		if p.IsSupergroup() {
			info.Kind = "group"
		} else {
			info.Kind = "channel"
		}
	}
	return info
}

// ResolveChatInfo 解析频道/群的真实 ID、标题与用户名（公开未加入可用 @username）。
func (m *Manager) ResolveChatInfo(ctx context.Context, chatID int64, username string) (*ChatInfo, error) {
	var info ChatInfo
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		peer, err := resolveChatPeer(ctx, client.API(), chatID, username, false)
		if err != nil {
			return err
		}
		info = peerChatInfo(peer)
		return nil
	})
	if err != nil {
		return nil, friendlyChatAccessErr(err)
	}
	return &info, nil
}

// FetchChatSnapshot 解析 peer 并取最新消息 ID。
// 水位走 messages.getPeerDialogs（与同步对话同类接口），不用 getHistory，避免普通会话被限流。
func (m *Manager) FetchChatSnapshot(ctx context.Context, chatID int64, username string) (*ChatInfo, int, error) {
	var info ChatInfo
	var lastMessageID int
	err := m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		peer, err := resolveChatPeer(ctx, api, chatID, username, false)
		if err != nil {
			return err
		}
		switch peer.(type) {
		case peers.Channel, peers.Chat:
			// ok
		default:
			return fmt.Errorf("仅支持频道或群组，不能添加用户会话")
		}
		info = peerChatInfo(peer)
		topID, topErr := peerTopMessageID(ctx, api, peer)
		if topErr != nil {
			if isChatAccessDenied(topErr) {
				return topErr
			}
			slog.Warn("snapshot top message", "chatId", info.ChatID, "err", topErr)
			return nil
		}
		lastMessageID = topID
		return nil
	})
	if err != nil {
		return nil, 0, friendlyChatAccessErr(err)
	}
	return &info, lastMessageID, nil
}

func peerTopMessageID(ctx context.Context, api *tg.Client, peer peers.Peer) (int, error) {
	res, err := api.MessagesGetPeerDialogs(ctx, []tg.InputDialogPeerClass{
		&tg.InputDialogPeer{Peer: peer.InputPeer()},
	})
	if err != nil {
		return 0, err
	}
	top := 0
	for _, d := range res.Dialogs {
		if dialog, ok := d.(*tg.Dialog); ok && dialog.TopMessage > top {
			top = dialog.TopMessage
		}
	}
	if top > 0 {
		return top, nil
	}
	for _, m := range res.Messages {
		if msg, ok := m.(*tg.Message); ok && msg.ID > top {
			top = msg.ID
		}
	}
	return top, nil
}

func resolveChatPeer(ctx context.Context, api *tg.Client, chatID int64, username string, allowDialogScan bool) (peers.Peer, error) {
	manager := peers.Options{}.Build(api)
	var accessErr error
	if username = strings.TrimPrefix(strings.TrimSpace(username), "@"); username != "" {
		p, err := manager.Resolve(ctx, username)
		if err == nil {
			return p, nil
		}
		if isChatAccessDenied(err) || tgerr.Is(err, "USERNAME_INVALID") || tgerr.Is(err, "USERNAME_NOT_OCCUPIED") {
			// 用户名明确无权 / 不存在：直接返回，避免被后续模糊错误盖住
			if chatID == 0 {
				return nil, err
			}
			accessErr = err
		} else if chatID == 0 {
			return nil, fmt.Errorf("找不到公开频道 @%s：%w", username, err)
		}
	}

	plain := plainChannelID(chatID)
	// 优先直接解析（公开频道 username 已试过；数字 ID / 已缓存 access_hash）
	tryIDs := []int64{chatID, plain}
	if plain > 0 {
		tryIDs = append(tryIDs, channelMarkedID(plain))
	}
	seen := map[int64]struct{}{}
	for _, id := range tryIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		p, err := tutil.GetInputPeer(ctx, manager, strconv.FormatInt(id, 10))
		if err == nil {
			return p, nil
		}
		if isChatAccessDenied(err) {
			accessErr = err
		}
	}

	if !allowDialogScan {
		if accessErr != nil {
			return nil, accessErr
		}
		if username != "" {
			return nil, fmt.Errorf("找不到频道 @%s：可能不存在、未加入或为非公开频道", username)
		}
		return nil, fmt.Errorf("找不到频道 %d：未加入的非公开频道无法访问；公开频道请填 @用户名", chatID)
	}

	// 已加入频道：从对话列表补 access_hash
	elems, err := dialogs.NewQueryBuilder(api).GetDialogs().BatchSize(100).Collect(ctx)
	if err != nil {
		if accessErr != nil {
			return nil, accessErr
		}
		return nil, errors.Wrap(err, "获取对话列表")
	}
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
			if channelMarkedID(ch.ID()) == chatID || ch.ID() == plain || channelMarkedID(ch.ID()) == channelMarkedID(plain) {
				return p, nil
			}
		}
	}
	if accessErr != nil {
		return nil, accessErr
	}
	if username != "" {
		return nil, fmt.Errorf("找不到频道 @%s：可能不存在、未加入或为非公开频道", username)
	}
	return nil, fmt.Errorf("找不到频道 %d：未加入的非公开频道无法访问；公开频道请填 @用户名", chatID)
}

// NormalizeChannelChatID 将 t.me/c/<id> 的正数为带 -100 前缀的频道 ID。
func NormalizeChannelChatID(id int64) int64 {
	if id > 0 {
		return channelMarkedID(id)
	}
	return id
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
