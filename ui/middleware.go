// Package ui embeds the built single-page interface.
package ui

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Middleware fronts the API handler with the embedded single-page UI: /api/*
// requests pass through to the handler, everything else is served from the
// embedded dist tree with fallback to index.html.
func Middleware(handler http.Handler) http.Handler {
	staticFS, err := fs.Sub(FS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}
	return newStaticHandler(handler, staticFS)
}

func newStaticHandler(apiHandler http.Handler, staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	indexHTML, indexErr := fs.ReadFile(staticFS, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		assetPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if assetPath == "" || path.Ext(assetPath) == "" {
			serveIndex(w, indexHTML, indexErr)
			return
		}
		if _, err := fs.Stat(staticFS, assetPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, indexHTML []byte, indexErr error) {
	if indexErr != nil {
		http.Error(w, "index.html not found in embedded UI", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}
