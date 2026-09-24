package library

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
)

const thumbMaxSide = 360

// ThumbCachePath 返回缩略图缓存路径：{downloadDir}/.tdload-thumbs/{chatID}_{messageID}_{size}.jpg
// 键与 media_index 唯一约束一致，下载时可直接写入，无需等待 DB id。
func ThumbCachePath(downloadDir string, chatID int64, messageID int, size int64) string {
	name := fmt.Sprintf("%d_%d_%d.jpg", chatID, messageID, size)
	return filepath.Join(downloadDir, ".tdload-thumbs", name)
}

// EnsureThumb 生成或复用缩略图，返回 JPEG 文件路径。
// kind: image | video
// 视频封面依赖下载时写入的 Telegram document thumb；EnsureThumb 仅读缓存，
// 缺失时由 API 调用 Manager.EnsureVideoThumb 按需补拉。
func EnsureThumb(downloadDir, srcPath, kind string, chatID int64, messageID int, size int64) (string, error) {
	cache := ThumbCachePath(downloadDir, chatID, messageID, size)
	if info, err := os.Stat(cache); err == nil && info.Size() > 0 {
		return cache, nil
	}
	if kind == "video" {
		return "", fmt.Errorf("无视频封面缓存")
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return "", err
	}
	tmp := cache + ".tmp"
	defer func() { _ = os.Remove(tmp) }()
	if err := writeImageThumb(srcPath, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, cache); err != nil {
		return "", err
	}
	return cache, nil
}

// InstallVideoThumb 将已下载的 Telegram 封面（任意图片）压成列表缩略图并写入缓存。
func InstallVideoThumb(downloadDir string, chatID int64, messageID int, size int64, srcPath string) error {
	if size <= 0 || messageID <= 0 {
		return fmt.Errorf("无效封面键")
	}
	cache := ThumbCachePath(downloadDir, chatID, messageID, size)
	if info, err := os.Stat(cache); err == nil && info.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	tmp := cache + ".tmp"
	defer func() { _ = os.Remove(tmp) }()
	if err := writeImageThumb(srcPath, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, cache)
}

// HasVideoThumb 判断视频封面缓存是否已存在。
func HasVideoThumb(downloadDir string, chatID int64, messageID int, size int64) bool {
	cache := ThumbCachePath(downloadDir, chatID, messageID, size)
	info, err := os.Stat(cache)
	return err == nil && info.Size() > 0
}

func writeImageThumb(srcPath, dstPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("无效图片尺寸")
	}
	maxSide := thumbMaxSide
	if w <= maxSide && h <= maxSide {
		return encodeJPEG(dstPath, img, 82)
	}
	scale := float64(maxSide) / float64(w)
	if h > w {
		scale = float64(maxSide) / float64(h)
	}
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return encodeJPEG(dstPath, dst, 82)
}

func encodeJPEG(path string, img image.Image, quality int) error {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// DetectMediaKind 根据 mime / 扩展名判断 image|video|file。
func DetectMediaKind(mime, fileName string) string {
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	lower := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"),
		strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".webp"), strings.HasSuffix(lower, ".gif"):
		return "image"
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".mkv"),
		strings.HasSuffix(lower, ".mov"), strings.HasSuffix(lower, ".webm"):
		return "video"
	default:
		return "file"
	}
}

// PurgeStaleThumbs 清理不在 keep 集合中的缩略图（键：chatID_messageID_size）。
func PurgeStaleThumbs(downloadDir string, keepKeys map[string]struct{}) {
	dir := filepath.Join(downloadDir, ".tdload-thumbs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		key := strings.TrimSuffix(name, filepath.Ext(name))
		if _, ok := keepKeys[key]; !ok {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// ThumbKey 返回与 ThumbCachePath 文件名主体一致的键。
func ThumbKey(chatID int64, messageID int, size int64) string {
	return fmt.Sprintf("%d_%d_%d", chatID, messageID, size)
}
