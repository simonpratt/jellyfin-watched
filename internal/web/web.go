// Package web renders the dashboard: html/template pages, components and the
// static assets (CSS, htmx), all embedded in the binary. It reads library
// data from a snapshot and never talks to Jellyfin itself.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"jellyfin-watched/internal/library"
)

//go:embed templates static
var assets embed.FS

// Snapshots provides the current library snapshot; *library.Cache satisfies it.
type Snapshots interface {
	Snapshot() (library.Snapshot, bool)
}

type handler struct {
	lib   Snapshots
	log   *slog.Logger
	pages map[string]*template.Template
}

// NewHandler returns the dashboard's HTTP handler.
func NewHandler(lib Snapshots, logger *slog.Logger) (http.Handler, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	h := &handler{lib: lib, log: logger, pages: pages}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/movies", http.StatusFound))
	mux.HandleFunc("GET /movies", h.movies)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	return mux, nil
}

// parsePages builds one template set per page: the layout, every component,
// and that page's "content" block.
func parsePages() (map[string]*template.Template, error) {
	base, err := template.ParseFS(assets, "templates/layout.tmpl", "templates/components/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing layout and components: %w", err)
	}
	pageFiles, err := fs.Glob(assets, "templates/pages/*.tmpl")
	if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	for _, f := range pageFiles {
		t, err := template.Must(base.Clone()).ParseFS(assets, f)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}
		name := f[len("templates/pages/") : len(f)-len(".tmpl")]
		pages[name] = t
	}
	return pages, nil
}

func (h *handler) movies(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.lib.Snapshot()
	h.render(w, "movies", "layout", newMoviesPage(snap, ok))
}

// render executes a page into a buffer first, so a template error becomes a
// clean 500 rather than a half-written page.
func (h *handler) render(w http.ResponseWriter, page, block string, data any) {
	var buf bytes.Buffer
	if err := h.pages[page].ExecuteTemplate(&buf, block, data); err != nil {
		h.log.Error("rendering page", "page", page, "err", err)
		http.Error(w, "Something went wrong rendering this page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
