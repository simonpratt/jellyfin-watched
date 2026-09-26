package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"jellyfin-watched/internal/library"
)

// searchMovies are in Jellyfin's sort-name order.
var searchMovies = stubSnapshots{ok: true, snap: library.Snapshot{
	Users: users("Sam"),
	Movies: []library.Movie{
		{ID: "alien", Title: "Alien", Year: 1979, Added: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Size: 30 << 30},
		{ID: "blade", Title: "Blade Runner", Year: 1982, Added: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Size: 50 << 30},
		{ID: "blade2049", Title: "Blade Runner 2049", Year: 2017, Added: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "matrix", Title: "The Matrix", Year: 1999, Added: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), Size: 20 << 30},
	},
	Series: []library.Series{
		{ID: "andor", Title: "Andor", Year: 2022, Added: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Size: 90 << 30,
			Seasons: []library.Season{{ID: "a1", Name: "Season 1", Episodes: 12, Size: 90 << 30}}},
		{ID: "flea", Title: "Fleabag", Year: 2016, Added: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Size: 12 << 30},
		{ID: "simpsons", Title: "The Simpsons", Year: 1989, Added: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Size: 2000 << 30,
			Seasons: manySeasons(35)},
	},
}}

var rowTitle = regexp.MustCompile(`<h2 class="(?:movie|series)-row__title">([^<]*)</h2>`)

// titles lists the row titles in a page, in order.
func titles(body string) []string {
	var out []string
	for _, m := range rowTitle.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func htmxGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, "GET", path, map[string]string{"HX-Request": "true", "HX-Trigger": "list-controls", "HX-Target": "results"})
}

func TestSearchFiltersBothTabsIgnoringCase(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/movies?q=BLADE").Body.String()
	if got := strings.Join(titles(body), "|"); got != "Blade Runner|Blade Runner 2049" {
		t.Errorf("movies q=BLADE: %s", got)
	}
	assertContains(t, body, `<p id="page-summary" class="page-head__summary">2 of 4 movies match</p>`,
		`value="BLADE"`)

	tv := get(t, h, "/tv?q=simp").Body.String()
	if got := strings.Join(titles(tv), "|"); got != "The Simpsons" {
		t.Errorf("tv q=simp: %s", got)
	}
	assertContains(t, tv, `<p id="page-summary" class="page-head__summary">1 of 3 series match</p>`)
}

func TestSortBothTabsBothDirections(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	for _, tc := range []struct{ path, want string }{
		{"/movies", "Alien|Blade Runner|Blade Runner 2049|The Matrix"},
		{"/movies?dir=desc", "The Matrix|Blade Runner 2049|Blade Runner|Alien"},
		{"/movies?sort=year", "Alien|Blade Runner|The Matrix|Blade Runner 2049"},
		{"/movies?sort=year&dir=desc", "Blade Runner 2049|The Matrix|Blade Runner|Alien"},
		{"/movies?sort=added", "Blade Runner|Blade Runner 2049|Alien|The Matrix"},
		{"/movies?sort=added&dir=desc", "The Matrix|Alien|Blade Runner 2049|Blade Runner"},
		// Unknown sizes sort last in both directions.
		{"/movies?sort=size", "The Matrix|Alien|Blade Runner|Blade Runner 2049"},
		{"/movies?sort=size&dir=desc", "Blade Runner|Alien|The Matrix|Blade Runner 2049"},
		{"/tv", "Andor|Fleabag|The Simpsons"},
		{"/tv?dir=desc", "The Simpsons|Fleabag|Andor"},
		{"/tv?sort=year", "The Simpsons|Fleabag|Andor"},
		{"/tv?sort=year&dir=desc", "Andor|Fleabag|The Simpsons"},
		{"/tv?sort=added", "Fleabag|Andor|The Simpsons"},
		{"/tv?sort=added&dir=desc", "The Simpsons|Andor|Fleabag"},
		// Series sort by their total size.
		{"/tv?sort=size", "Fleabag|Andor|The Simpsons"},
		{"/tv?sort=size&dir=desc", "The Simpsons|Andor|Fleabag"},
		{"/movies?q=blade&sort=size&dir=desc", "Blade Runner|Blade Runner 2049"},
	} {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", tc.path, rec.Code)
			continue
		}
		if got := strings.Join(titles(rec.Body.String()), "|"); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.path, got, tc.want)
		}
	}
}

