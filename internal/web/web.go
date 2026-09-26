// Package web renders the dashboard: html/template pages, components and the
// static assets (CSS, htmx), all embedded in the binary. It reads library
// data from a snapshot and never talks to Jellyfin itself.
package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"jellyfin-watched/internal/library"
)

//go:embed templates static
var assets embed.FS

// Snapshots provides the current library snapshot; *library.Cache satisfies it.
type Snapshots interface {
	Snapshot() (library.Snapshot, bool)
}

// Posters provides poster images; *artwork.Cache satisfies it. tag is the
// poster's current tag in the snapshot.
type Posters interface {
	Poster(ctx context.Context, itemID, tag string) (data []byte, contentType string, err error)
}

type handler struct {
	lib     Snapshots
	posters Posters
	log     *slog.Logger
	pages   map[string]*template.Template
}

// NewHandler returns the dashboard's HTTP handler.
func NewHandler(lib Snapshots, posters Posters, logger *slog.Logger) (http.Handler, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	h := &handler{lib: lib, posters: posters, log: logger, pages: pages}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/movies", http.StatusFound))
	mux.HandleFunc("GET /movies", h.movies)
	mux.HandleFunc("GET /tv", h.tv)
	mux.HandleFunc("GET "+posterPath+"{id}/{tag}", h.poster)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	return mux, nil
}

// parseBase parses the layout and every component, without any page.
func parseBase() (*template.Template, error) {
	base, err := template.ParseFS(assets, "templates/layout.tmpl", "templates/components/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing layout and components: %w", err)
	}
	return base, nil
}

// parsePages builds one template set per page: the layout, every component,
// and that page's "content" block.
func parsePages() (map[string]*template.Template, error) {
	base, err := parseBase()
	if err != nil {
		return nil, err
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

func (h *handler) tv(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.lib.Snapshot()
	h.render(w, "tv", "layout", newTVPage(snap, ok))
}

// poster serves a poster from the cache, fetching it from Jellyfin on first
// use. Only items in the library snapshot that have a poster are served, so
// the route can't be used to reach anything else on Jellyfin. The response
// is built afresh: no Jellyfin header is passed through.
func (h *handler) poster(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.lib.Snapshot()
	if !ok {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	tag, ok := snap.PosterTag(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, contentType, err := h.posters.Poster(r.Context(), id, tag)
	if err != nil {
		if r.Context().Err() == nil {
			h.log.Warn("fetching poster", "item", id, "err", err)
		}
		http.Error(w, "The poster couldn't be fetched from Jellyfin.", http.StatusBadGateway)
		return
	}
	// The URL carries the tag, which changes with the image, so a matching
	// URL can be cached for good. An old tag (a page rendered before a
	// refresh) gets the current poster, revalidated each time.
	cache := "public, max-age=31536000, immutable"
	if r.PathValue("tag") != tag {
		cache = "no-cache"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", cache)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
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
