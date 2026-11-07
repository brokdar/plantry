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

const (
	// Vite names everything under /assets after a content hash, so a cached
	// copy can never go stale.
	cacheImmutable = "public, max-age=31536000, immutable"
	// index.html and the other unhashed files are revalidated on each load,
	// so a new build is picked up at once.
	cacheRevalidate = "no-cache"
)

// Handler returns a handler that serves the embedded Vite build with an
// index.html fallback for client-side routes. In Phase 0 the files/ directory
// is empty except for a .keep marker; the handler still works and will start
// serving the SPA as soon as Docker Stage B copies the build in.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "files")
	if err != nil {
		return nil, err
	}
	return newHandler(sub), nil
}

func newHandler(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := path.Clean(r.URL.Path)
		trimmed := strings.TrimPrefix(cleaned, "/")
		if trimmed == "" {
			trimmed = "index.html"
		}

		if f, err := sub.Open(trimmed); err == nil {
			_ = f.Close()
			if strings.HasPrefix(trimmed, "assets/") {
				w.Header().Set("Cache-Control", cacheImmutable)
			} else {
				w.Header().Set("Cache-Control", cacheRevalidate)
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// A missing asset is a 404, not the SPA shell: a stale tab asking for
		// a chunk of an older build must get an error it can recover from,
		// not HTML where it expects a script.
		if strings.HasPrefix(trimmed, "assets/") || path.Ext(trimmed) != "" {
			http.NotFound(w, r)
			return
		}

		if f, err := sub.Open("index.html"); err == nil {
			_ = f.Close()
			w.Header().Set("Cache-Control", cacheRevalidate)
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		http.NotFound(w, r)
	})
}
