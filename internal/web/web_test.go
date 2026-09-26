package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

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

func TestMovieRowShowsYearAndAddedDate(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true, snap: library.Snapshot{Movies: []library.Movie{
		{ID: "a", Title: "2001: A Space Odyssey", Year: 1968, Added: time.Date(2024, 2, 14, 23, 0, 0, 0, time.UTC)},
		{ID: "b", Title: "Undated"},
	}}})
	body := get(t, h, "/movies").Body.String()
	for _, want := range []string{
		`<span class="movie-row__year">1968</span>`,
		`<span class="movie-row__added">Added 14 Feb 2024</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if strings.Count(body, "movie-row__added") != 1 {
		t.Error("a movie with no added date still shows an added date")
	}
	// Every row has a poster slot, a placeholder until cover art arrives.
	if n := strings.Count(body, `class="poster poster--placeholder"`); n != 2 {
		t.Errorf("%d placeholder posters, want 2", n)
	}
}

func TestHeaderHasMarkNameTabsAndRefreshSlot(t *testing.T) {
	movies := make([]library.Movie, 1284)
	h := newTestHandler(t, stubSnapshots{ok: true, snap: library.Snapshot{Movies: movies}})
	body := get(t, h, "/movies").Body.String()
	for _, want := range []string{
		`class="app-header__mark"`,
		`>Spool<`,
		`<nav class="app-header__tabs" aria-label="Library">`,
		`<a class="app-header__tab" href="/movies" aria-current="page">Movies <span class="app-header__count">1,284</span></a>`,
		`<a class="app-header__tab" href="/tv">TV shows</a>`,
		`class="app-header__actions"`,
		`<h1 class="page-head__title">Movies</h1>`,
		`<p class="page-head__summary">1,284 movies</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
}

func TestMovieCountHiddenBeforeFirstLoad(t *testing.T) {
	body := get(t, newTestHandler(t, stubSnapshots{}), "/movies").Body.String()
	if strings.Contains(body, "app-header__count") {
		t.Error("tab shows a count before the library has loaded")
	}
}

func TestTVShowsPagePlaceholder(t *testing.T) {
	rec := get(t, newTestHandler(t, stubSnapshots{ok: true}), "/tv")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<title>TV shows · Spool</title>`,
		`<a class="app-header__tab" href="/tv" aria-current="page">TV shows</a>`,
		`<h1 class="page-head__title">TV shows</h1>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if strings.Count(body, `aria-current="page"`) != 1 {
		t.Error("want exactly one current tab")
	}
}

func TestFaviconIsOneEmbeddedSVG(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true})
	body := get(t, h, "/movies").Body.String()
	if !strings.Contains(body, `<link rel="icon" href="/static/favicon.svg" type="image/svg+xml">`) {
		t.Error("page does not link the SVG favicon")
	}
	rec := get(t, h, "/static/favicon.svg")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("favicon: status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	svg := rec.Body.String()
	if strings.Count(svg, "<svg") != 1 || strings.Contains(svg, "prefers-color-scheme") {
		t.Errorf("favicon is not a single dark-only SVG: %s", svg)
	}
}

func TestFontsEmbeddedWithLicence(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true})
	css := get(t, h, "/static/css/components.css").Body.String()
	urls := regexp.MustCompile(`url\("\.\./fonts/([^"]+\.woff2)"\)`).FindAllStringSubmatch(css, -1)
	families := map[string]bool{}
	for _, m := range regexp.MustCompile(`font-family: "([^"]+)";`).FindAllStringSubmatch(css, -1) {
		families[m[1]] = true
	}
	if !families["Instrument Sans"] || !families["IBM Plex Mono"] {
		t.Errorf("@font-face families = %v", families)
	}
	if len(urls) < 2 {
		t.Fatalf("components.css declares %d woff2 fonts", len(urls))
	}
	for _, m := range urls {
		rec := get(t, h, "/static/fonts/"+m[1])
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "wOF2") {
			t.Errorf("font %s: status %d, not woff2", m[1], rec.Code)
		}
	}
	for _, lic := range []string{"OFL-InstrumentSans.txt", "OFL-IBMPlexMono.txt"} {
		if body := get(t, h, "/static/fonts/"+lic).Body.String(); !strings.Contains(body, "SIL Open Font License, Version 1.1") {
			t.Errorf("%s missing or not the OFL", lic)
		}
	}
	// The system stacks stay as the fallback.
	tokens := get(t, h, "/static/css/tokens.css").Body.String()
	if !strings.Contains(tokens, `--font-sans: "Instrument Sans", ui-sans-serif, system-ui`) {
		t.Error("tokens.css lost the system font fallback")
	}
}

