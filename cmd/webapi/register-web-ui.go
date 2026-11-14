//go:build webui

package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/webui"
)

// registerWebUI mounts the embedded Vue SPA under /dashboard. Hashed assets
// are served from dist/; unknown extension-less paths fall back to index.html
// so client-side routes survive a full page reload. /api and every other path
// stay on the API handler.
func registerWebUI(hdl http.Handler) (http.Handler, error) {
	distDirectory, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("error embedding WebUI dist/ directory: %w", err)
	}

	index, err := fs.ReadFile(distDirectory, "index.html")
	if err != nil {
		return nil, fmt.Errorf("error reading embedded WebUI index.html: %w", err)
	}

	fileServer := http.FileServer(http.FS(distDirectory))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dashboard" {
			http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/dashboard/") {
			hdl.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Clean the URL-derived name so ".." segments can never escape dist/.
		name := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, "/dashboard/")), "/")
		if name != "" {
			if info, statErr := fs.Stat(distDirectory, name); statErr == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				w.Header().Set("X-Content-Type-Options", "nosniff")
				http.StripPrefix("/dashboard", fileServer).ServeHTTP(w, r)
				return
			}
			// A missing asset must stay a 404 instead of becoming HTML.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(index)
	}), nil
}
