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
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"jellyfin-watched/internal/library"
)

//go:embed templates static
var assets embed.FS

// Library provides the current library snapshot and the state of its
// refreshes, and starts a refresh on request; *library.Cache satisfies it.
type Library interface {
	Status() library.Status
	// RefreshNow starts a background refresh unless one is running.
	RefreshNow() bool
}

// Posters provides poster images; *artwork.Cache satisfies it. tag is the
// poster's current tag in the snapshot.
type Posters interface {
	Poster(ctx context.Context, itemID, tag string) (data []byte, contentType string, err error)
}

type handler struct {
	lib     Library
	posters Posters
	log     *slog.Logger
	pages   map[string]*template.Template
	now     func() time.Time
	gate    *gate // nil without a password
}

// NewHandler returns the dashboard's HTTP handler. A non-empty password
// puts every page, fragment and image behind the password screen (see
// auth.go).
func NewHandler(lib Library, posters Posters, logger *slog.Logger, password string) (http.Handler, error) {
	return newHandler(lib, posters, logger, time.Now, password)
}

// newHandler is NewHandler with a clock, for "N minutes ago" and session
// expiry.
func newHandler(lib Library, posters Posters, logger *slog.Logger, now func() time.Time, password string) (http.Handler, error) {
	pages, err := parsePages()
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	h := &handler{lib: lib, posters: posters, log: logger, pages: pages, now: now}
	if password != "" {
		if h.gate, err = newGate(password, now, logger); err != nil {
			return nil, err
		}
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", http.RedirectHandler("/movies", http.StatusFound))
	mux.HandleFunc("GET /movies", h.movies)
	mux.HandleFunc("GET /tv", h.tv)
	mux.HandleFunc("GET "+refreshPath, h.refreshState)
	mux.HandleFunc("POST "+refreshPath, h.refreshNow)
	mux.HandleFunc("GET "+seriesPath+"{id}", h.series)
	mux.HandleFunc("GET "+posterPath+"{id}/{tag}", h.poster)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET "+loginPath, h.passwordScreen)
	mux.HandleFunc("POST "+loginPath, h.login)
	if h.gate != nil {
		return h.gate.protect(mux), nil
	}
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

// movies serves the Movies page; see listView.
func (h *handler) movies(w http.ResponseWriter, r *http.Request) {
	v, ok := h.listView(w, r, "")
	if !ok {
		return
	}
	h.render(w, "movies", listBlock(w, r), newMoviesPage(h.lib.Status(), h.now(), v))
}

// tv serves the TV shows page; see listView. ?seasons=all or none expands
// or collapses every series ("Expand all"), which swaps the page's content
// into <main>.
func (h *handler) tv(w http.ResponseWriter, r *http.Request) {
	seasons := r.URL.Query().Get("seasons")
	if seasons != "all" && seasons != "none" {
		seasons = ""
	}
	v, ok := h.listView(w, r, seasons)
	if !ok {
		return
	}
	h.render(w, "tv", listBlock(w, r), newTVPage(h.lib.Status(), h.now(), v, seasons))
}

// listView reads a list page's search and sort from its URL (see
// parseView), which is where they live so that a reload, a shared link or
// the page reloading itself after a refresh shows the same view.
//
// A browser submitting the list-controls form without JavaScript sends
// every field (?q=&sort=title&dir=asc&set-sort=year); it is redirected to
// the tidy URL for the view (?sort=year), and listView reports false. So is
// any other URL with parameters beyond the view's own; the order they come
// in doesn't matter. An
// htmx request from the form is answered in place, and the response puts
// the tidy URL in the address bar instead.
func (h *handler) listView(w http.ResponseWriter, r *http.Request, seasons string) (library.View, bool) {
	query := r.URL.Query()
	v := parseView(query)
	if r.Header.Get("HX-Request") != "true" {
		if !maps.EqualFunc(query, viewQuery(v, seasons), slices.Equal) {
			http.Redirect(w, r, viewURL(r.URL.Path, v, seasons), http.StatusSeeOther)
			return v, false
		}
		return v, true
	}
	if r.Header.Get("HX-Trigger") == listControlsID {
		w.Header().Set("HX-Replace-Url", viewURL(r.URL.Path, v, seasons))
	}
	return v, true
}

// listBlock is the block a list page renders: an htmx request (the search
// and sort form, or "Expand all") gets the page's content alone; a normal
// load, htmx restoring a page missing from its history cache, or the page
// reloading itself after a refresh (targeting #app) gets the whole page.
func listBlock(w http.ResponseWriter, r *http.Request) string {
	w.Header().Add("Vary", "HX-Request, HX-Target")
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" &&
		r.Header.Get("HX-Target") != "app" {
		return "list-fragment"
	}
	return "layout"
}

// refreshNow is "Refresh now": it starts a background refresh, unless one
// is already running, and answers straight away with the refresh control
// in its "Refreshing…" state. The list stays as it is until the refresh
// has finished.
func (h *handler) refreshNow(w http.ResponseWriter, r *http.Request) {
	h.lib.RefreshNow()
	h.renderRefreshControl(w, h.lib.Status())
}

// refreshState serves the refresh control, which fetches itself every few
// seconds. ?since is the state the page was rendered with (see stateKey);
// once the data or the failure state has changed, the response triggers
// "library-updated", on which the page reloads itself.
func (h *handler) refreshState(w http.ResponseWriter, r *http.Request) {
	st := h.lib.Status()
	if since := r.URL.Query().Get("since"); since != "" && since != stateKey(st) {
		w.Header().Set("HX-Trigger", "library-updated")
	}
	h.renderRefreshControl(w, st)
}

func (h *handler) renderRefreshControl(w http.ResponseWriter, st library.Status) {
	w.Header().Set("Cache-Control", "no-store")
	h.render(w, "movies", "refresh-control", newRefreshControl(st, h.now()))
}

// series serves one series row as an htmx fragment, with its seasons shown
// (?seasons=show) or hidden (?seasons=hide). The row's disclosure button
// swaps it in place of itself.
func (h *handler) series(w http.ResponseWriter, r *http.Request) {
	st := h.lib.Status()
	snap, ok := st.Snapshot, st.Loaded
	if !ok {
		http.NotFound(w, r)
		return
	}
	sr, ok := snap.SeriesByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	row := newSeriesRow(sr, newUserTags(snap.Users), len(snap.Users), parseExpansion(r.URL.Query().Get("seasons")))
	h.render(w, "tv", "series-row", row)
}

// poster serves a poster from the cache, fetching it from Jellyfin on first
// use. Only items in the library snapshot that have a poster are served, so
// the route can't be used to reach anything else on Jellyfin. The response
// is built afresh: no Jellyfin header is passed through.
func (h *handler) poster(w http.ResponseWriter, r *http.Request) {
	st := h.lib.Status()
	snap, ok := st.Snapshot, st.Loaded
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
	// Behind a password, only the browser may keep it, not a shared cache.
	cache := "public, max-age=31536000, immutable"
	if h.gate != nil {
		cache = "private, max-age=31536000, immutable"
	}
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
	h.renderStatus(w, http.StatusOK, page, block, data)
}

// renderStatus is render with a status other than 200.
func (h *handler) renderStatus(w http.ResponseWriter, status int, page, block string, data any) {
	var buf bytes.Buffer
	if err := h.pages[page].ExecuteTemplate(&buf, block, data); err != nil {
		h.log.Error("rendering page", "page", page, "err", err)
		http.Error(w, "Something went wrong rendering this page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
