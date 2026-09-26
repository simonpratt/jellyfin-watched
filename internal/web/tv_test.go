package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"jellyfin-watched/internal/library"
)

const gib = 1 << 30

var (
	tvSally = library.User{ID: "s", Name: "sally"}
	tvAK    = library.User{ID: "a", Name: "Anastasia Kowalczyk"}
	tvH     = library.User{ID: "h", Name: "H"}
)

func manySeasons(n int) []library.Season {
	out := make([]library.Season, n)
	for i := range out {
		out[i] = library.Season{ID: "simp-" + strconv.Itoa(i+1), Name: "Season " + strconv.Itoa(i+1), Episodes: 22, Size: 10 * gib}
	}
	return out
}

var tvSnap = stubSnapshots{ok: true, snap: library.Snapshot{
	Users: []library.User{tvSally, tvAK, tvH},
	Series: []library.Series{
		// sally completed Season 1 only; AK is partway through it; H has
		// started Season 2.
		{ID: "andor", Title: "Andor", Year: 2022, PosterTag: "pt", Size: 89 * gib, Episodes: 212,
			InProgress: []library.Progress{{User: tvSally, Played: 12, Available: 212}, {User: tvAK, Played: 9, Available: 212}, {User: tvH, Played: 1, Available: 212}},
			Seasons: []library.Season{
				{ID: "andor-1", Name: "Season 1", Episodes: 12, Size: 44 * gib,
					CompletedBy: []library.User{tvSally},
					InProgress:  []library.Progress{{User: tvAK, Played: 9, Available: 12}}},
				{ID: "andor-2", Name: "Season 2", Episodes: 200, Size: 45 * gib,
					InProgress: []library.Progress{{User: tvH, Played: 1, Available: 200}}},
			}},
		{ID: "fleabag", Title: "Fleabag", Year: 2016, Size: 2 * gib, Episodes: 6, CompletedBy: []library.User{tvSally, tvAK, tvH}, Seasons: []library.Season{
			{ID: "flea-1", Name: "Season 1", Episodes: 6, Size: 2 * gib, CompletedBy: []library.User{tvSally, tvAK, tvH}},
		}},
		{ID: "simpsons", Title: "The Simpsons", Year: 1989, Size: 350 * gib, Episodes: 35 * 22, Seasons: manySeasons(35)},
		{ID: "twin", Title: "Twin Peaks", Year: 1990, Size: 38 * gib, UnknownSizes: 2, Episodes: 30, Seasons: []library.Season{
			{ID: "twin-1", Name: "Season 1", Episodes: 8, UnknownSizes: 8},
			{ID: "twin-2", Name: "Season 2", Episodes: 22, Size: 38 * gib},
		}},
		{ID: "empty", Title: "Nothing On Disk"},
	},
}}

func TestTVPageListsSeriesAndSeasons(t *testing.T) {
	rec := get(t, newTestHandler(t, tvSnap), "/tv?seasons=all")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="app-header__tab" href="/tv" aria-current="page">TV shows <span class="app-header__count">5</span></a>`,
		// 2 + 1 + 35 + 2 seasons; 89 + 2 + 350 + 38 GiB.
		`<p id="page-summary" class="page-head__summary">5 series · 40 seasons · 479.0 GB on disk</p>`,
		`<span class="list-head__completion">Completed by · In progress</span>`,
		`<h2 class="series-row__title">Andor</h2>`,
		`<span class="series-row__year">2022</span>`,
		`<span class="series-row__counts">2 seasons · 212 episodes</span>`,
		`<span class="size__value">89.0</span> <span class="size__unit">GB</span>`,
		`<img class="poster__image" src="/posters/andor/pt" alt="Andor"`,
		`<h3 class="season-row__name">Season 1</h3>`,
		`<span class="season-row__episodes">12 episodes</span>`,
		`<span class="size__value">44.0</span> <span class="size__unit">GB</span>`,
		// In progress: the user's tag with "x / y" and a bar.
		`<span class="user-tag user-tag--progress">`,
		`<span class="user-tag__count" aria-hidden="true">9 / 12</span><span class="user-tag__sr">9 of 12 episodes played</span>`,
		`<span class="user-tag__count" aria-hidden="true">1 / 200</span>`,
		// Everyone completed Fleabag.
		`<span class="user-tag__name">H</span>`,
		// Unknown sizes: a season that's all unknown, and a series sum
		// that leaves them out with a note.
		`<span class="size size--unknown">Unknown</span>`,
		`<span class="size__value">38.0</span> <span class="size__unit">GB</span><span class="size__note">+ 2 unknown</span>`,
		`<span class="series-row__counts">0 seasons · 0 episodes</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	// Twin Peaks' and The Simpsons' seasons nobody has watched say so;
	// Andor's Season 2 has someone in progress, so it doesn't.
	if n := strings.Count(body, "Not completed by anyone"); n != 2+35 {
		t.Errorf(`"Not completed by anyone" appears %d times, want 37`, n)
	}
	if strings.Contains(body, "0 / ") {
		t.Error("a user who hasn't started is listed")
	}
}

