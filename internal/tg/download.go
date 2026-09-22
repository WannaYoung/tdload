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
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
)

type DownloadProgress func(doneFiles, totalFiles int, fileName string)

type DownloadOptions struct {
	URLs       []string
	OutDir     string
	OutSubdir  string
	Threads    int
	Template   string
	GroupAlbum bool
	SkipSame   bool
	RewriteExt bool
	OnProgress DownloadProgress
	Exists     func(chatID int64, messageID int, size int64) (bool, string, error)
	OnFile     func(chatID int64, messageID int, fileName string, size int64, path, mime string) error
	OnItem     func(chatID int64, messageID int, status, fileName, localPath, errMsg string)
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
		done := 0
		saved := 0
		skipped := 0
		if opt.OnProgress != nil {
			opt.OnProgress(done, total, "准备下载")
		}

		for _, j := range jobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if opt.OnItem != nil {
				opt.OnItem(j.chatID, j.msgID, "downloading", "", "", "")
			}
			file, ok := messages.Elem{Msg: j.msg}.File()
			if !ok {
				skipped++
				done++
				if opt.OnItem != nil {
					opt.OnItem(j.chatID, j.msgID, "skipped", "", "", "无媒体")
				}
				if opt.OnProgress != nil {
					opt.OnProgress(done, total, fmt.Sprintf("跳过无媒体 msg=%d", j.msgID))
				}
				continue
			}

			size := mediaSize(j.msg)
			if opt.SkipSame && opt.Exists != nil && size > 0 {
				if exists, path, err := opt.Exists(j.chatID, j.msgID, size); err == nil && exists {
					if opt.OnItem != nil {
						opt.OnItem(j.chatID, j.msgID, "skipped", file.Name, path, "")
					}
					if opt.OnFile != nil {
						_ = opt.OnFile(j.chatID, j.msgID, file.Name, size, path, file.MIMEType)
					}
					saved++
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
			name := renderFileName(opt.Template, j.chatID, j.msgID, j.msg, rawName, size)
			chatDir := filepath.Join(outRoot, chatFolderName(j.chatID, j.chatName))
			if opt.OutSubdir != "" {
				chatDir = outRoot
			}
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
			builder := dl.Download(api, file.Location).WithThreads(threads)
			_, err := builder.ToPath(fileCtx, path)
			cancel()
			if err != nil {
				_ = os.Remove(path)
				if opt.OnItem != nil {
					opt.OnItem(j.chatID, j.msgID, "failed", name, "", err.Error())
				}
				return errors.Wrapf(err, "下载 %s", name)
			}
			if size == 0 {
				if st, err := os.Stat(path); err == nil {
					size = st.Size()
				}
			}
			if opt.OnFile != nil {
				if err := opt.OnFile(j.chatID, j.msgID, name, size, path, file.MIMEType); err != nil {
					slog.Warn("index media", "err", err)
				}
			}
			if opt.OnItem != nil {
				opt.OnItem(j.chatID, j.msgID, "done", name, path, "")
			}
			saved++
			done++
			if opt.OnProgress != nil {
				opt.OnProgress(done, total, name)
			}
			slog.Info("downloaded", "file", name, "path", path)
		}
		if saved == 0 {
			return fmt.Errorf("未下载到任何文件（共 %d 条消息，跳过 %d）", total, skipped)
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
