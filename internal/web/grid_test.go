package web

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"jellyfin-watched/internal/library"
)

// Both tabs have the layout control: List and Grid, as submit buttons of the
// list-controls form, so they work from the keyboard and without
// JavaScript. The current layout is a hidden field, kept by every other
// submit (search, sort).
func TestLayoutControlOnBothTabs(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	for _, path := range []string{"/movies", "/tv"} {
		body := get(t, h, path).Body.String()
		form := body[strings.Index(body, `<form id="list-controls"`):]
		form = form[:strings.Index(form, "</form>")]
		assertContains(t, form,
			`<div id="layout-control" class="layout-control">`,
			`<input type="hidden" name="layout" value="list">`,
			`<div class="layout-control__segments" role="group" aria-label="Layout">`,
			`<button type="submit" id="layout-list" class="layout-control__segment" name="set-layout" value="list" aria-pressed="true">`,
			`<button type="submit" id="layout-grid" class="layout-control__segment" name="set-layout" value="grid" aria-pressed="false">`,
			`List</button>`, `Grid</button>`,
		)
		grid := get(t, h, path+"?layout=grid").Body.String()
		assertContains(t, grid,
			`<input type="hidden" name="layout" value="grid">`,
			`id="layout-list" class="layout-control__segment" name="set-layout" value="list" aria-pressed="false">`,
			`id="layout-grid" class="layout-control__segment" name="set-layout" value="grid" aria-pressed="true">`,
		)
	}
}

// The layout lives in the URL beside the search and sort: the tidy URL has
// layout=grid, and list (the default) is left out.
func TestLayoutIsKeptInTheURL(t *testing.T) {
	h := newTestHandler(t, searchMovies)
	for _, tc := range []struct{ path, want string }{
		{"/movies?q=&sort=title&dir=asc&layout=list&set-layout=grid", "/movies?layout=grid"},
		{"/movies?q=blade&sort=size&dir=desc&layout=grid&set-layout=list", "/movies?dir=desc&q=blade&sort=size"},
		// Search and sort keep the layout.
		{"/movies?q=blade&sort=title&dir=asc&layout=grid&set-sort=year", "/movies?layout=grid&q=blade&sort=year"},
		{"/movies?q=blade&sort=title&dir=asc&layout=grid&clear=1", "/movies?layout=grid"},
		{"/tv?seasons=all&q=simp&sort=title&dir=asc&layout=list&set-layout=grid", "/tv?layout=grid&q=simp&seasons=all"},
		{"/movies?layout=list", "/movies"},
		{"/movies?layout=bogus", "/movies"},
	} {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.want {
			t.Errorf("%s: %d to %q, want 303 to %q", tc.path, rec.Code, rec.Header().Get("Location"), tc.want)
		}
	}
	for _, path := range []string{"/movies?layout=grid", "/movies?dir=desc&layout=grid&q=blade&sort=size", "/tv?layout=grid&seasons=all"} {
		if rec := get(t, h, path); rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}

	// With htmx the form swaps the results in place, along with the layout
	// control, and puts the tidy URL in the address bar.
	body := get(t, h, "/movies").Body.String()
	assertContains(t, body, `hx-select-oob="#page-summary,#sort-control,#layout-control"`)
	assertContains(t, get(t, h, "/tv").Body.String(), `hx-select-oob="#page-summary,#sort-control,#layout-control,#tv-expand-all"`)
	rec := htmxGet(t, h, "/movies?q=blade&sort=title&dir=asc&layout=list&set-layout=grid")
	if got := rec.Header().Get("HX-Replace-Url"); got != "/movies?layout=grid&q=blade" {
		t.Errorf("HX-Replace-Url = %q", got)
	}
	frag := rec.Body.String()
	assertContains(t, frag, `<ul class="movie-list movie-list--grid">`, `aria-pressed="true">`+gridIconPrefix)
	if got := strings.Join(titles(frag), "|"); got != "Blade Runner|Blade Runner 2049" {
		t.Errorf("fragment rows: %s", got)
	}

	// "Expand all" keeps the layout, and the page reloading itself after a
	// refresh (#app, from the current URL) gets the grid again.
	assertContains(t, get(t, h, "/tv?layout=grid").Body.String(), `hx-get="/tv?layout=grid&amp;seasons=all"`)
	reload := do(t, h, "GET", "/movies?layout=grid", map[string]string{"HX-Request": "true", "HX-Target": "app"}).Body.String()
	assertContains(t, reload, `<div id="app"`, `<ul class="movie-list movie-list--grid">`)
}

// gridIconPrefix is how the Grid button's content starts: its icon.
const gridIconPrefix = `<svg class="layout-control__icon"`

var gridMovies = stubSnapshots{ok: true, snap: library.Snapshot{
	Users: users("Sam", "Priya"),
	Movies: []library.Movie{
		{ID: "a1", Title: "Dr. Strangelove or: How I Learned to Stop Worrying & Love the Bomb", Year: 1964,
			PosterTag: "t1", Size: 21 * gib, CompletedBy: users("Sam")},
		{ID: "b2", Title: "Columbus", Year: 2017},
	},
}}

