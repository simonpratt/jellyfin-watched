package web

import "jellyfin-watched/internal/library"

// Page is what the layout needs from every page: the document title and the
// header's tabs.
type Page struct {
	Title string
	Tabs  []Tab
}

// Tab is one of the header's Movies / TV shows tabs.
type Tab struct {
	Label   string
	Href    string
	Count   string // empty until the library has loaded
	Current bool
}

// newPage builds the layout's view model. current is "movies" or "tv".
func newPage(title, current string, snap library.Snapshot, loaded bool) Page {
	movies := Tab{Label: "Movies", Href: "/movies", Current: current == "movies"}
	if loaded {
		movies.Count = formatNumber(len(snap.Movies))
	}
	// The TV shows count arrives with the TV shows list (ticket 06).
	tv := Tab{Label: "TV shows", Href: "/tv", Current: current == "tv"}
	return Page{Title: title, Tabs: []Tab{movies, tv}}
}

// MoviesPage is the view model for pages/movies.tmpl.
type MoviesPage struct {
	Page
	Loaded  bool
	Summary string // e.g. "1,284 movies"
	Movies  []MovieRow
}

// TVPage is the view model for pages/tv.tmpl.
type TVPage struct {
	Page
}

// Poster is the view model for the poster component. Until cover art arrives
// (ticket 05) every poster is the generated placeholder.
type Poster struct {
	Title   string // alt text once the poster is an image
	Initial string // may be empty, leaving a plain block
}

// Button is the view model for the button component.
type Button struct {
	Label   string
	Primary bool // the accent-filled button, password screen only
}

// MovieRow is the view model for the movie-row component.
type MovieRow struct {
	Title  string
	Year   string // empty when unknown
	Added  string // e.g. "Added 14 Feb 2024"; empty when unknown
	Poster Poster
}

func newMovieRow(m library.Movie) MovieRow {
	return MovieRow{
		Title:  m.Title,
		Year:   formatYear(m.Year),
		Added:  formatAdded(m.Added),
		Poster: Poster{Title: m.Title, Initial: posterInitial(m.Title)},
	}
}

func newMoviesPage(snap library.Snapshot, loaded bool) MoviesPage {
	p := MoviesPage{Page: newPage("Movies", "movies", snap, loaded), Loaded: loaded}
	if !loaded {
		return p
	}
	p.Summary = formatCount(len(snap.Movies), "movie", "movies")
	p.Movies = make([]MovieRow, 0, len(snap.Movies))
	for _, m := range snap.Movies {
		p.Movies = append(p.Movies, newMovieRow(m))
	}
	return p
}

func newTVPage(snap library.Snapshot, loaded bool) TVPage {
	return TVPage{Page: newPage("TV shows", "tv", snap, loaded)}
}
