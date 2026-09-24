package tg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// resolveDownloadLimits 规范化线程数与文件并发数。
func resolveDownloadLimits(opt *DownloadOptions, cfgThreads, cfgConc int) (threads, concurrency int) {
	threads = opt.Threads
	if threads <= 0 {
		threads = cfgThreads
	}
	if threads <= 0 {
		threads = 4
	}
	if threads > 32 {
		threads = 32
	}
	opt.Threads = threads

	concurrency = opt.Concurrency
	if concurrency <= 0 {
		concurrency = cfgConc
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 16 {
		concurrency = 16
	}
	opt.Concurrency = concurrency
	return threads, concurrency
}

// cbGuard 串行化进度回调，避免并发写库 / SSE 乱序踩踏。
type cbGuard struct {
	mu  sync.Mutex
	opt *DownloadOptions
}

func (g *cbGuard) item(chatID int64, messageID int, status, fileName, localPath, errMsg string) {
	if g == nil || g.opt == nil || g.opt.OnItem == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opt.OnItem(chatID, messageID, status, fileName, localPath, errMsg)
}

func (g *cbGuard) file(chatID int64, messageID int, fileName string, size int64, path, mime string) error {
	if g == nil || g.opt == nil || g.opt.OnFile == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.opt.OnFile(chatID, messageID, fileName, size, path, mime)
}

func (g *cbGuard) progress(done, total int, title string) {
	if g == nil || g.opt == nil || g.opt.OnProgress == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opt.OnProgress(done, total, title)
}

var pathAllocMu sync.Mutex

// allocUniquePath 并发安全地预占落盘路径（O_EXCL 创建空文件）。
func allocUniquePath(path string) (string, error) {
	pathAllocMu.Lock()
	defer pathAllocMu.Unlock()
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	try := path
	for i := 0; i < 1000; i++ {
		if i > 0 {
			try = fmt.Sprintf("%s_%d%s", base, i, ext)
		}
		f, err := os.OpenFile(try, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return try, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
	try = fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
	f, err := os.OpenFile(try, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	_ = f.Close()
	return try, nil
}

// bumpDone 原子递增已完成计数并推送进度。
func bumpDone(done *atomic.Int32, total int, title string, g *cbGuard) {
	n := int(done.Add(1))
	g.progress(n, total, title)
}