func TestMovieRowsShowSizeAndWhoCompleted(t *testing.T) {
	all := []library.User{
		{ID: "s", Name: "sally"}, {ID: "a", Name: "Anastasia Kowalczyk"}, {ID: "h", Name: "H"},
		{ID: "k", Name: "kayla"}, {ID: "b", Name: "bec"}, {ID: "f", Name: "follicle"},
	}
	h := newTestHandler(t, stubSnapshots{ok: true, snap: library.Snapshot{Users: all, Movies: []library.Movie{
		{ID: "1", Title: "Two Users", Size: 15676630630, CompletedBy: []library.User{all[1], all[2]}},
		{ID: "2", Title: "Nobody", Size: 85899345920},
		{ID: "3", Title: "Unknown Size", CompletedBy: all[:1]},
		{ID: "4", Title: "Everyone", Size: 1 << 30, CompletedBy: all},
	}}})
	body := get(t, h, "/movies").Body.String()

	for _, want := range []string{
		`<span class="size__value">14.6</span> <span class="size__unit">GB</span>`,
		`<span class="size__value">80.0</span> <span class="size__unit">GB</span>`,
		`<span class="size size--unknown">Unknown</span>`,
		// Tags carry the name and a two-letter code, so colour is never the
		// only way to tell users apart; hues follow /Users order.
		`<span class="user-code user-code--hue-2" aria-hidden="true">AK</span><span class="user-tag__name">Anastasia Kowalczyk</span>`,
		`<span class="user-code user-code--hue-3" aria-hidden="true">H</span><span class="user-tag__name">H</span>`,
		`<span class="user-code user-code--hue-1" aria-hidden="true">SA</span><span class="user-tag__name">sally</span>`,
		`<p class="completion__none">Not completed by anyone</p>`,
		// Five or more users collapse into an expandable group.
		`<details class="user-group">`,
		`All <span class="user-group__count">6</span> users`,
		`<span class="list-head__size">Size</span>`,
		`<span class="list-head__completion">Completed by</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if n := strings.Count(body, "Not completed by anyone"); n != 1 {
		t.Errorf(`"Not completed by anyone" appears %d times, want 1`, n)
	}
	if strings.Contains(body, ">0<") || strings.Contains(body, ">0 B<") {
		t.Error("an unknown size rendered as a number")
	}
	// The same user has the same tag everywhere.
	if n := strings.Count(body, `<span class="user-code user-code--hue-1" aria-hidden="true">SA</span><span class="user-tag__name">sally</span>`); n != 2 {
		t.Errorf("sally's tag appears %d times, want 2", n)
	}
}

// DESIGN.md §3: nothing marks a row as big, unwatched or otherwise worth
// deleting. Every row carries exactly the same classes, whatever its data.
func TestMovieRowsAreStyledTheSameWhateverTheirData(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true, snap: library.Snapshot{
		Users: []library.User{{ID: "s", Name: "sally"}},
		Movies: []library.Movie{
			{ID: "1", Title: "Huge Unwatched", Size: 85899345920},
			{ID: "2", Title: "Small Watched", Size: 1 << 20, CompletedBy: []library.User{{ID: "s", Name: "sally"}}},
		}}})
	body := get(t, h, "/movies").Body.String()
	rows := regexp.MustCompile(`<li class="([^"]*)"`).FindAllStringSubmatch(body, -1)
	var rowClasses []string
	for _, m := range rows {
		if strings.HasPrefix(m[1], "movie-row") {
			rowClasses = append(rowClasses, m[1])
		}
	}
	if len(rowClasses) != 2 || rowClasses[0] != "movie-row" || rowClasses[1] != "movie-row" {
		t.Errorf("row classes = %v, want two plain movie-row", rowClasses)
	}
	sizes := regexp.MustCompile(`<span class="(size[^"]*)"`).FindAllStringSubmatch(body, -1)
	for _, m := range sizes {
		if m[1] != "size" && m[1] != "size__value" && m[1] != "size__unit" {
			t.Errorf("size styled with %q", m[1])
		}
	}
}