// The controls are a plain GET form: with no JavaScript, pressing Enter in
// the search box or any sort button submits it, and the page redirects to
// the tidy URL for that view.
func TestControlsAreAPlainForm(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/movies?q=blade&sort=size&dir=desc").Body.String()
	assertContains(t, body,
		`<form id="list-controls" class="list-controls" role="search" action="/movies" method="get"`,
		`<input type="search" id="search-q" class="search__input" name="q" value="blade" placeholder="Search titles" aria-label="Search titles"`,
		`<input type="hidden" name="sort" value="size">`,
		`<input type="hidden" name="dir" value="desc">`,
		`<button type="submit" id="sort-size" class="sort-control__segment" name="set-sort" value="size" aria-pressed="true">Size</button>`,
		`<button type="submit" id="sort-title" class="sort-control__segment" name="set-sort" value="title" aria-pressed="false">Title</button>`,
		`name="set-dir" value="asc"`, `Descending</button>`,
		`name="clear" value="1"`,
	)
	// Enter submits with the form's default button: a nameless one ahead
	// of every other, so it adds nothing to the query.
	form := body[strings.Index(body, `<form id="list-controls"`):]
	if first := regexp.MustCompile(`<button [^>]*>`).FindString(form); !strings.Contains(first, `class="list-controls__default"`) || strings.Contains(first, "name=") {
		t.Errorf("first button in the form is %s", first)
	}

	for _, tc := range []struct{ path, want string }{
		{"/movies?q=blade&sort=title&dir=asc", "/movies?q=blade"},
		{"/movies?q=blade&sort=title&dir=asc&set-sort=year", "/movies?q=blade&sort=year"},
		{"/movies?q=&sort=year&dir=asc&set-dir=desc", "/movies?dir=desc&sort=year"},
		{"/movies?q=blade&sort=size&dir=desc&clear=1", "/movies?dir=desc&sort=size"},
		{"/movies?q=++blade+&sort=bogus", "/movies?q=blade"},
		{"/movies?q=&sort=title&dir=asc", "/movies"},
		{"/tv?seasons=all&q=simp&sort=title&dir=asc&set-sort=size", "/tv?q=simp&seasons=all&sort=size"},
	} {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.want {
			t.Errorf("%s: %d to %q, want 303 to %q", tc.path, rec.Code, rec.Header().Get("Location"), tc.want)
		}
	}
	// A tidy URL is served as it is.
	for _, path := range []string{"/movies", "/movies?q=blade", "/movies?dir=desc&sort=year", "/tv?q=simp&seasons=all&sort=size"} {
		if rec := get(t, h, path); rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
}

// With htmx, the form fetches the page's content and swaps in the results,
// summary and sort control, leaving the search box (and its caret) alone.
// The response puts the tidy URL in the address bar.
func TestControlsSwapResultsWithHtmx(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/movies").Body.String()
	assertContains(t, body,
		`hx-get="/movies" hx-trigger="submit, input delay:200ms from:#search-q" hx-target="#results" hx-select="#results" hx-swap="outerHTML" hx-select-oob="#page-summary,#sort-control,#layout-control" hx-sync="this:replace"`,
		`<div id="results" class="results">`,
		`<div id="sort-control" class="sort-control">`,
	)
	tv := get(t, h, "/tv").Body.String()
	assertContains(t, tv, `hx-get="/tv"`, `hx-select-oob="#page-summary,#sort-control,#layout-control,#tv-expand-all"`)

	rec := htmxGet(t, h, "/movies?q=blade&sort=title&dir=asc&set-sort=size")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("HX-Replace-Url"); got != "/movies?q=blade&sort=size" {
		t.Errorf("HX-Replace-Url = %q", got)
	}
	frag := rec.Body.String()
	assertNotContains(t, frag, "<html", "app-header__actions")
	assertContains(t, frag, `<div id="results" class="results">`, `id="page-summary"`, `aria-pressed="true">Size</button>`)
	if got := strings.Join(titles(frag), "|"); got != "Blade Runner|Blade Runner 2049" {
		t.Errorf("fragment rows: %s", got)
	}

	// Clearing from htmx drops the search.
	rec = htmxGet(t, h, "/movies?q=blade&sort=title&dir=asc&clear=1")
	if got := rec.Header().Get("HX-Replace-Url"); got != "/movies" {
		t.Errorf("clear: HX-Replace-Url = %q", got)
	}
}

// "Expand all" keeps the search and sort, and the form keeps the expansion.
func TestTVExpandAllComposesWithSearchAndSort(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/tv?dir=desc&q=o&sort=size").Body.String()
	assertContains(t, body, `hx-get="/tv?dir=desc&amp;q=o&amp;seasons=all&amp;sort=size"`)
	all := get(t, h, "/tv?dir=desc&q=o&seasons=all&sort=size").Body.String()
	assertContains(t, all, `hx-get="/tv?dir=desc&amp;q=o&amp;seasons=none&amp;sort=size"`,
		`<input type="hidden" name="seasons" value="all">`)
	if n := strings.Count(all, `aria-expanded="true"`); n != 2 {
		t.Errorf("seasons=all with search: %d expanded, want 2", n)
	}
}

func TestNoSearchResults(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/movies?q=xyz").Body.String()
	assertContains(t, body,
		`<h2 class="empty-state__headline">No movies match “xyz”</h2>`,
		`<p class="empty-state__line">Search looks at titles only.</p>`,
		`form="list-controls" name="clear" value="1"`, `Clear search</button>`,
		`<p id="page-summary" class="page-head__summary">0 of 4 movies match</p>`,
		`<div id="results" class="results">`,
	)
	assertNotContains(t, body, "list-head")
	tv := get(t, h, "/tv?q=%3Cb%3E").Body.String()
	assertContains(t, tv, `<h2 class="empty-state__headline">No TV shows match “&lt;b&gt;”</h2>`, "Clear search</button>")
	assertNotContains(t, tv, "<b>")
}

// An empty library has its own state, and nothing to search or sort.
func TestEmptyLibraryHasNoControls(t *testing.T) {
	h := newTestHandler(t, stubSnapshots{ok: true})
	for _, path := range []string{"/movies", "/tv"} {
		body := get(t, h, path).Body.String()
		assertContains(t, body, "on this server</h2>", `<div id="results" class="results">`)
		assertNotContains(t, body, "list-controls", "search__input", "sort-control")
	}
	// Before the first load there's no page head at all.
	assertNotContains(t, get(t, newTestHandler(t, stubSnapshots{}), "/movies").Body.String(), "list-controls")
}

// DESIGN.md §3: every sort is presented the same way; no key is styled or
// labelled differently from the others.
func TestEverySortLooksTheSame(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	for _, sort := range []string{"", "?sort=year", "?sort=added", "?sort=size"} {
		body := get(t, h, "/movies"+sort).Body.String()
		for _, m := range regexp.MustCompile(`<button type="submit" id="sort-(?:title|year|added|size)" class="([^"]*)"`).FindAllStringSubmatch(body, -1) {
			if m[1] != "sort-control__segment" {
				t.Errorf("%s: segment styled %q", sort, m[1])
			}
		}
		sortControl := body[strings.Index(body, `<div id="sort-control"`):strings.Index(body, `<div id="layout-control"`)]
		if n := strings.Count(sortControl, `aria-pressed="true"`); n != 1 {
			t.Errorf("%s: %d segments selected", sort, n)
		}
	}
}

// The tabs carry the sort, direction and layout to the other tab, but not
// the search or the TV page's expansion.
func TestTabsCarrySortAndLayoutButNotSearch(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	body := get(t, h, "/movies?q=blade&sort=size&dir=desc&layout=grid").Body.String()
	assertContains(t, body,
		`<nav id="app-tabs" class="app-header__tabs" aria-label="Library">`,
		`href="/movies?dir=desc&amp;layout=grid&amp;sort=size" aria-current="page">Movies`,
		`href="/tv?dir=desc&amp;layout=grid&amp;sort=size">TV shows`)

	tv := get(t, h, "/tv?q=simp&sort=year&seasons=all").Body.String()
	assertContains(t, tv, `href="/movies?sort=year">Movies`, `href="/tv?sort=year" aria-current="page">TV shows`)

	// A search or sort swap sends the tabs along, swapped out of band into
	// the header, so they follow the new sort.
	frag := htmxGet(t, h, "/movies?q=blade&set-sort=year").Body.String()
	assertContains(t, frag, `<nav id="app-tabs" class="app-header__tabs" aria-label="Library" hx-swap-oob="true">`,
		`href="/tv?sort=year">TV shows`)
	assertNotContains(t, frag, "<html", "app-header__actions")
}

func TestMoviesSummaryShowsSizeOnDisk(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	// 30 + 50 + 20 GiB known; Blade Runner 2049's size is unknown.
	assertContains(t, get(t, h, "/movies").Body.String(),
		`<p id="page-summary" class="page-head__summary">4 movies · 100.0 GB on disk</p>`)
}
