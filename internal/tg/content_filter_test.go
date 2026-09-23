package tg

import "testing"

func TestMatchContentType(t *testing.T) {
	cases := []struct {
		ct, mime, name string
		want           bool
	}{
		{"all", "application/pdf", "a.pdf", true},
		{"all", "image/jpeg", "a.jpg", true},
		{"media", "image/jpeg", "a.jpg", true},
		{"media", "video/mp4", "a.mp4", true},
		{"media", "application/pdf", "a.pdf", false},
		{"media", "audio/mpeg", "a.mp3", false},
		{"image", "image/png", "a.png", true},
		{"image", "video/mp4", "a.mp4", false},
		{"video", "video/webm", "a.webm", true},
		{"video", "image/jpeg", "a.jpg", false},
		{"video", "", "clip.mkv", true},
		{"image", "", "photo.webp", true},
		{"", "application/zip", "a.zip", true},
		{"unknown", "application/zip", "a.zip", true},
	}
	for _, c := range cases {
		got := MatchContentType(c.ct, c.mime, c.name)
		if got != c.want {
			t.Fatalf("MatchContentType(%q,%q,%q)=%v want %v", c.ct, c.mime, c.name, got, c.want)
		}
	}
}

func TestNormalizeContentType(t *testing.T) {
	if NormalizeContentType("") != ContentAll {
		t.Fatal("empty")
	}
	if NormalizeContentType("IMAGE") != ContentImage {
		t.Fatal("IMAGE")
	}
}
