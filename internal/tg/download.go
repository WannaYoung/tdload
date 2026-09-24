package tg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
	"golang.org/x/sync/errgroup"
)

type DownloadProgress func(doneFiles, totalFiles int, fileName string)

type DownloadOptions struct {
	URLs         []string
	OutDir       string
	OutSubdir    string
	Threads      int
	Concurrency  int // 同时下载的文件数（对应 tdl -l）
	Template     string
	GroupAlbum   bool
	SkipSame     bool
	RewriteExt   bool
	ContentType  string // all|media|image|video，空视为 all
	OnProgress   DownloadProgress
	Exists       func(chatID int64, messageID int, size int64) (bool, string, error)
	OnFile       func(chatID int64, messageID int, fileName string, size int64, path, mime string) error
	OnItem       func(chatID int64, messageID int, status, fileName, localPath, errMsg string)
	OnResolved   func(info ChatInfo) // 解析到真实频道后回调
	// OnScanProgress：扫描待下载媒体时回调（found 递增）；OnReadyToDownload：扫描结束、开始下载前。
	OnScanProgress    func(found int)
	OnReadyToDownload func(total int)
}

// ChatInfo 解析后的频道/群信息。
type ChatInfo struct {
	ChatID   int64
	Title    string
	Username string
	Kind     string // channel | group
}

// DefaultFileTemplate 默认文件名模板（占位符 {{Field}}，无 Go 模板点号）。
const DefaultFileTemplate = "{{DialogID }}-{{MessageID }}-{{FileName }}"

