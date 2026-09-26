package web

import (
	"net/url"

	"jellyfin-watched/internal/library"
)

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
	tv := Tab{Label: "TV shows", Href: "/tv", Current: current == "tv"}
	if loaded {
		tv.Count = formatNumber(len(snap.Series))
	}
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
	Loaded    bool
	Summary   string  // e.g. "212 series · 1,406 seasons · 9.7 TB on disk"
	ExpandAll *Button // "Expand all" / "Collapse all"; nil with no series
	Series    []SeriesRow
}

// Poster is the view model for the poster component: an image when Src is
// set, the missing-poster box when Missing, otherwise the generated
// placeholder showing Initial.
type Poster struct {
	Title   string // the image's alt text
	Src     string // the dashboard's poster URL, e.g. "/posters/{id}/{tag}"
	Missing bool   // Jellyfin has no poster for the item
	Initial string // placeholder only; may be empty, leaving a plain block
}

// posterPath is where the dashboard serves posters.
const posterPath = "/posters/"

// newPoster builds an item's poster. tag is its Jellyfin poster tag, "" when
// it has none. The URL points at the dashboard, never at Jellyfin, and
// includes the tag so that a changed poster gets a new URL.
func newPoster(id, title, tag string) Poster {
	if tag == "" {
		return Poster{Title: title, Missing: true}
	}
	return Poster{Title: title, Src: posterPath + url.PathEscape(id) + "/" + url.PathEscape(tag)}
}

