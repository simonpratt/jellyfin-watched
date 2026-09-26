package web

import "jellyfin-watched/internal/library"

// Page is what the layout needs from every page.
type Page struct {
	Title string
}

// MoviesPage is the view model for pages/movies.tmpl.
type MoviesPage struct {
	Page
	Loaded bool
	Count  string
	Movies []MovieRow
}

// MovieRow is the view model for the movie-row component.
type MovieRow struct {
	Title string
	Year  string // empty when unknown
}

func newMoviesPage(snap library.Snapshot, loaded bool) MoviesPage {
	p := MoviesPage{Page: Page{Title: "Movies"}, Loaded: loaded}
	if !loaded {
		return p
	}
	p.Count = formatCount(len(snap.Movies), "movie", "movies")
	p.Movies = make([]MovieRow, 0, len(snap.Movies))
	for _, m := range snap.Movies {
		p.Movies = append(p.Movies, MovieRow{Title: m.Title, Year: formatYear(m.Year)})
	}
	return p
}
