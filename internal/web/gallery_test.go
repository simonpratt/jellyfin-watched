package web

import (
	"bytes"
	"os"
	"path/filepath"
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
</style>
</head>
<body>
<div class="gallery">
<section><h2 class="gallery__label">Header · movies current</h2>{{template "app-header" .HeaderMovies}}</section>
<section><h2 class="gallery__label">Header · TV shows current, before first load</h2>{{template "app-header" .HeaderTV}}</section>
<section><h2 class="gallery__label">Poster · placeholder</h2><div class="gallery__row">{{range .Posters}}{{template "poster" .}}{{end}}</div></section>
<section><h2 class="gallery__label">Button · default, primary</h2><div class="gallery__row">{{range .Buttons}}{{template "button" .}}{{end}}</div></section>
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
	added := time.Date(2025, 3, 2, 12, 0, 0, 0, time.UTC)
	movies := []library.Movie{
		{Title: "2001: A Space Odyssey", Year: 1968, Added: time.Date(2024, 2, 14, 0, 0, 0, 0, time.UTC)},
		{Title: "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", Year: 1964, Added: added},
		{Title: "The Simpsons Movie", Year: 2007, Added: added},
		{Title: "No year, no added date"},
		{Title: "Супер фильм", Year: 2020, Added: added},
	}
	g := gallery{
		HeaderMovies: newPage("Movies", "movies", library.Snapshot{Movies: make([]library.Movie, 1284)}, true),
		HeaderTV:     newPage("TV shows", "tv", library.Snapshot{}, false),
		Buttons:      []Button{{Label: "Clear search"}, {Label: "Open dashboard", Primary: true}},
	}
	for _, m := range movies {
		row := newMovieRow(m)
		g.Rows = append(g.Rows, row)
		g.Posters = append(g.Posters, row.Poster)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "gallery", g); err != nil {
		t.Fatalf("rendering gallery: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", "button--primary", "1,284"} {
		if !strings.Contains(out, want) {
			t.Errorf("gallery missing %q", want)
		}
	}

	path := filepath.Join("..", "..", "tmp", "gallery.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
