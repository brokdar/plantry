package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:files
var embedded embed.FS

// Handler returns a handler that serves the embedded Vite build with an
// index.html fallback for client-side routes. In Phase 0 the files/ directory
// is empty except for a .keep marker; the handler still works and will start
// serving the SPA as soon as Docker Stage B copies the build in.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "files")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := path.Clean(r.URL.Path)
		trimmed := strings.TrimPrefix(cleaned, "/")
		if trimmed == "" {
			trimmed = "index.html"
		}

		if f, err := sub.Open(trimmed); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		if f, err := sub.Open("index.html"); err == nil {
			_ = f.Close()
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		http.NotFound(w, r)
	}), nil
}
