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

// Size is the view model for the size component: "14.6 GB", or "Unknown".
type Size struct {
	Value   string // "14.6"
	Unit    string // "GB"
	Unknown bool   // Jellyfin reported no size
	// UnknownCount is how many items a sum left out because their size is
	// unknown, shown as "+ 2 unknown" under the value.
	UnknownCount int
}

// newSize builds a single item's size; 0 bytes means Jellyfin reported none.
func newSize(bytes int64) Size {
	if bytes <= 0 {
		return Size{Unknown: true}
	}
	v, u := formatSize(bytes)
	return Size{Value: v, Unit: u}
}

// UserTag is the view model for the user-tag component: a user's name with
// their two-letter code on their hue.
type UserTag struct {
	Name string
	Code string // "SA", "AK"; what tells users apart
	Hue  int    // 1–10, selecting --user-1…--user-10
}

// userHues is how many --user-N tokens there are; past that, hues wrap.
const userHues = 10

// newUserTags gives every user their tag, keyed by user ID. Hues follow
// Jellyfin's /Users order, so a user looks the same everywhere in the app.
func newUserTags(users []library.User) map[string]UserTag {
	tags := make(map[string]UserTag, len(users))
	for i, u := range users {
		tags[u.ID] = UserTag{Name: u.Name, Code: userCode(u.Name), Hue: i%userHues + 1}
	}
	return tags
}

// groupCollapseAt is the number of users at which a user-group collapses to
// a stack of codes (Components sheet: "Five or more users, collapsed").
const groupCollapseAt = 5

// UserGroup is the view model for the user-group component. Collapsed groups
// show a stack of codes and "All 10 users" / "6 users", expanding to Tags.
type UserGroup struct {
	Tags        []UserTag
	Collapsed   bool
	CountPrefix string // "All " when the group is every user, else ""
	Count       string // "6"
	CountSuffix string // " users"
}

func newUserGroup(users []library.User, tags map[string]UserTag, totalUsers int) UserGroup {
	g := UserGroup{Tags: make([]UserTag, 0, len(users))}
	for _, u := range users {
		g.Tags = append(g.Tags, tags[u.ID])
	}
	if len(users) >= groupCollapseAt {
		g.Collapsed = true
		g.Count = formatNumber(len(users))
		g.CountSuffix = " users"
		if len(users) == totalUsers {
			g.CountPrefix = "All "
		}
	}
	return g
}

// Completion is the view model for the completion component: who has
// completed an item, or "Not completed by anyone". In-progress users arrive
// with TV seasons (ticket 06).
type Completion struct {
	Completed UserGroup
}

func newCompletion(completed []library.User, tags map[string]UserTag, totalUsers int) Completion {
	return Completion{Completed: newUserGroup(completed, tags, totalUsers)}
}

// MovieRow is the view model for the movie-row component.
type MovieRow struct {
	Title      string
	Year       string // empty when unknown
	Added      string // e.g. "Added 14 Feb 2024"; empty when unknown
	Poster     Poster
	Size       Size
	Completion Completion
}

func newMovieRow(m library.Movie, tags map[string]UserTag, totalUsers int) MovieRow {
	return MovieRow{
		Title:      m.Title,
		Year:       formatYear(m.Year),
		Added:      formatAdded(m.Added),
		Poster:     Poster{Title: m.Title, Initial: posterInitial(m.Title)},
		Size:       newSize(m.Size),
		Completion: newCompletion(m.CompletedBy, tags, totalUsers),
	}
}

func newMoviesPage(snap library.Snapshot, loaded bool) MoviesPage {
	p := MoviesPage{Page: newPage("Movies", "movies", snap, loaded), Loaded: loaded}
	if !loaded {
		return p
	}
	p.Summary = formatCount(len(snap.Movies), "movie", "movies")
	tags := newUserTags(snap.Users)
	p.Movies = make([]MovieRow, 0, len(snap.Movies))
	for _, m := range snap.Movies {
		p.Movies = append(p.Movies, newMovieRow(m, tags, len(snap.Users)))
	}
	return p
}

func newTVPage(snap library.Snapshot, loaded bool) TVPage {
	return TVPage{Page: newPage("TV shows", "tv", snap, loaded)}
}
