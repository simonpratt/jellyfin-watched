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
		{ID: "andor", Title: "Andor", Year: 2022, PosterTag: "pt", Size: 89 * gib, Seasons: []library.Season{
			{ID: "andor-1", Name: "Season 1", Episodes: 12, Size: 44 * gib,
				CompletedBy: []library.User{tvSally},
				InProgress:  []library.Progress{{User: tvAK, Played: 9, Available: 12}}},
			{ID: "andor-2", Name: "Season 2", Episodes: 200, Size: 45 * gib,
				InProgress: []library.Progress{{User: tvH, Played: 1, Available: 200}}},
		}},
		{ID: "fleabag", Title: "Fleabag", Year: 2016, Size: 2 * gib, Seasons: []library.Season{
			{ID: "flea-1", Name: "Season 1", Episodes: 6, Size: 2 * gib, CompletedBy: []library.User{tvSally, tvAK, tvH}},
		}},
		{ID: "simpsons", Title: "The Simpsons", Year: 1989, Size: 350 * gib, Seasons: manySeasons(35)},
		{ID: "twin", Title: "Twin Peaks", Year: 1990, Size: 38 * gib, UnknownSizes: 2, Seasons: []library.Season{
			{ID: "twin-1", Name: "Season 1", Episodes: 8, UnknownSizes: 8},
			{ID: "twin-2", Name: "Season 2", Episodes: 22, Size: 38 * gib},
		}},
		{ID: "empty", Title: "Nothing On Disk"},
	},
}}

func TestTVPageListsSeriesAndSeasons(t *testing.T) {
	rec := get(t, newTestHandler(t, tvSnap), "/tv")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="app-header__tab" href="/tv" aria-current="page">TV shows <span class="app-header__count">5</span></a>`,
		// 2 + 1 + 35 + 2 seasons; 89 + 2 + 350 + 38 GiB.
		`<p class="page-head__summary">5 series · 40 seasons · 479.0 GB on disk</p>`,
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
	// Twin Peaks' seasons nobody has watched say so; Andor's Season 2 has
	// someone in progress, so it doesn't.
	if n := strings.Count(body, "Not completed by anyone"); n != 2 {
		t.Errorf(`"Not completed by anyone" appears %d times, want 2`, n)
	}
	if strings.Contains(body, "0 / ") {
		t.Error("a user who hasn't started is listed")
	}
}

// Series with few seasons show them; long-running ones start collapsed
// behind "Show 35 seasons". Each disclosure is a keyboard-operable button
// that swaps in the same series-row component with its seasons.
func TestTVSeriesDisclosure(t *testing.T) {
	body := get(t, newTestHandler(t, tvSnap), "/tv").Body.String()
	for _, want := range []string{
		`<button type="button" id="series-andor-toggle" class="button button--disclosure" aria-expanded="true" aria-controls="series-andor-seasons" hx-get="/tv/series/andor?seasons=hide" hx-target="closest .series-row" hx-swap="outerHTML">`,
		`<ul class="series-row__seasons" id="series-andor-seasons">`,
		`Hide seasons</button>`,
		`<button type="button" id="series-simpsons-toggle" class="button button--disclosure" aria-expanded="false" hx-get="/tv/series/simpsons?seasons=show" hx-target="closest .series-row" hx-swap="outerHTML">`,
		`Show 35 seasons</button>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if strings.Contains(body, "simpsons-seasons") || strings.Contains(body, "Season 35") {
		t.Error("The Simpsons' seasons are shown by default")
	}
	// A series with no seasons has nothing to disclose.
	if strings.Contains(body, "series-empty-toggle") {
		t.Error("series with no seasons has a disclosure")
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
	for _, want := range []string{`aria-expanded="true"`, `Hide seasons`, `Season 35`, `hx-get="/tv/series/simpsons?seasons=hide"`} {
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

func TestTVExpandAll(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	body := get(t, h, "/tv").Body.String()
	want := `<button type="button" id="tv-expand-all" class="button" hx-get="/tv?seasons=all" hx-target="main" hx-swap="innerHTML" hx-push-url="true">`
	if !strings.Contains(body, want) || !strings.Contains(body, `Expand all</button>`) {
		t.Errorf("page missing expand all button")
	}

	all := get(t, h, "/tv?seasons=all").Body.String()
	if n := strings.Count(all, `aria-expanded="true"`); n != 4 {
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

// An htmx request gets the page's content alone, to swap into <main>; a
// normal load or an htmx history restore gets the whole page.
func TestTVContentFragmentForHtmx(t *testing.T) {
	h := newTestHandler(t, tvSnap)
	req := httptest.NewRequest(http.MethodGet, "/tv?seasons=all", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "app-header") || !strings.Contains(body, `<h1 class="page-head__title">TV shows</h1>`) {
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
