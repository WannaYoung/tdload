package tg

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// MediaFilter 对齐 watched_chats.filter_json / 任务 options。
type MediaFilter struct {
	IncludeExt []string `json:"include_ext"`
	ExcludeExt []string `json:"exclude_ext"`
	MinSize    int64    `json:"min_size"`
	KeywordAny []string `json:"keyword_any"`
}

func ParseMediaFilter(raw string) MediaFilter {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return MediaFilter{}
	}
	var f MediaFilter
	_ = json.Unmarshal([]byte(raw), &f)
	return f
}

func (f MediaFilter) Empty() bool {
	return len(f.IncludeExt) == 0 && len(f.ExcludeExt) == 0 && f.MinSize <= 0 && len(f.KeywordAny) == 0
}

// Match 判断文件是否通过过滤；空过滤器恒为 true。
func (f MediaFilter) Match(fileName string, size int64, caption string) bool {
	if f.Empty() {
		return true
	}
	if f.MinSize > 0 && size < f.MinSize {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if len(f.ExcludeExt) > 0 && containsFold(f.ExcludeExt, ext) {
		return false
	}
	if len(f.IncludeExt) > 0 && !containsFold(f.IncludeExt, ext) {
		return false
	}
	if len(f.KeywordAny) > 0 {
		hay := strings.ToLower(fileName + " " + caption)
		ok := false
		for _, kw := range f.KeywordAny {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw != "" && strings.Contains(hay, kw) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func containsFold(list []string, v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, x := range list {
		if strings.ToLower(strings.TrimSpace(x)) == v {
			return true
		}
	}
	return false
}

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
