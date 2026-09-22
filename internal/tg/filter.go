package tg

import (
	"path/filepath"
	"strings"
)

// rewriteExtByMIME 按 MIME 纠正扩展名（对齐 tdl --rewrite-ext）。
func rewriteExtByMIME(name, mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	want := ""
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		want = ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		want = ".png"
	case strings.HasPrefix(mime, "image/webp"):
		want = ".webp"
	case strings.HasPrefix(mime, "image/gif"):
		want = ".gif"
	case strings.HasPrefix(mime, "video/mp4"):
		want = ".mp4"
	case strings.HasPrefix(mime, "video/webm"):
		want = ".webm"
	case strings.HasPrefix(mime, "video/quicktime"):
		want = ".mov"
	case strings.HasPrefix(mime, "audio/mpeg"):
		want = ".mp3"
	case strings.HasPrefix(mime, "application/pdf"):
		want = ".pdf"
	default:
		return name
	}
	cur := strings.ToLower(filepath.Ext(name))
	if cur == want {
		return name
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		return name
	}
	return base + want
}
