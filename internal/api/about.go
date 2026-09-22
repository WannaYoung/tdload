package api

import (
	"net/http"

	"tdload/internal/version"
)

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]any{
		"name":      "TDLoad",
		"version":   version.Version,
		"license":   version.License,
		"sourceUrl": version.SourceURL,
		"notice":    "本软件基于 AGPL-3.0 许可，包含 iyear/tdl 等第三方组件，详见仓库 NOTICE。",
	})
}
