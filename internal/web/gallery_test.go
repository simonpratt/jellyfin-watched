package web

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"jellyfin-watched/internal/library"
)

// galleryTmpl lays out every component with edge-case fixtures. Stylesheets
// are linked relative to tmp/gallery.html so the file opens straight from disk.
const galleryTmpl = `{{define "gallery"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Component gallery · Spool</title>
<link rel="stylesheet" href="../internal/web/static/css/tokens.css">
<link rel="stylesheet" href="../internal/web/static/css/components.css">
<style>
.gallery { padding: var(--space-8); display: grid; gap: var(--space-10); }
.gallery__label { font: var(--weight-medium) var(--text-xs)/1.4 var(--font-mono); color: var(--text-3); margin: 0 0 var(--space-3); }
.gallery__row { display: flex; flex-wrap: wrap; gap: var(--space-4); align-items: center; }
.gallery__sizes { display: grid; gap: var(--space-3); width: calc(2 * var(--space-12)); text-align: right; }
.gallery__stack { display: grid; gap: var(--space-4); justify-items: start; }
.gallery__wide { justify-items: stretch; }
.gallery__wide .notice { margin: 0; }
.gallery__refresh { position: relative; overflow-x: clip; padding-bottom: var(--space-2); }
.gallery__tools { justify-content: flex-start; }
.gallery__password { justify-items: stretch; grid-template-columns: repeat(auto-fill, minmax(min(100%, calc(9 * var(--space-10))), 1fr)); gap: var(--space-10); align-items: start; }
.gallery__panel { border: 1px solid var(--line); border-radius: var(--radius-lg); margin-bottom: var(--space-4); }
</style>
</head>
<body>
<div class="gallery">
<section><h2 class="gallery__label">Header · movies current</h2>{{template "app-header" .HeaderMovies}}</section>
<section><h2 class="gallery__label">Header · TV shows current, before first load (hairline, no control)</h2>{{template "app-header" .HeaderTV}}</section>
<section><h2 class="gallery__label">Header · refreshing</h2>{{template "app-header" .HeaderRefreshing}}</section>
<section><h2 class="gallery__label">Refresh control · idle, refreshing, stale (3 hours), failed first load</h2><div class="gallery__stack">{{range .Refresh}}<div class="gallery__refresh">{{template "refresh-control" .}}</div>{{end}}</div></section>
<section><h2 class="gallery__label">Notice · stale data, failed first load</h2><div class="gallery__stack gallery__wide">{{range .Notices}}{{template "notice" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Empty state · first load, empty movies, empty TV shows, no search results</h2>{{range .Empties}}<div class="gallery__panel">{{template "empty-state" .}}</div>{{end}}</section>
<section><h2 class="gallery__label">Search · empty, typed (× clears); sort control · title ascending, size descending</h2><div class="gallery__stack">{{range .Searches}}<form>{{template "search" .}}</form>{{end}}{{range .Sorts}}<form>{{template "sort-control" .}}</form>{{end}}</div></section>
<section><h2 class="gallery__label">List controls · TV, with expand all</h2><div class="page-head__tools gallery__tools">{{template "list-controls" .Controls}}{{template "button" .ExpandAll}}</div></section>
<section><h2 class="gallery__label">Poster · image, image that failed or is still loading, missing, generated placeholder</h2><div class="gallery__row">{{range .Posters}}{{template "poster" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Button · default, primary, disclosure collapsed and expanded, expand all, collapse all</h2><div class="gallery__row">{{range .Buttons}}{{template "button" .}}{{end}}</div></section>
<section><h2 class="gallery__label">User tag · all 10 users, then user 11 wrapping to hue 1</h2><div class="gallery__row">{{range .Tags}}{{template "user-tag" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Size · movie, 4K remux, series total, unknown, sum with unknowns</h2><div class="gallery__sizes">{{range .Sizes}}<div>{{template "size" .}}</div>{{end}}</div></section>
<section><h2 class="gallery__label">User group · 4 users, 6 users collapsed, all 10 open</h2><div class="gallery__stack">{{range .Groups}}<div>{{template "user-group" .}}</div>{{end}}</div></section>
<section><h2 class="gallery__label">User tag · in progress: 9 / 12, 1 / 200, 20-character name, 1,199 / 1,200</h2><div class="gallery__row">{{range .ProgressTags}}{{template "user-tag" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Completion · nobody; completed then in progress; in progress only</h2><div class="gallery__stack">{{template "completion" .Nobody}}{{range .Completions}}{{template "completion" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Movie row</h2><ul class="movie-list">{{range .Rows}}{{template "movie-row" .}}{{end}}</ul></section>
<section><h2 class="gallery__label">Password form · empty, wrong password, too many attempts</h2><div class="gallery__stack gallery__password">{{range .PasswordForms}}{{template "password-form" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Series row · expanded (no roll-up), collapsed rolled up to all 10 users, collapsed with completed and in progress, collapsed with 35 seasons and nobody, expanded with unknown sizes and nobody, missing poster with no seasons</h2><ul class="series-list">{{range .SeriesRows}}{{template "series-row" .}}{{end}}</ul></section>
</div>
</body>
</html>
{{end}}`

