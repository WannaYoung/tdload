package library

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
)

const thumbMaxSide = 360

// ThumbCachePath 返回缩略图缓存路径：{downloadDir}/.tdload-thumbs/{id}_{mtime}_{size}.jpg
func ThumbCachePath(downloadDir string, mediaID int64, mtimeUnix, size int64) string {
	name := fmt.Sprintf("%d_%d_%d.jpg", mediaID, mtimeUnix, size)
	return filepath.Join(downloadDir, ".tdload-thumbs", name)
}

// EnsureThumb 生成或复用缩略图，返回 JPEG 文件路径。
// kind: image | video
func EnsureThumb(downloadDir, srcPath, kind string, mediaID int64) (string, error) {
	st, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}
	cache := ThumbCachePath(downloadDir, mediaID, st.ModTime().Unix(), st.Size())
	if info, err := os.Stat(cache); err == nil && info.Size() > 0 {
		return cache, nil
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return "", err
	}
	tmp := cache + ".tmp"
	defer func() { _ = os.Remove(tmp) }()

	switch kind {
	case "video":
		if err := extractVideoFrame(srcPath, tmp); err != nil {
			return "", err
		}
		// 视频帧再压成列表缩略图尺寸
		if err := writeImageThumb(tmp, tmp); err != nil {
			return "", err
		}
	default:
		if err := writeImageThumb(srcPath, tmp); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, cache); err != nil {
		return "", err
	}
	return cache, nil
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
		// 已足够小：仍编码为 jpeg 统一缓存格式
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

func extractVideoFrame(src, dst string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("未安装 ffmpeg")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-ss", "0.5",
		"-i", src,
		"-frames:v", "1",
		"-q:v", "4",
		dst,
	)
	if err := cmd.Run(); err != nil {
		return err
	}
	if st, err := os.Stat(dst); err != nil || st.Size() == 0 {
		return fmt.Errorf("视频封面生成失败")
	}
	return nil
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

// PurgeStaleThumbs 可选：清理过期缩略图缓存（按文件名前缀 mediaID）。
func PurgeStaleThumbs(downloadDir string, keepIDs map[int64]struct{}) {
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
		under := strings.IndexByte(name, '_')
		if under <= 0 {
			continue
		}
		id, err := strconv.ParseInt(name[:under], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := keepIDs[id]; !ok {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
