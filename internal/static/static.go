package static

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// FileServer 托管前端构建产物；未知路径回退 index.html（SPA）。
func FileServer(webDir string) http.Handler {
	root := http.Dir(webDir)
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		if p != "/" {
			f, err := root.Open(strings.TrimPrefix(p, "/"))
			if err == nil {
				stat, err := f.Stat()
				_ = f.Close()
				if err == nil && !stat.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			} else if !os.IsNotExist(err) {
				http.Error(w, "static error", http.StatusInternalServerError)
				return
			}
			// SPA fallback
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func Exists(webDir string) bool {
	info, err := os.Stat(webDir)
	return err == nil && info.IsDir()
}
