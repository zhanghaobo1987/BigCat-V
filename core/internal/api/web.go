package api

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed webui
var webUI embed.FS

// webFS 内嵌的 Web UI 文件系统（发布构建时由 scripts/build-release.sh
// 将 desktop/src 拷贝到 webui/ 后编译；开发构建仅含 .gitkeep，
// 此时 index.html 不存在，Web 路由自动关闭，仅提供 API）。
func webFS() fs.FS {
	sub, err := fs.Sub(webUI, "webui")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}

func contentType(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// handleWeb 内嵌 Web UI：单页应用，不存在的路径回退 index.html。
func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "." || p == "" {
		p = "index.html"
	}
	if _, err := fs.Stat(s.web, p); err != nil {
		p = "index.html"
	}
	data, err := fs.ReadFile(s.web, p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType(p))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
