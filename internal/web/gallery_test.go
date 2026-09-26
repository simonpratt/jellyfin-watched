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
</style>
</head>
<body>
<div class="gallery">
<section><h2 class="gallery__label">Header · movies current</h2>{{template "app-header" .HeaderMovies}}</section>
<section><h2 class="gallery__label">Header · TV shows current, before first load</h2>{{template "app-header" .HeaderTV}}</section>
<section><h2 class="gallery__label">Poster · image, image that failed or is still loading, missing, generated placeholder</h2><div class="gallery__row">{{range .Posters}}{{template "poster" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Button · default, primary</h2><div class="gallery__row">{{range .Buttons}}{{template "button" .}}{{end}}</div></section>
<section><h2 class="gallery__label">User tag · all 10 users, then user 11 wrapping to hue 1</h2><div class="gallery__row">{{range .Tags}}{{template "user-tag" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Size · movie, 4K remux, series total, unknown, sum with unknowns</h2><div class="gallery__sizes">{{range .Sizes}}<div>{{template "size" .}}</div>{{end}}</div></section>
<section><h2 class="gallery__label">User group · 4 users, 6 users collapsed, all 10 open</h2><div class="gallery__stack">{{range .Groups}}<div>{{template "user-group" .}}</div>{{end}}</div></section>
<section><h2 class="gallery__label">Completion · nobody</h2>{{template "completion" .Nobody}}</section>
<section><h2 class="gallery__label">Movie row</h2><ul class="movie-list">{{range .Rows}}{{template "movie-row" .}}{{end}}</ul></section>
</div>
</body>
</html>
{{end}}`

type gallery struct {
	HeaderMovies, HeaderTV Page
	Posters                []Poster
	Buttons                []Button
	Rows                   []MovieRow
	Tags                   []UserTag
	Sizes                  []Size
	Groups                 []UserGroup
	Nobody                 Completion
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
		HeaderMovies: newPage("Movies", "movies", library.Snapshot{Movies: make([]library.Movie, 1284)}, true),
		HeaderTV:     newPage("TV shows", "tv", library.Snapshot{}, false),
		Buttons:      []Button{{Label: "Clear search"}, {Label: "Open dashboard", Primary: true}},
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
	g.Nobody = newCompletion(nil, tags, len(ten))

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
	for _, want := range []string{"Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", "button--primary", "1,284",
		"Christopher Albrecht", "Unknown", "+ 2 unknown", "Not completed by anyone", `All <span class="user-group__count">10</span> users`,
		`<span class="user-group__count">6</span> users`, "user-code--hue-10",
		`class="poster poster--missing"`, `alt="Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb"`, `loading="lazy"`} {
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