// Every series starts collapsed, whatever its season count. Each
// disclosure is a link: with htmx it swaps in the same series-row component
// with its seasons; without JavaScript it loads the page with that series
// shown (?show=), at the series.
func TestTVSeriesDisclosure(t *testing.T) {
	body := get(t, newTestHandler(t, tvSnap), "/tv").Body.String()
	for _, want := range []string{
		`<a href="/tv?show=andor#series-andor" id="series-andor-toggle" class="button button--disclosure" aria-expanded="false" hx-get="/tv/series/andor?seasons=show" hx-target="closest .series-row" hx-swap="outerHTML">`,
		`Show 2 seasons</a>`,
		`<a href="/tv?show=simpsons#series-simpsons" id="series-simpsons-toggle" class="button button--disclosure" aria-expanded="false" hx-get="/tv/series/simpsons?seasons=show" hx-target="closest .series-row" hx-swap="outerHTML">`,
		`Show 35 seasons</a>`,
		`Show 1 season</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if strings.Contains(body, "season-row") || strings.Contains(body, `aria-expanded="true"`) {
		t.Error("a series is expanded by default")
	}
	// A series with no seasons has nothing to disclose.
	if strings.Contains(body, "series-empty-toggle") {
		t.Error("series with no seasons has a disclosure")
	}
}

// completionOf returns the series row's roll-up: its completion cell.
func completionOf(t *testing.T, body, id string) string {
	t.Helper()
	i := strings.Index(body, `<li class="series-row" id="series-`+id+`">`)
	if i < 0 {
		t.Fatalf("no row for %s", id)
	}
	row := body[i:]
	if end := strings.Index(row[1:], `<li class="series-row"`); end >= 0 {
		row = row[:end+1]
	}
	j := strings.Index(row, `<div class="series-row__completion">`)
	if j < 0 {
		return ""
	}
	return row[j:]
}

// A collapsed series row rolls its seasons' completion up: completed users,
// then each user in progress with played / available across the series, or
// "Not completed by anyone". A series with no episodes has no roll-up.
func TestTVCollapsedSeriesShowsRollUp(t *testing.T) {
	body := get(t, newTestHandler(t, tvSnap), "/tv").Body.String()
	andor := completionOf(t, body, "andor")
	for _, want := range []string{
		`<span class="user-tag__count" aria-hidden="true">12 / 212</span><span class="user-tag__sr">12 of 212 episodes played</span>`,
		`<span class="user-tag__count" aria-hidden="true">9 / 212</span>`,
		`<span class="user-tag__count" aria-hidden="true">1 / 212</span>`,
	} {
		if !strings.Contains(andor, want) {
			t.Errorf("Andor roll-up missing %s", want)
		}
	}
	flea := completionOf(t, body, "fleabag")
	if !strings.Contains(flea, `<ul class="user-group__tags">`) || strings.Contains(flea, "user-tag--progress") ||
		strings.Count(flea, `<span class="user-tag__name">`) != 3 {
		t.Errorf("Fleabag roll-up:\n%s", flea)
	}
	for _, id := range []string{"simpsons", "twin"} {
		if !strings.Contains(completionOf(t, body, id), "Not completed by anyone") {
			t.Errorf("%s: no \"Not completed by anyone\"", id)
		}
	}
	if got := completionOf(t, body, "empty"); got != "" {
		t.Errorf("series with no episodes has a roll-up:\n%s", got)
	}
	// The counts are the roll-up's episodes.
	if !strings.Contains(body, `<span class="series-row__counts">35 seasons · 770 episodes</span>`) {
		t.Error("Simpsons counts")
	}
}

// Expanding a series shows its seasons in place of the roll-up.
func TestTVExpandedSeriesHidesRollUp(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	for _, path := range []string{"/tv?seasons=all", "/tv/series/andor?seasons=show"} {
		body := get(t, h, path).Body.String()
		if got := completionOf(t, body, "andor"); got != "" {
			t.Errorf("%s: expanded Andor shows its roll-up", path)
		}
		if !strings.Contains(body, "9 / 12") || strings.Contains(body, "/ 212") {
			t.Errorf("%s: seasons not shown in place of the roll-up", path)
		}
	}
	if got := completionOf(t, get(t, h, "/tv/series/andor?seasons=hide").Body.String(), "andor"); !strings.Contains(got, "9 / 212") {
		t.Errorf("collapsed again: roll-up %q", got)
	}
}

func TestTVSeriesFragment(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	rec := get(t, h, "/tv/series/simpsons?seasons=show")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, `<li class="series-row"`) || strings.Contains(body, "<html") || strings.Contains(body, "app-header") {
		t.Errorf("fragment is not just the series row:\n%s", body)
	}
	for _, want := range []string{`aria-expanded="true"`, `aria-controls="series-simpsons-seasons"`, `Hide seasons`, `Season 35`,
		`hx-get="/tv/series/simpsons?seasons=hide"`, `href="/tv#series-simpsons"`} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment missing %s", want)
		}
	}
	if n := strings.Count(body, `<li class="season-row">`); n != 35 {
		t.Errorf("%d season rows, want 35", n)
	}

	body = get(t, h, "/tv/series/andor?seasons=hide").Body.String()
	if !strings.Contains(body, `aria-expanded="false"`) || !strings.Contains(body, "Show 2 seasons") || strings.Contains(body, "season-row") {
		t.Errorf("hidden fragment:\n%s", body)
	}

	for _, path := range []string{"/tv/series/nope", "/tv/series/"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s status %d", path, rec.Code)
		}
	}
	if rec := get(t, newTestHandler(t, stubSnapshots{}), "/tv/series/andor"); rec.Code != http.StatusNotFound {
		t.Errorf("before first load: status %d", rec.Code)
	}
}

// Which series are expanded is part of the page's URL, so it survives the
// page reloading itself after a refresh, and works without JavaScript:
// ?show= lists the series shown, or under ?seasons=all, ?hide= those
// hidden.
func TestTVExpansionInURL(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	body := get(t, h, "/tv?q=a&show=andor&show=twin").Body.String()
	if n := strings.Count(body, `aria-expanded="true"`); n != 2 || completionOf(t, body, "andor") != "" || completionOf(t, body, "twin") != "" {
		t.Errorf("show=andor,twin: %d expanded", n)
	}
	for _, want := range []string{
		// Hiding one keeps the other shown, and the search.
		`href="/tv?q=a&amp;show=twin#series-andor"`,
		`href="/tv?q=a&amp;show=andor#series-twin"`,
		`href="/tv?q=a&amp;show=andor&amp;show=fleabag&amp;show=twin#series-fleabag"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}

	all := get(t, h, "/tv?seasons=all&hide=andor").Body.String()
	if n := strings.Count(all, `aria-expanded="true"`); n != 3 || !strings.Contains(completionOf(t, all, "andor"), "9 / 212") {
		t.Errorf("seasons=all&hide=andor: %d expanded", n)
	}
	for _, want := range []string{`href="/tv?seasons=all#series-andor"`, `href="/tv?hide=andor&amp;hide=fleabag&amp;seasons=all#series-fleabag"`,
		// Expand all and Collapse all start afresh.
		`hx-get="/tv?seasons=none"`} {
		if !strings.Contains(all, want) {
			t.Errorf("seasons=all page missing %s", want)
		}
	}

	// Untidy lists are tidied: sorted, without repeats, and only the one
	// that applies.
	for path, want := range map[string]string{
		"/tv?show=twin&show=andor&show=twin": "/tv?show=andor&show=twin",
		"/tv?hide=andor":                     "/tv",
		"/tv?seasons=all&show=andor":         "/tv?seasons=all",
		"/tv?show=":                          "/tv",
		"/movies?show=andor":                 "/movies",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want %q", path, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}

// With htmx, a series' disclosure updates the address bar from the page's
// current URL, so the page's self-reload after a refresh, and back and
// forward, keep the series as it was left.
func TestTVSeriesFragmentUpdatesPageURL(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	for _, tc := range []struct{ path, current, want, href string }{
		{"/tv/series/andor?seasons=show", "http://example.com/tv?sort=year&show=twin", "/tv?show=andor&show=twin&sort=year", `href="/tv?show=twin&amp;sort=year#series-andor"`},
		{"/tv/series/twin?seasons=hide", "http://example.com/tv?show=andor&show=twin", "/tv?show=andor", `href="/tv?show=andor&amp;show=twin#series-twin"`},
		{"/tv/series/andor?seasons=hide", "http://example.com/tv?seasons=all", "/tv?hide=andor&seasons=all", `href="/tv?seasons=all#series-andor"`},
		{"/tv/series/andor?seasons=show", "http://example.com/tv?hide=andor&seasons=all", "/tv?seasons=all", `href="/tv?hide=andor&amp;seasons=all#series-andor"`},
	} {
		rec := do(t, h, "GET", tc.path, map[string]string{"HX-Request": "true", "HX-Current-URL": tc.current})
		if got := rec.Header().Get("HX-Replace-Url"); got != tc.want {
			t.Errorf("%s from %s: HX-Replace-Url = %q, want %q", tc.path, tc.current, got, tc.want)
		}
		if !strings.Contains(rec.Body.String(), tc.href) {
			t.Errorf("%s from %s: toggle missing %s", tc.path, tc.current, tc.href)
		}
	}
	// From anywhere but the TV page, the address bar is left alone.
	for _, current := range []string{"", "http://example.com/movies?show=x", "::"} {
		rec := do(t, h, "GET", "/tv/series/andor?seasons=show", map[string]string{"HX-Request": "true", "HX-Current-URL": current})
		if got := rec.Header().Get("HX-Replace-Url"); got != "" {
			t.Errorf("from %q: HX-Replace-Url = %q", current, got)
		}
	}
}

func TestTVExpandAll(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	body := get(t, h, "/tv").Body.String()
	want := `<button type="button" id="tv-expand-all" class="button" hx-get="/tv?seasons=all" hx-target="main" hx-swap="innerHTML" hx-push-url="true">`
	if !strings.Contains(body, want) || !strings.Contains(body, `Expand all</button>`) {
		t.Errorf("page missing expand all button")
	}

	all := get(t, h, "/tv?seasons=all").Body.String()
	if n := strings.Count(all, `aria-expanded="true"`); n != 4 || strings.Contains(all, "series-row__completion") {
		t.Errorf("seasons=all: %d series expanded, want 4", n)
	}
	if !strings.Contains(all, `hx-get="/tv?seasons=none"`) || !strings.Contains(all, "Collapse all</button>") {
		t.Error("seasons=all: no collapse all button")
	}

	none := get(t, h, "/tv?seasons=none").Body.String()
	if strings.Contains(none, `aria-expanded="true"`) || strings.Contains(none, "season-row") {
		t.Error("seasons=none: something is expanded")
	}
}

// An htmx request gets the page's content, to swap into <main>, plus only
// the header's tabs (out of band); a normal load or an htmx history restore gets the whole page.
func TestTVContentFragmentForHtmx(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	req := httptest.NewRequest(http.MethodGet, "/tv?seasons=all", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "<header") || !strings.Contains(body, `<h1 class="page-head__title">TV shows</h1>`) {
		t.Errorf("htmx response is not the content alone:\n%.300s", body)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "HX-Request") {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}

	req.Header.Set("HX-History-Restore-Request", "true")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Error("history restore did not get the whole page")
	}
}

func TestTVPageStates(t *testing.T) {
	if body := get(t, newTestHandler(t, stubSnapshots{}), "/tv").Body.String(); !strings.Contains(body, "Loading your library.") ||
		strings.Contains(body, "tv-expand-all") {
		t.Errorf("before first load:\n%s", body)
	}
	body := get(t, newTestHandler(t, stubSnapshots{ok: true}), "/tv").Body.String()
	if !strings.Contains(body, "There are no TV shows on this server") || strings.Contains(body, "tv-expand-all") {
		t.Errorf("empty library:\n%s", body)
	}
}

// DESIGN.md §3: series and season rows carry the same classes whatever their
// size or completion.
func TestTVRowsAreStyledTheSameWhateverTheirData(t *testing.T) {
	body := get(t, newTestHandler(t, tvSnap), "/tv?seasons=all").Body.String()
	for _, m := range regexp.MustCompile(`<li class="((?:series|season)-row[^"]*)"`).FindAllStringSubmatch(body, -1) {
		if m[1] != "series-row" && m[1] != "season-row" {
			t.Errorf("row styled with %q", m[1])
		}
	}
}