// fileTplField 匹配 {{DialogID}} / {{ DialogID }} / 旧式 {{ .DialogID }}。
var fileTplField = regexp.MustCompile(`\{\{\s*\.?([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Run runs fn with an authorized Telegram client.
func (m *Manager) Run(ctx context.Context, fn func(ctx context.Context, client *telegram.Client) error) error {
	if err := m.lockClient(ctx); err != nil {
		return err
	}
	defer m.clientMu.Unlock()

	client, err := m.newClient()
	if err != nil {
		return err
	}
	return client.Run(ctx, func(ctx context.Context) error {
		st, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !st.Authorized {
			return fmt.Errorf("Telegram 未登录")
		}
		return fn(ctx, client)
	})
}

func (m *Manager) lockClient(ctx context.Context) error {
	for {
		if m.clientMu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) DownloadURLs(ctx context.Context, opt DownloadOptions) error {
	if len(opt.URLs) == 0 {
		return fmt.Errorf("没有链接")
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
	outRoot := opt.OutDir
	if opt.OutSubdir != "" {
		outRoot = filepath.Join(opt.OutDir, safeFileName(opt.OutSubdir))
	}
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return err
	}

	return m.Run(ctx, func(ctx context.Context, client *telegram.Client) error {
		return m.withAPI(ctx, client, func(ctx context.Context, api *tg.Client) error {
		manager := peers.Options{}.Build(api)
		dl := downloader.NewDownloader()

		type job struct {
			chatID   int64
			chatName string
			msgID    int
			msg      *tg.Message
		}
		var jobs []job

		for _, raw := range opt.URLs {
			u := normalizeURL(raw)
			slog.Info("parse url", "url", u)
			peer, msgID, err := tutil.ParseMessageLink(ctx, manager, u)
			if err != nil {
				return errors.Wrapf(err, "解析链接 %s", raw)
			}
			msg, err := tutil.GetSingleMessage(ctx, api, peer.InputPeer(), msgID)
			if err != nil {
				return errors.Wrapf(err, "获取消息 %s", raw)
			}
			msgs := []*tg.Message{msg}
			if opt.GroupAlbum {
				gctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				grouped, gerr := tutil.GetGroupedMessages(gctx, api, peer.InputPeer(), msg)
				cancel()
				if gerr == nil && len(grouped) > 0 {
					msgs = grouped
					slog.Info("album messages", "count", len(msgs), "url", u)
				}
			}
			chatName := peer.VisibleName()
			for _, mm := range msgs {
				jobs = append(jobs, job{chatID: peer.ID(), chatName: chatName, msgID: mm.ID, msg: mm})
			}
		}

		total := len(jobs)
		if total == 0 {
			return fmt.Errorf("没有可下载的消息")
		}
		var done atomic.Int32
		var saved atomic.Int32
		var skipped atomic.Int32
		cb := &cbGuard{opt: &opt}
		if opt.OnProgress != nil {
			opt.OnProgress(0, total, "准备下载")
		}

		type fileJob struct {
			chatID   int64
			chatName string
			msgID    int
			name     string
			path     string
			size     int64
			mime     string
			loc      tg.InputFileLocationClass
		}
		var toFetch []fileJob

		for _, j := range jobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			file, ok := messages.Elem{Msg: j.msg}.File()
			if !ok {
				skipped.Add(1)
				cb.item(j.chatID, j.msgID, "skipped", "", "", "无媒体")
				bumpDone(&done, total, fmt.Sprintf("跳过无媒体 msg=%d", j.msgID), cb)
				continue
			}

			size := mediaSize(j.msg)
			if opt.SkipSame && opt.Exists != nil && size > 0 {
				if exists, path, err := opt.Exists(j.chatID, j.msgID, size); err == nil && exists {
					cb.item(j.chatID, j.msgID, "skipped", file.Name, path, "")
					_ = cb.file(j.chatID, j.msgID, file.Name, size, path, file.MIMEType)
					saved.Add(1)
					bumpDone(&done, total, file.Name+" (已存在)", cb)
					continue
				}
			}

			rawName := file.Name
			if opt.RewriteExt {
				rawName = rewriteExtByMIME(rawName, file.MIMEType)
			}
			name := renderFileName(opt.Template, j.chatID, j.msgID, j.msg, rawName, size)
			chatDir := filepath.Join(outRoot, chatFolderName(j.chatID, j.chatName))
			if opt.OutSubdir != "" {
				chatDir = outRoot
			}
			if err := os.MkdirAll(chatDir, 0o755); err != nil {
				return err
			}
			path, err := allocUniquePath(filepath.Join(chatDir, name))
			if err != nil {
				cb.item(j.chatID, j.msgID, "failed", name, "", err.Error())
				bumpDone(&done, total, name+" 失败", cb)
				continue
			}
			toFetch = append(toFetch, fileJob{
				chatID: j.chatID, chatName: j.chatName, msgID: j.msgID,
				name: name, path: path, size: size, mime: file.MIMEType, loc: file.Location,
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
				cb.item(fj.chatID, fj.msgID, "downloading", fj.name, "", "")
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
					cb.item(fj.chatID, fj.msgID, "failed", fj.name, "", err.Error())
					bumpDone(&done, total, fj.name+" 失败", cb)
					return errors.Wrapf(err, "下载 %s", fj.name)
				}
				size := fj.size
				if size == 0 {
					if st, err := os.Stat(fj.path); err == nil {
						size = st.Size()
					}
				}
				if err := cb.file(fj.chatID, fj.msgID, fj.name, size, fj.path, fj.mime); err != nil {
					slog.Warn("index media", "err", err)
				}
				cb.item(fj.chatID, fj.msgID, "done", fj.name, fj.path, "")
				saved.Add(1)
				bumpDone(&done, total, fj.name, cb)
				slog.Info("downloaded", "file", fj.name, "path", fj.path)
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
		if saved.Load() == 0 {
			return fmt.Errorf("未下载到任何文件（共 %d 条消息，跳过 %d）", total, skipped.Load())
		}
		return nil
		})
	})
}

func chatFolderName(chatID int64, chatName string) string {
	id := strconv.FormatInt(chatID, 10)
	name := safeFileName(strings.TrimSpace(chatName))
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" || name == "." || name == ".." {
		return id
	}
	folder := id + "-" + name
	if len(folder) > 120 {
		folder = folder[:120]
	}
	return folder
}

func renderFileName(tpl string, chatID int64, msgID int, msg *tg.Message, rawName string, size int64) string {
	base := safeFileName(rawName)
	if base == "" {
		base = fmt.Sprintf("%d_%d", chatID, msgID)
	}
	caption := ""
	var msgDate int64
	if msg != nil {
		caption = msg.Message
		msgDate = int64(msg.Date)
	}
	vals := map[string]string{
		"DialogID":     strconv.FormatInt(chatID, 10),
		"MessageID":    strconv.Itoa(msgID),
		"MessageDate":  strconv.FormatInt(msgDate, 10),
		"FileName":     base,
		"FileCaption":  caption,
		"FileSize":     strconv.FormatInt(size, 10),
		"DownloadDate": strconv.FormatInt(time.Now().Unix(), 10),
	}
	name := fileTplField.ReplaceAllStringFunc(tpl, func(m string) string {
		sub := fileTplField.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		if v, ok := vals[sub[1]]; ok {
			return v
		}
		return ""
	})
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	// 模板只允许文件名；目录统一用频道 ID
	name = filepath.Base(name)
	name = safeFileName(name)
	if name == "" || name == "." || name == ".." {
		name = base
	}
	return name
}

func mediaSize(msg *tg.Message) int64 {
	switch m := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		if d, ok := m.Document.AsNotEmpty(); ok {
			return d.Size
		}
	case *tg.MessageMediaPhoto:
		// photo 无准确 size，返回 0
	}
	return 0
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") {
		s = "https://" + strings.TrimPrefix(s, "http://")
	}
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "t.me/") {
		if strings.Contains(s, "t.me/") {
			i := strings.Index(s, "t.me/")
			s = "https://" + s[i:]
		}
	}
	if strings.HasPrefix(s, "t.me/") {
		s = "https://" + s
	}
	return s
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = unsafeName.ReplaceAllString(name, "_")
	if len(name) > 180 {
		ext := filepath.Ext(name)
		name = name[:180-len(ext)] + ext
	}
	return name
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; i < 1000; i++ {
		p := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
	return fmt.Sprintf("%s_%d%s", base, time.Now().Unix(), ext)
}
