package tg

import (
	"path/filepath"
	"strings"
)

// 监听内容类型：all=全部附件，media=图片+视频，image/video=仅对应类型。
const (
	ContentAll   = "all"
	ContentMedia = "media"
	ContentImage = "image"
	ContentVideo = "video"
)

// NormalizeContentType 规范化内容类型；未知值视为全部。
func NormalizeContentType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ContentMedia, ContentImage, ContentVideo:
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return ContentAll
	}
}

// MatchContentType 判断文件是否符合内容类型筛选。
func MatchContentType(contentType, mime, fileName string) bool {
	ct := NormalizeContentType(contentType)
	if ct == ContentAll {
		return true
	}
	kind := classifyMediaKind(mime, fileName)
	switch ct {
	case ContentMedia:
		return kind == ContentImage || kind == ContentVideo
	case ContentImage:
		return kind == ContentImage
	case ContentVideo:
		return kind == ContentVideo
	default:
		return true
	}
}

func classifyMediaKind(mime, fileName string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if strings.HasPrefix(mime, "image/") {
		return ContentImage
	}
	if strings.HasPrefix(mime, "video/") {
		return ContentVideo
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff", ".heic", ".heif":
		return ContentImage
	case ".mp4", ".mkv", ".mov", ".webm", ".avi", ".m4v", ".mpeg", ".mpg":
		return ContentVideo
	default:
		return "file"
	}
}