type gallery struct {
	HeaderMovies, HeaderTV Page
	HeaderRefreshing       Page
	Refresh                []RefreshControl
	Notices                []*Notice
	Empties                []*EmptyState
	Posters                []Poster
	Buttons                []Button
	Rows                   []MovieRow
	Tags                   []UserTag
	Sizes                  []Size
	Groups                 []UserGroup
	Nobody                 Completion
	ProgressTags           []UserTag
	Completions            []Completion
	SeriesRows             []SeriesRow
	Searches               []Search
	Sorts                  []SortControl
	Controls               *ListControls
	ExpandAll              Button
	PasswordForms          []PasswordForm
}

// TestGallery renders every component with edge-case fixtures and writes
// tmp/gallery.html for screenshots. It fails if any template errors.
func TestGallery(t *testing.T) {
	base, err := parseBase()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := base.New("gallery").Parse(galleryTmpl)
	if err != nil {
		t.Fatal(err)
	}
	const gib = 1 << 30
	var users []library.User
	for i, name := range []string{"Sam", "Priya", "Jonas", "Mei", "Alex", "Theo", "Rosa", "Kwame", "Lin",
		"Anastasia Kowalczyk", "Christopher Albrecht"} {
		users = append(users, library.User{ID: strconv.Itoa(i), Name: name})
	}
	ten := users[:10]
	tags := newUserTags(ten)
	tags["10"] = newUserTags(users)["10"]

	added := time.Date(2025, 3, 2, 12, 0, 0, 0, time.UTC)
	movies := []library.Movie{
		{Title: "2001: A Space Odyssey", Year: 1968, Added: time.Date(2024, 2, 14, 0, 0, 0, 0, time.UTC), Size: 582 * gib / 10, CompletedBy: []library.User{ten[0], ten[2]}},
		{Title: "Alien", Year: 1979, Added: added, Size: 324 * gib / 10, CompletedBy: ten},
		{Title: "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", Year: 1964, Added: added, Size: 213 * gib / 10, CompletedBy: []library.User{ten[9], ten[0]}},
		{Title: "Everything Everywhere All at Once", Year: 2022, Added: added, Size: 12 * gib, CompletedBy: []library.User{ten[1], ten[8], ten[7], ten[4], ten[5], ten[2]}},
		{Title: "The Simpsons Movie", Year: 2007, Added: added, Size: 798 * gib / 10, CompletedBy: []library.User{ten[5], ten[3], ten[7], ten[8]}},
		{Title: "No year, no added date, unknown size"},
		{Title: "Супер фильм", Year: 2020, Added: added, Size: 44 * gib / 10, CompletedBy: []library.User{ten[3]}},
	}
	g := gallery{
		HeaderMovies:     newPage("Movies", "movies", galleryStatus(12*time.Minute, false, false), galleryNow, library.View{}),
		HeaderTV:         newPage("TV shows", "tv", library.Status{Refreshing: true}, galleryNow, library.View{}),
		HeaderRefreshing: newPage("Movies", "movies", galleryStatus(12*time.Minute, true, false), galleryNow, library.View{}),
		Refresh: []RefreshControl{
			newRefreshControl(galleryStatus(12*time.Minute, false, false), galleryNow),
			newRefreshControl(galleryStatus(12*time.Minute, true, false), galleryNow),
			newRefreshControl(galleryStatus(3*time.Hour, false, true), galleryNow),
			newRefreshControl(library.Status{Failed: true}, galleryNow),
		},
		Notices: []*Notice{
			newNotice(galleryStatus(3*time.Hour, false, true), galleryNow),
			newNotice(library.Status{Failed: true}, galleryNow),
		},
		Empties: []*EmptyState{
			firstLoad(library.Status{}),
			emptyLibrary(galleryStatus(12*time.Minute, false, false), galleryNow, "movies", "movies", "Movies"),
			emptyLibrary(galleryStatus(12*time.Minute, false, false), galleryNow, "TV shows", "series", "Shows"),
			noMatches("movies", "Dr. Strangelove or: How I Learned"),
		},
		Buttons: []Button{{Label: "Clear search"}, {Label: "Open dashboard", Primary: true}},
	}
	// Every movie but one has a poster; the rows point at generated stand-in
	// images written next to the gallery, not at /posters/.
	posterDir := filepath.Join("..", "..", "tmp", "gallery-posters")
	if err := os.MkdirAll(posterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := 5 // "No year, no added date, unknown size"
	for i, m := range movies {
		m.ID = "m" + strconv.Itoa(i)
		if i != missing {
			m.PosterTag = "t"
		}
		row := newMovieRow(m, tags, len(ten))
		if !strings.HasPrefix(row.Poster.Src, "/posters/"+m.ID+"/t") && i != missing {
			t.Errorf("%s: poster Src = %q", m.Title, row.Poster.Src)
		}
		if row.Poster.Src != "" {
			name := m.ID + ".png"
			writeStandInPoster(t, filepath.Join(posterDir, name), i)
			row.Poster.Src = "gallery-posters/" + name
		}
		g.Rows = append(g.Rows, row)
	}
	g.Posters = []Poster{
		g.Rows[0].Poster,
		{Title: "Never loads", Src: "gallery-posters/does-not-exist.png"},
		g.Rows[missing].Poster,
		{Title: "Alien", Initial: posterInitial("Alien")},
	}
	for _, u := range users {
		g.Tags = append(g.Tags, tags[u.ID])
	}
	g.Sizes = []Size{newSize(146 * gib / 10), newSize(798 * gib / 10), newSize(194 * (1 << 40) / 100), newSize(0),
		{Value: "38.1", Unit: "GB", UnknownCount: 2}}
	g.Groups = []UserGroup{
		newUserGroup(ten[:4], tags, len(ten)),
		newUserGroup(ten[4:], tags, len(ten)),
		newUserGroup(ten, tags, len(ten)),
	}
	g.Nobody = newCompletion(nil, nil, tags, len(ten))

	// TV: in-progress tags, completions, buttons and series rows.
	longName := library.User{ID: "long", Name: "Maximiliana Oyelaran"} // 20 characters
	tags["long"] = UserTag{Name: longName.Name, Code: userCode(longName.Name), Hue: 3}
	progress := func(u library.User, played, available int) library.Progress {
		return library.Progress{User: u, Played: played, Available: available}
	}
	for _, p := range []library.Progress{progress(ten[1], 9, 12), progress(ten[7], 1, 200), progress(longName, 1, 12), progress(ten[3], 1199, 1200)} {
		g.ProgressTags = append(g.ProgressTags, newProgressTag(tags[p.User.ID], p))
	}
	g.Completions = []Completion{
		newCompletion([]library.User{ten[0]}, []library.Progress{progress(ten[2], 3, 12), progress(ten[9], 1, 12)}, tags, len(ten)),
		newCompletion(nil, []library.Progress{progress(ten[3], 97, 148)}, tags, len(ten)),
	}
	g.Buttons = append(g.Buttons,
		Button{Label: "Show 35 seasons", Disclosure: true, Expanded: "false"},
		Button{Label: "Hide seasons", Disclosure: true, Expanded: "true"},
		Button{Label: "Expand all", Icon: "expand-all"},
		Button{Label: "Collapse all", Icon: "collapse-all"})
	seasons35 := make([]library.Season, 35)
	for i := range seasons35 {
		seasons35[i] = library.Season{ID: "simp" + strconv.Itoa(i), Name: "Season " + strconv.Itoa(i+1), Episodes: 22, Size: 57 * gib}
	}
	series := []library.Series{
		{ID: "hxh", Title: "Hunter × Hunter", Year: 2011, PosterTag: "t", Size: 1426 * gib / 10, Episodes: 148,
			CompletedBy: []library.User{ten[8]}, InProgress: []library.Progress{progress(ten[3], 97, 148), progress(ten[7], 1, 148)}, Seasons: []library.Season{
				{Name: "Season 1", Episodes: 148, Size: 1426 * gib / 10, CompletedBy: []library.User{ten[8]},
					InProgress: []library.Progress{progress(ten[3], 97, 148), progress(ten[7], 1, 148)}}}},
		{ID: "flea", Title: "Fleabag", Year: 2016, PosterTag: "t", Size: 118 * gib / 10, Episodes: 12, CompletedBy: ten, Seasons: []library.Season{
			{Name: "Season 1", Episodes: 6, Size: 59 * gib / 10, CompletedBy: ten},
			{Name: "Season 2", Episodes: 6, Size: 59 * gib / 10, CompletedBy: ten}}},
		{ID: "who", Title: "Doctor Who", Year: 2005, PosterTag: "t", Size: 3100 * gib / 10, Episodes: 200,
			CompletedBy: []library.User{ten[0]}, InProgress: []library.Progress{progress(longName, 150, 200), progress(ten[7], 1, 200)},
			Seasons: []library.Season{
				{Name: "Season 1", Episodes: 100, Size: 1550 * gib / 10, CompletedBy: []library.User{ten[0], longName}, InProgress: []library.Progress{progress(ten[7], 1, 100)}},
				{Name: "Season 2", Episodes: 100, Size: 1550 * gib / 10, CompletedBy: []library.User{ten[0]}, InProgress: []library.Progress{progress(longName, 50, 100)}}}},
		{ID: "simpsons", Title: "The Simpsons", Year: 1989, PosterTag: "t", Size: 1995 * gib, Episodes: 770, Seasons: seasons35},
		{ID: "twin", Title: "Twin Peaks", Year: 1990, PosterTag: "t", Size: 381 * gib / 10, UnknownSizes: 26, Episodes: 48, Seasons: []library.Season{
			{Name: "Season 1", Episodes: 8, UnknownSizes: 8},
			{Name: "Season 2", Episodes: 22, Size: 381 * gib / 10},
			{Name: "Season 3", Episodes: 18, UnknownSizes: 18}}},
		{ID: "none", Title: "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", Year: 1964},
	}
	// Hunter × Hunter and Twin Peaks are expanded; the rest are collapsed
	// and show their roll-up.
	shown := expansion{except: []string{"hxh", "twin"}}
	for i, sr := range series {
		row := newSeriesRow(sr, tags, len(ten), library.View{}, shown)
		if row.Poster.Src != "" {
			name := "s" + strconv.Itoa(i) + ".png"
			writeStandInPoster(t, filepath.Join(posterDir, name), i+3)
			row.Poster.Src = "gallery-posters/" + name
		}
		g.SeriesRows = append(g.SeriesRows, row)
	}

	blade := library.View{Search: "blade"}
	sizeDesc := library.View{Sort: library.SortSize, Desc: true}
	g.Searches = []Search{newListControls("/movies", library.View{}, "").Search, newListControls("/movies", blade, "").Search}
	g.Sorts = []SortControl{newListControls("/movies", library.View{}, "").Sort, newListControls("/movies", sizeDesc, "").Sort}
	// The gallery has one list-controls form, so the others' IDs aren't
	// repeated in it.
	g.Controls = newListControls("/tv", library.View{Search: "hunter", Sort: library.SortYear}, "")
	g.ExpandAll = Button{Label: "Expand all", Icon: "expand-all"}

	for _, msg := range []string{"", wrongPassword, tooManyPassword} {
		g.PasswordForms = append(g.PasswordForms, newPasswordPage("/tv", msg).Form)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "gallery", g); err != nil {
		t.Fatalf("rendering gallery: %v", err)
	}
	// Show the "all 10" group and the 6-user movie row open, so the
	// screenshot has both states (Components sheet shows both).
	parts := strings.Split(buf.String(), `<details class="user-group">`)
	for i := range parts[1:] {
		if i == 1 || i == 3 {
			parts[i+1] = " open>" + parts[i+1]
		} else {
			parts[i+1] = ">" + parts[i+1]
		}
	}
	out := strings.Join(parts, `<details class="user-group"`)
	if strings.Count(out, `<details class="user-group" open>`) != 2 {
		t.Errorf("gallery: could not open two groups")
	}
	for _, want := range []string{"Updated 12 minutes ago", "Refreshing…", "Updated 3 hours ago", "refresh-control__hairline",
		"Showing data from 3 hours ago.", "Last updated today at 11:42.", "Loading your library.", "There are no TV shows on this server",
		"Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", "button--primary", "1,284",
		"Christopher Albrecht", "Unknown", "+ 2 unknown", "Not completed by anyone", `All <span class="user-group__count">10</span> users`,
		`<span class="user-group__count">6</span> users`, "user-code--hue-10",
		`class="poster poster--missing"`, `alt="Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb"`, `loading="lazy"`,
		"Maximiliana Oyelaran", `aria-hidden="true">1 / 200</span>`, `aria-hidden="true">150 / 200</span>`, `<div class="series-row__completion">`, `aria-hidden="true">1,199 / 1,200</span>`, "Show 35 seasons", "Hide seasons",
		"Expand all", "Collapse all", `<h2 class="series-row__title">Hunter × Hunter</h2>`, `<span class="size__note">+ 26 unknown</span>`,
		`<span class="series-row__counts">0 seasons · 0 episodes</span>`, "No movies match “Dr. Strangelove or: How I Learned”",
		"Clear search</button>", `placeholder="Search titles"`, `value="blade"`, "Date added</button>", "Descending</button>", "Ascending</button>",
		`aria-pressed="true">Size</button>`, `aria-pressed="true">Year</button>`,
		"That password is not correct. Check it and try again.", "Too many wrong passwords.", "Open dashboard</button>"} {
		if !strings.Contains(out, want) {
			t.Errorf("gallery missing %q", want)
		}
	}

	path := filepath.Join("..", "..", "tmp", "gallery.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

// galleryNow is the gallery's clock: 14:42, so "3 hours ago" was 11:42.
var galleryNow = time.Date(2026, 9, 26, 14, 42, 0, 0, time.UTC)

// galleryStatus is a loaded library with 1,284 movies and 212 series,
// last updated ago before galleryNow.
func galleryStatus(ago time.Duration, refreshing, failed bool) library.Status {
	return library.Status{Loaded: true, Generation: 1, Refreshing: refreshing, Failed: failed,
		Snapshot: library.Snapshot{Movies: make([]library.Movie, 1284), Series: make([]library.Series, 212), LoadedAt: galleryNow.Add(-ago)}}
}

// writeStandInPoster writes a 96×144 PNG with a vertical gradient, standing
// in for a Jellyfin poster in the gallery.
func writeStandInPoster(t *testing.T, path string, n int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 96, 144))
	r, g, b := uint8(40+n*30%120), uint8(60+n*50%100), uint8(90+n*70%130)
	for y := 0; y < 144; y++ {
		for x := 0; x < 96; x++ {
			k := 144 - y/2
			img.Set(x, y, color.RGBA{uint8(int(r) * k / 144), uint8(int(g) * k / 144), uint8(int(b) * k / 144), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