// Button is the view model for the button component. The zero value of every
// field but Label gives the default outlined button.
type Button struct {
	Label   string
	Primary bool // the accent-filled button, password screen only
	// Disclosure is the borderless "Show 35 seasons" / "Hide seasons"
	// button, with a chevron that follows Expanded.
	Disclosure bool
	// Icon is drawn before the label: "expand-all" or "collapse-all".
	// Disclosure buttons draw their own chevron.
	Icon string
	ID   string // stable across htmx swaps, so focus stays on the button
	// Expanded is aria-expanded, "true" or "false"; "" leaves it out.
	Expanded string
	Controls string // aria-controls: the ID of what it expands
	// htmx: the fragment to GET, where it goes and how. PushURL puts the
	// GET's URL in the address bar.
	Get, Target, Swap string
	PushURL           bool
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
// their two-letter code on their hue. An in-progress tag also shows how many
// episodes they've played, "9 / 12", with a thin bar.
type UserTag struct {
	Name string
	Code string // "SA", "AK"; what tells users apart
	Hue  int    // 1–10, selecting --user-1…--user-10
	// Played and Available are set for an in-progress tag only.
	Played, Available string
	Percent           int // the bar's fill, 1–99
}

// newProgressTag is the user's tag with their progress through a season.
func newProgressTag(tag UserTag, p library.Progress) UserTag {
	tag.Played = formatNumber(p.Played)
	tag.Available = formatNumber(p.Available)
	// At least a sliver shows for "1 / 200"; never full, since in progress
	// means not every episode.
	tag.Percent = min(max(p.Played*100/max(p.Available, 1), 1), 99)
	return tag
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
// completed an item, then each user in progress (seasons only), or "Not
// completed by anyone" when there are neither.
type Completion struct {
	Completed  UserGroup
	InProgress []UserTag
}

func newCompletion(completed []library.User, inProgress []library.Progress, tags map[string]UserTag, totalUsers int) Completion {
	c := Completion{Completed: newUserGroup(completed, tags, totalUsers)}
	for _, p := range inProgress {
		c.InProgress = append(c.InProgress, newProgressTag(tags[p.User.ID], p))
	}
	return c
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
		Poster:     newPoster(m.ID, m.Title, m.PosterTag),
		Size:       newSize(m.Size),
		Completion: newCompletion(m.CompletedBy, nil, tags, totalUsers),
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

// newSumSize builds the size of a sum: the bytes that are known, with a note
// of how many items were left out because their size is unknown. It is
// "Unknown" only when nothing in the sum has a known size.
func newSumSize(bytes int64, unknown int) Size {
	if bytes <= 0 && unknown > 0 {
		return Size{Unknown: true}
	}
	v, u := formatSize(bytes)
	return Size{Value: v, Unit: u, UnknownCount: unknown}
}

// SeriesRow is the view model for the series-row component.
type SeriesRow struct {
	ID     string // the element ID, "series-{id}"
	Title  string
	Year   string // empty when unknown
	Counts string // "2 seasons · 24 episodes"
	Poster Poster
	Size   Size
	// Toggle shows or hides the seasons; nil when there are none.
	Toggle *Button
	// Seasons are shown when the series is expanded, and nil otherwise.
	Seasons   []SeasonRow
	SeasonsID string // the season list's element ID
}

// SeasonRow is the view model for the season-row component.
type SeasonRow struct {
	Name       string // "Season 2"
	Episodes   string // "12 episodes"
	Size       Size
	Completion Completion
}

// expandedByDefault is how many seasons a series can have and still show
// them when the page loads. Longer-running series start collapsed behind
// "Show 35 seasons" (TV shows screen: The Simpsons). DESIGN.md §7 gives 1–5
// seasons as typical.
const expandedByDefault = 5

// expansion is which series show their seasons, set by /tv?seasons= and a
// series fragment's ?seasons=: every one ("all"/"show"), none
// ("none"/"hide"), or the default.
type expansion int

const (
	expandDefault expansion = iota
	expandAll
	expandNone
)

func parseExpansion(v string) expansion {
	switch v {
	case "all", "show":
		return expandAll
	case "none", "hide":
		return expandNone
	}
	return expandDefault
}

// seriesPath is where a series row is served as an htmx fragment.
const seriesPath = "/tv/series/"

func newSeriesRow(sr library.Series, tags map[string]UserTag, totalUsers int, exp expansion) SeriesRow {
	episodes := 0
	for _, s := range sr.Seasons {
		episodes += s.Episodes
	}
	row := SeriesRow{
		ID:        "series-" + sr.ID,
		Title:     sr.Title,
		Year:      formatYear(sr.Year),
		Counts:    formatCount(len(sr.Seasons), "season", "seasons") + " · " + formatCount(episodes, "episode", "episodes"),
		Poster:    newPoster(sr.ID, sr.Title, sr.PosterTag),
		Size:      newSumSize(sr.Size, sr.UnknownSizes),
		SeasonsID: "series-" + sr.ID + "-seasons",
	}
	if len(sr.Seasons) == 0 {
		return row
	}
	expanded := exp == expandAll || (exp == expandDefault && len(sr.Seasons) <= expandedByDefault)
	toggle := &Button{
		Disclosure: true,
		ID:         row.ID + "-toggle",
		Target:     "closest .series-row",
		Swap:       "outerHTML",
	}
	href := seriesPath + url.PathEscape(sr.ID) + "?seasons="
	if expanded {
		toggle.Label, toggle.Expanded, toggle.Controls, toggle.Get = "Hide seasons", "true", row.SeasonsID, href+"hide"
		for _, s := range sr.Seasons {
			row.Seasons = append(row.Seasons, SeasonRow{
				Name:       s.Name,
				Episodes:   formatCount(s.Episodes, "episode", "episodes"),
				Size:       newSumSize(s.Size, s.UnknownSizes),
				Completion: newCompletion(s.CompletedBy, s.InProgress, tags, totalUsers),
			})
		}
	} else {
		toggle.Label, toggle.Expanded, toggle.Get = "Show "+formatCount(len(sr.Seasons), "season", "seasons"), "false", href+"show"
	}
	row.Toggle = toggle
	return row
}

func newTVPage(snap library.Snapshot, loaded bool, exp expansion) TVPage {
	p := TVPage{Page: newPage("TV shows", "tv", snap, loaded), Loaded: loaded}
	if !loaded {
		return p
	}
	tags := newUserTags(snap.Users)
	seasons := 0
	var bytes int64
	p.Series = make([]SeriesRow, 0, len(snap.Series))
	for _, sr := range snap.Series {
		seasons += len(sr.Seasons)
		bytes += sr.Size
		p.Series = append(p.Series, newSeriesRow(sr, tags, len(snap.Users), exp))
	}
	p.Summary = formatCount(len(snap.Series), "series", "series") + " · " + formatCount(seasons, "season", "seasons")
	if bytes > 0 {
		v, u := formatSize(bytes)
		p.Summary += " · " + v + " " + u + " on disk"
	}
	if len(p.Series) > 0 {
		p.ExpandAll = &Button{Label: "Expand all", Icon: "expand-all", ID: "tv-expand-all",
			Get: "/tv?seasons=all", Target: "main", Swap: "innerHTML", PushURL: true}
		if exp == expandAll {
			p.ExpandAll.Label, p.ExpandAll.Icon, p.ExpandAll.Get = "Collapse all", "collapse-all", "/tv?seasons=none"
		}
	}
	return p
}
