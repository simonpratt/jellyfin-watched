package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"jellyfin-watched/internal/library"
)

type stubSnapshots struct {
	snap library.Snapshot
	ok   bool
}

func (s stubSnapshots) Snapshot() (library.Snapshot, bool) { return s.snap, s.ok }

func newTestHandler(t *testing.T, s Snapshots) http.Handler {
	t.Helper()
	h, err := NewHandler(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestMoviesPageListsTitlesAndYears(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true, snap: library.Snapshot{Movies: []library.Movie{
		{ID: "a", Title: "Alien", Year: 1979},
		{ID: "b", Title: "Dr. Strangelove or: How I Learned to Stop Worrying & Love the Bomb", Year: 1964},
		{ID: "c", Title: "No Year Known"},
	}}})
	rec := get(t, h, "/movies")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Alien", "1979",
		"Dr. Strangelove or: How I Learned to Stop Worrying &amp; Love the Bomb", "1964",
		"No Year Known", "3 movies",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(body, ">0<") {
		t.Error("unknown year rendered as 0")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestMoviesPageBeforeFirstLoad(t *testing.T) {
	rec := get(t, newTestHandler(t, stubSnapshots{}), "/movies")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hasn't loaded yet") {
		t.Errorf("status %d body %s", rec.Code, rec.Body)
	}
}

func TestMoviesPageEmptyLibrary(t *testing.T) {
	rec := get(t, newTestHandler(t, stubSnapshots{ok: true}), "/movies")
	if !strings.Contains(rec.Body.String(), "No movies in the library.") {
		t.Errorf("body %s", rec.Body)
	}
}

func TestRootRedirectsToMovies(t *testing.T) {
	rec := get(t, newTestHandler(t, stubSnapshots{}), "/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/movies" {
		t.Errorf("status %d Location %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := get(t, newTestHandler(t, stubSnapshots{}), "/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("/nope status %d", rec.Code)
	}
}

// Everything the page references is served from the binary; nothing comes
// from a CDN or any other host.
func TestAssetsEmbeddedAndLocal(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true})
	body := get(t, h, "/movies").Body.String()

	refs := regexp.MustCompile(`(?:src|href)="([^"]*)"`).FindAllStringSubmatch(body, -1)
	if len(refs) < 2 {
		t.Fatalf("expected stylesheet and script references, got %v", refs)
	}
	for _, m := range refs {
		ref := m[1]
		if !strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "//") {
			t.Errorf("non-local reference %q", ref)
			continue
		}
		if rec := get(t, h, ref); rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("GET %s: status %d, %d bytes", ref, rec.Code, rec.Body.Len())
		}
	}
	for _, want := range []string{"/static/htmx.min.js", "/static/css/components.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not reference %s", want)
		}
	}
	if !strings.Contains(get(t, h, "/static/htmx.min.js").Body.String(), "htmx") {
		t.Error("htmx.min.js is not htmx")
	}
}