// The grid shows each movie as a card with the same facts as its row:
// poster (a larger image), title, year, date added, size and who completed
// it. There are no column labels.
func TestMovieGridShowsTheSameFacts(t *testing.T) {
	h := newTestHandler(t, gridMovies)
	list := get(t, h, "/movies").Body.String()
	grid := get(t, h, "/movies?layout=grid").Body.String()
	assertContains(t, grid,
		`<ul class="movie-list movie-list--grid">`,
		`<li class="movie-row movie-row--card">`,
		`<div class="poster poster--large"><img class="poster__image" src="/posters/a1/t1?size=grid" alt="Dr. Strangelove or: How I Learned to Stop Worrying &amp; Love the Bomb" width="160" height="240" loading="lazy" decoding="async"></div>`,
		`<div class="poster poster--large poster--missing" aria-hidden="true">`,
	)
	assertNotContains(t, grid, "list-head", `<li class="movie-row">`)
	assertContains(t, list, "list-head", `<ul class="movie-list">`, `<li class="movie-row">`)

	// Every fact in the list is in the grid, unchanged.
	facts := regexp.MustCompile(`<h2 class="movie-row__title">[^<]*</h2>|<span class="movie-row__(?:year|added)">[^<]*</span>|<span class="size size--unknown">Unknown</span>|<span class="size">.*?</span></span>|<span class="user-tag__name">[^<]*</span>|Not completed by anyone`)
	if l, g := facts.FindAllString(list, -1), facts.FindAllString(grid, -1); !reflect.DeepEqual(l, g) || len(l) < 8 {
		t.Errorf("facts differ:\nlist %q\ngrid %q", l, g)
	}
}

// In the grid, series start closed: each card has the same disclosure as
// its row ("Show 2 seasons"), which swaps in the open card. An open card
// lists its seasons as cards with their posters (the season's own, else the
// series').
func TestTVGridReachesSeasonsThroughTheDisclosure(t *testing.T) {
	snap := tvSnap
	snap.snap.Series = append([]library.Series(nil), tvSnap.snap.Series...)
	andor := snap.snap.Series[0]
	andor.Seasons = append([]library.Season(nil), andor.Seasons...)
	andor.Seasons[0].PosterID, andor.Seasons[0].PosterTag = "andor-1", "s1"
	andor.Seasons[1].PosterID, andor.Seasons[1].PosterTag = "andor", "pt"
	snap.snap.Series[0] = andor
	h := newTestHandler(t, snap)

	grid := get(t, h, "/tv?layout=grid").Body.String()
	assertContains(t, grid,
		`<ul class="series-list series-list--grid">`,
		`<li class="series-row series-row--card" id="series-andor">`,
		`<img class="poster__image" src="/posters/andor/pt?size=grid" alt="Andor" width="160" height="240"`,
		`<button type="button" id="series-andor-toggle" class="button button--disclosure" aria-expanded="false" hx-get="/tv/series/andor?layout=grid&amp;seasons=show" hx-target="closest .series-row" hx-swap="outerHTML">`,
		`Show 2 seasons</button>`,
		`<span class="series-row__counts">2 seasons · 212 episodes</span>`,
		`<span class="size__value">89.0</span> <span class="size__unit">GB</span>`,
	)
	assertNotContains(t, grid, "list-head", "season-row", `aria-expanded="true"`)
	// The list is as it was: short series open, no season posters.
	list := get(t, h, "/tv").Body.String()
	assertContains(t, list, `<li class="series-row" id="series-andor">`, `<li class="season-row">`,
		`hx-get="/tv/series/andor?seasons=hide"`)
	assertNotContains(t, list, "/posters/andor-1/", "season-row--card")

	open := get(t, h, "/tv/series/andor?layout=grid&seasons=show").Body.String()
	assertContains(t, open,
		`<li class="series-row series-row--card series-row--open" id="series-andor">`,
		`aria-expanded="true" aria-controls="series-andor-seasons" hx-get="/tv/series/andor?layout=grid&amp;seasons=hide"`,
		`<ul class="series-row__seasons" id="series-andor-seasons">`,
		`<li class="season-row season-row--card">`,
		`<img class="poster__image" src="/posters/andor-1/s1?size=grid" alt="Andor, Season 1" width="160" height="240"`,
		`<img class="poster__image" src="/posters/andor/pt?size=grid" alt="Andor, Season 2"`,
		`<h3 class="season-row__name">Season 1</h3>`, `<span class="season-row__episodes">12 episodes</span>`,
		`<span class="user-tag__count" aria-hidden="true">9 / 12</span>`,
		`<span class="user-tag__count" aria-hidden="true">1 / 200</span>`,
	)
	closed := get(t, h, "/tv/series/andor?layout=grid&seasons=hide").Body.String()
	assertContains(t, closed, `<li class="series-row series-row--card" id="series-andor">`, `aria-expanded="false"`)

	// "Expand all" opens every card; Twin Peaks' season with no poster at
	// all shows the missing box.
	all := get(t, h, "/tv?layout=grid&seasons=all").Body.String()
	if n := strings.Count(all, "series-row--open"); n != 4 {
		t.Errorf("seasons=all: %d open cards, want 4", n)
	}
	assertContains(t, all, `<div class="poster poster--large poster--missing" aria-hidden="true">`, "Season 35</h3>")
}

// The grid's posters are fetched at the card's size; nothing else can be
// asked for.
func TestPosterRouteServesGridSize(t *testing.T) {
	p := &stubPosters{}
	h := newTestHandlerWithPosters(t, posterSnap, p)
	if rec := get(t, h, "/posters/a1/t1?size=grid"); rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("grid size: status %d Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	for _, path := range []string{"/posters/a1/t1?size=huge", "/posters/a1/t1?size=320x480", "/posters/a1/t1?size=grid&size=list"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}
	if want := []string{"a1/t1 320x480"}; !reflect.DeepEqual(p.calls, want) {
		t.Errorf("Poster calls = %v, want %v", p.calls, want)
	}
}
