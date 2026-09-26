package web

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"jellyfin-watched/internal/library"
)

// Page is what the layout needs from every page: the document title, the
// header's tabs and its refresh control.
type Page struct {
	Title   string
	Tabs    []Tab
	Refresh RefreshControl
}

// Tab is one of the header's Movies / TV shows tabs.
type Tab struct {
	Label   string
	Href    string
	Count   string // empty until the library has loaded
	Current bool
}

// newPage builds the layout's view model. current is "movies" or "tv". The
// tabs carry v's sort and direction to the other tab, but not its search or
// the TV page's expansion.
func newPage(title, current string, st library.Status, now time.Time, v library.View) Page {
	carried := library.View{Sort: v.Sort, Desc: v.Desc}
	movies := Tab{Label: "Movies", Href: viewURL("/movies", carried, expansion{}), Current: current == "movies"}
	tv := Tab{Label: "TV shows", Href: viewURL("/tv", carried, expansion{}), Current: current == "tv"}
	if st.Loaded {
		movies.Count = formatNumber(len(st.Snapshot.Movies))
		tv.Count = formatNumber(len(st.Snapshot.Series))
	}
	return Page{Title: title, Tabs: []Tab{movies, tv}, Refresh: newRefreshControl(st, now)}
}

// refreshPath is where the refresh control starts a refresh (POST) and
// fetches its own state (GET).
const refreshPath = "/refresh"

// RefreshControl is the view model for the refresh-control component:
// "Updated 12 minutes ago" and the "Refresh now" / "Refreshing…" button,
// plus the loading hairline under the header while a refresh runs. The
// control fetches itself from Poll every Every, so the age stays current
// and the page learns when new data has arrived.
type RefreshControl struct {
	Updated string  // "Updated 12 minutes ago"; empty until the first load
	Button  *Button // nil during the first load, when there's nothing to refresh
	Loading bool    // a refresh is running: the hairline slides
	Poll    string  // e.g. "/refresh?since=g3"
	Every   string  // "2s" while loading, "30s" otherwise
}

// Poll intervals: often enough while loading that new rows appear soon
// after they arrive, and otherwise often enough that "N minutes ago" is
// never more than a minute out.
const (
	pollLoading = "2s"
	pollIdle    = "30s"
)

// stateKey names what a page shows: which snapshot, and whether the last
// refresh failed. The refresh control carries it, so a poll can tell the
// page to reload once either has changed. "g3" is the third snapshot; "g3f"
// the same with a failed refresh since.
func stateKey(st library.Status) string {
	k := "g" + strconv.Itoa(st.Generation)
	if st.Failed {
		k += "f"
	}
	return k
}

func newRefreshControl(st library.Status, now time.Time) RefreshControl {
	rc := RefreshControl{
		Loading: st.Refreshing || (!st.Loaded && !st.Failed),
		Poll:    refreshPath + "?since=" + stateKey(st),
		Every:   pollIdle,
	}
	if rc.Loading {
		rc.Every = pollLoading
	}
	if st.Loaded {
		rc.Updated = "Updated " + formatAgo(now.Sub(st.Snapshot.LoadedAt))
	}
	if !st.Loaded && !st.Failed {
		return rc // first load: the hairline alone
	}
	// The ID stays the same in both states, so htmx keeps focus on the
	// button when the control swaps.
	rc.Button = &Button{Label: "Refresh now", Icon: "refresh", ID: "refresh-now",
		Post: refreshPath, Target: "#refresh-control", Swap: "outerHTML"}
	if st.Refreshing {
		rc.Button = &Button{Label: "Refreshing…", Icon: "spinner", ID: "refresh-now", Busy: true}
	}
	return rc
}

// Notice is the view model for the notice component: what happened, then
// what the app is doing about it.
type Notice struct {
	Headline string
	Detail   string
}

// newNotice explains a failed refresh, or returns nil if the last one
// succeeded. With data loaded, the last good data stays on screen.
func newNotice(st library.Status, now time.Time) *Notice {
	if !st.Failed {
		return nil
	}
	if !st.Loaded {
		return &Notice{
			Headline: "Can't reach Jellyfin, so the library hasn't loaded yet.",
			Detail:   "Refresh now tries the connection again.",
		}
	}
	return &Notice{
		Headline: "Can't reach Jellyfin. Showing data from " + formatAgo(now.Sub(st.Snapshot.LoadedAt)) + ".",
		Detail:   "Last updated " + formatClock(st.Snapshot.LoadedAt, now) + ". Refresh now tries the connection again.",
	}
}

// EmptyState is the view model for the empty-state component, shown in
// place of a list. Loading is the first-load variant, centred under the
// Spool reel.
type EmptyState struct {
	Loading  bool
	Headline string
	Lines    []string
	Action   *Button // e.g. "Clear search"; nil for none
}

// firstLoad is the empty state before the library has loaded, unless the
// first refresh failed (a notice says so instead).
func firstLoad(st library.Status) *EmptyState {
	if st.Loaded || st.Failed {
		return nil
	}
	return &EmptyState{
		Loading:  true,
		Headline: "Loading your library. This can take a minute on first start.",
		Lines:    []string{"The page will show your movies and TV shows as soon as Jellyfin has sent them."},
	}
}

// emptyLibrary is the empty state for a library with none of an item kind:
// what ("movies"), what Jellyfin calls them ("series") and how new ones
// are described ("Shows").
func emptyLibrary(st library.Status, now time.Time, what, jellyfinName, added string) *EmptyState {
	return &EmptyState{
		Headline: "There are no " + what + " on this server",
		Lines: []string{
			"Jellyfin reported no " + jellyfinName + " at the last refresh, " + formatAgo(now.Sub(st.Snapshot.LoadedAt)) + ".",
			added + " added in Jellyfin appear here after the next refresh.",
		},
	}
}

// noMatches is the empty state for a search that matched nothing, e.g.
// "No movies match “xyz”", with a button that clears the search.
func noMatches(what, search string) *EmptyState {
	return &EmptyState{
		Headline: "No " + what + " match “" + search + "”",
		Lines:    []string{"Search looks at titles only."},
		Action:   clearSearchButton(),
	}
}

// listControlsID is the ID of the search and sort form; "Clear search"
// buttons outside it submit it through their form attribute.
const listControlsID = "list-controls"

// clearSearchButton is "Clear search": a submit button of the list-controls
// form that clears the search. With htmx it empties the search box, moves
// focus there and submits the form; with no JavaScript it submits
// clear=1.
func clearSearchButton() *Button {
	return &Button{Label: "Clear search", Type: "submit", Form: listControlsID, Name: "clear", Value: "1", ClearsSearch: true}
}

// matchSummary is the page summary during a search: "3 of 1,284 movies
// match".
func matchSummary(matched, total int, singular, plural string) string {
	return formatNumber(matched) + " of " + formatCount(total, singular, plural) + " match"
}

// ListControls is the view model for the list-controls component: the
// search box and sort control above a list, as one GET form. With no
// JavaScript it submits normally; with htmx, typing (debounced) or a sort
// button fetches the page and swaps in the results and the parts of the page
// head that change (SelectOOB), leaving the search box as it is.
type ListControls struct {
	ID        string // listControlsID
	Action    string // the page's path, "/movies" or "/tv"
	SelectOOB string // IDs swapped in alongside the results
	Seasons   string // the TV page's ?seasons=, kept across searches
	Search    Search
	Sort      SortControl
}

// Search is the view model for the search component: the box and, once
// it has text, a × that clears it.
type Search struct {
	ID    string // the input's ID, which the form's trigger listens to
	Value string
}

// SortControl is the view model for the sort-control component: "Sort by"
// and a segment per key, then the direction button. Sort and Dir are the
// current view, carried as hidden fields; the segments and the direction
// button submit set-sort and set-dir, which replace them.
type SortControl struct {
	Sort, Dir string
	Options   []SortOption
	Direction Button
}

// SortOption is one segment of the sort control. Every key looks the same
// apart from which is selected (DESIGN.md §3).
type SortOption struct {
	ID       string // "sort-size", stable across swaps so focus stays put
	Label    string
	Value    string
	Selected bool
}

var sortLabels = map[library.SortKey]string{
	library.SortTitle: "Title",
	library.SortYear:  "Year",
	library.SortAdded: "Date added",
	library.SortSize:  "Size",
}

// parseView reads a list's view from its query: q, sort and dir, then the
// form's buttons, set-sort, set-dir and clear, which override them.
func parseView(q url.Values) library.View {
	v := library.View{
		Search: strings.TrimSpace(q.Get("q")),
		Sort:   library.ParseSortKey(q.Get("sort")),
		Desc:   q.Get("dir") == "desc",
	}
	if s := q.Get("set-sort"); s != "" {
		v.Sort = library.ParseSortKey(s)
	}
	if d := q.Get("set-dir"); d != "" {
		v.Desc = d == "desc"
	}
	if q.Get("clear") != "" {
		v.Search = ""
	}
	return v
}

// viewQuery is the tidy query for a view: only what differs from the
// default (title, ascending, no search), plus the TV page's expansion.
func viewQuery(v library.View, exp expansion) url.Values {
	q := url.Values{}
	if v.Search != "" {
		q.Set("q", v.Search)
	}
	if v.Sort != library.SortTitle && v.Sort != "" {
		q.Set("sort", string(v.Sort))
	}
	if v.Desc {
		q.Set("dir", "desc")
	}
	if exp.seasons != "" {
		q.Set("seasons", exp.seasons)
	}
	if len(exp.except) > 0 {
		q[exp.exceptKey()] = exp.except
	}
	return q
}

// viewURL is path with the view's tidy query.
func viewURL(path string, v library.View, exp expansion) string {
	if q := viewQuery(v, exp).Encode(); q != "" {
		return path + "?" + q
	}
	return path
}

// newListControls builds a list's search and sort form. seasons is the TV
// page's ?seasons=, which the form keeps; the series shown or hidden one by
// one aren't kept, so a search or sort starts them afresh.
func newListControls(action string, v library.View, seasons string, selectOOB ...string) *ListControls {
	c := &ListControls{
		ID:        listControlsID,
		Action:    action,
		SelectOOB: "#page-summary,#sort-control",
		Seasons:   seasons,
		Search:    Search{ID: "search-q", Value: v.Search},
		Sort:      SortControl{Sort: string(v.Sort), Dir: "asc"},
	}
	for _, id := range selectOOB {
		c.SelectOOB += ",#" + id
	}
	if c.Sort.Sort == "" {
		c.Sort.Sort = string(library.SortTitle)
	}
	for _, k := range library.SortKeys {
		c.Sort.Options = append(c.Sort.Options, SortOption{ID: "sort-" + string(k), Label: sortLabels[k],
			Value: string(k), Selected: string(k) == c.Sort.Sort})
	}
	// The button shows the current direction and switches to the other.
	c.Sort.Direction = Button{Label: "Ascending", Icon: "arrow-up", ID: "sort-dir", Type: "submit", Name: "set-dir", Value: "desc"}
	if v.Desc {
		c.Sort.Dir = "desc"
		c.Sort.Direction.Label, c.Sort.Direction.Icon, c.Sort.Direction.Value = "Descending", "arrow-down", "asc"
	}
	return c
}

// MoviesPage is the view model for pages/movies.tmpl.
type MoviesPage struct {
	Page
	Loaded   bool
	Summary  string        // e.g. "1,284 movies", or "3 of 1,284 movies match"
	Controls *ListControls // search and sort; nil with no movies
	Notice   *Notice       // a failed refresh; nil normally
	Empty    *EmptyState   // first load, no movies or no matches; nil when there's a list
	Movies   []MovieRow
}

// TVPage is the view model for pages/tv.tmpl.
type TVPage struct {
	Page
	Loaded    bool
	Summary   string        // e.g. "212 series · 1,406 seasons · 9.7 TB on disk"
	Controls  *ListControls // search and sort; nil with no series
	Notice    *Notice       // a failed refresh; nil normally
	Empty     *EmptyState   // first load, no series or no matches; nil when there's a list
	ExpandAll *Button       // "Expand all" / "Collapse all"; nil with no series
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
	// Icon is drawn before the label: "expand-all", "collapse-all",
	// "refresh", or "spinner" (an arc that spins, still under reduced
	// motion). Disclosure buttons draw their own chevron.
	Icon string
	ID   string // stable across htmx swaps, so focus stays on the button
	// Busy is an action already under way ("Refreshing…"): dimmed and
	// aria-disabled, but still focusable so focus isn't lost.
	Busy bool
	// Expanded is aria-expanded, "true" or "false"; "" leaves it out.
	Expanded string
	Controls string // aria-controls: the ID of what it expands
	// Href makes the button a link to this URL, which is followed when
	// htmx isn't there to handle Get (no JavaScript). Type, Name, Value and
	// Form don't apply to a link.
	Href string
	// htmx: the fragment to GET, or the action to POST, where the response
	// goes and how. PushURL puts the GET's URL in the address bar.
	Get, Post, Target, Swap string
	PushURL                 bool
	// Type is "submit" for a form's button; "" is type="button". Name and
	// Value are what it submits, and Form the ID of a form it submits from
	// outside it.
	Type, Name, Value, Form string
	// ClearsSearch makes the button clear the list-controls search box,
	// focus it and submit the form (with JavaScript).
	ClearsSearch bool
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

func newMoviesPage(st library.Status, now time.Time, v library.View) MoviesPage {
	p := MoviesPage{Page: newPage("Movies", "movies", st, now, v), Loaded: st.Loaded,
		Notice: newNotice(st, now), Empty: firstLoad(st)}
	if !st.Loaded {
		return p
	}
	snap := st.Snapshot
	p.Summary = formatCount(len(snap.Movies), "movie", "movies")
	var bytes int64
	for _, m := range snap.Movies {
		bytes += m.Size
	}
	p.Summary += onDisk(bytes)
	if len(snap.Movies) == 0 {
		p.Empty = emptyLibrary(st, now, "movies", "movies", "Movies")
		return p
	}
	p.Controls = newListControls("/movies", v, "")
	movies := library.ArrangeMovies(snap.Movies, v)
	if v.Search != "" {
		p.Summary = matchSummary(len(movies), len(snap.Movies), "movie", "movies")
		if len(movies) == 0 {
			p.Empty = noMatches("movies", v.Search)
		}
	}
	tags := newUserTags(snap.Users)
	p.Movies = make([]MovieRow, 0, len(movies))
	for _, m := range movies {
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
	// Completion rolls the seasons' completion up to the series. It is nil
	// while the seasons are shown, so the same facts aren't shown twice, and
	// for a series with no episodes.
	Completion *Completion
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

// expansion is which series show their seasons: the TV page's ?seasons=
// and ?show= or ?hide=. Every series starts collapsed; seasons=all
// ("Expand all") shows them all, and seasons=none ("Collapse all") hides
// them all, as the default does. except lists the series one has shown
// (show=) or, under seasons=all, hidden (hide=), sorted and without
// repeats. It lives in the URL so that the page's self-reload after a
// refresh, back and forward, and use without JavaScript all keep it.
type expansion struct {
	seasons string // "all", "none" or ""
	except  []string
}

// parseExpansion reads a page's expansion from its query, dropping
// anything that doesn't apply.
func parseExpansion(q url.Values) expansion {
	exp := expansion{seasons: q.Get("seasons")}
	if exp.seasons != "all" && exp.seasons != "none" {
		exp.seasons = ""
	}
	for _, id := range q[exp.exceptKey()] {
		if id != "" {
			exp.except = append(exp.except, id)
		}
	}
	slices.Sort(exp.except)
	exp.except = slices.Compact(exp.except)
	return exp
}

// exceptKey is the query key that lists exceptions: "hide" under
// seasons=all, "show" otherwise.
func (e expansion) exceptKey() string {
	if e.seasons == "all" {
		return "hide"
	}
	return "show"
}

// expanded reports whether the series with this ID shows its seasons.
func (e expansion) expanded(id string) bool {
	_, listed := slices.BinarySearch(e.except, id)
	return (e.seasons == "all") != listed
}

// with returns the expansion with the series shown or hidden.
func (e expansion) with(id string, expanded bool) expansion {
	if e.expanded(id) == expanded {
		return e
	}
	i, listed := slices.BinarySearch(e.except, id)
	out := expansion{seasons: e.seasons, except: slices.Clone(e.except)}
	if listed {
		out.except = slices.Delete(out.except, i, i+1)
	} else {
		out.except = slices.Insert(out.except, i, id)
	}
	return out
}

// seriesPath is where a series row is served as an htmx fragment.
const seriesPath = "/tv/series/"

// newSeriesRow builds a series row as it shows on the TV page with view v
// and expansion exp: its seasons if exp expands it, otherwise its roll-up.
// Its disclosure links to the page with the series the other way (without
// JavaScript), and with htmx fetches the row that way from seriesPath.
func newSeriesRow(sr library.Series, tags map[string]UserTag, totalUsers int, v library.View, exp expansion) SeriesRow {
	row := SeriesRow{
		ID:        "series-" + sr.ID,
		Title:     sr.Title,
		Year:      formatYear(sr.Year),
		Counts:    formatCount(len(sr.Seasons), "season", "seasons") + " · " + formatCount(sr.Episodes, "episode", "episodes"),
		Poster:    newPoster(sr.ID, sr.Title, sr.PosterTag),
		Size:      newSumSize(sr.Size, sr.UnknownSizes),
		SeasonsID: "series-" + sr.ID + "-seasons",
	}
	expanded := exp.expanded(sr.ID) && len(sr.Seasons) > 0
	if !expanded && sr.Episodes > 0 {
		c := newCompletion(sr.CompletedBy, sr.InProgress, tags, totalUsers)
		row.Completion = &c
	}
	if len(sr.Seasons) == 0 {
		return row
	}
	toggle := &Button{
		Disclosure: true,
		ID:         row.ID + "-toggle",
		Href:       viewURL("/tv", v, exp.with(sr.ID, !expanded)) + "#" + row.ID,
		Target:     "closest .series-row",
		Swap:       "outerHTML",
	}
	get := seriesPath + url.PathEscape(sr.ID) + "?seasons="
	if expanded {
		toggle.Label, toggle.Expanded, toggle.Controls, toggle.Get = "Hide seasons", "true", row.SeasonsID, get+"hide"
		for _, s := range sr.Seasons {
			row.Seasons = append(row.Seasons, SeasonRow{
				Name:       s.Name,
				Episodes:   formatCount(s.Episodes, "episode", "episodes"),
				Size:       newSumSize(s.Size, s.UnknownSizes),
				Completion: newCompletion(s.CompletedBy, s.InProgress, tags, totalUsers),
			})
		}
	} else {
		toggle.Label, toggle.Expanded, toggle.Get = "Show "+formatCount(len(sr.Seasons), "season", "seasons"), "false", get+"show"
	}
	row.Toggle = toggle
	return row
}

// onDisk is the summary's " · 9.7 TB on disk", counting only known sizes,
// or nothing when no size is known.
func onDisk(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	value, unit := formatSize(bytes)
	return " · " + value + " " + unit + " on disk"
}

// newTVPage builds the TV shows page with view v and expansion exp.
func newTVPage(st library.Status, now time.Time, v library.View, exp expansion) TVPage {
	p := TVPage{Page: newPage("TV shows", "tv", st, now, v), Loaded: st.Loaded,
		Notice: newNotice(st, now), Empty: firstLoad(st)}
	if !st.Loaded {
		return p
	}
	snap := st.Snapshot
	if len(snap.Series) == 0 {
		p.Empty = emptyLibrary(st, now, "TV shows", "series", "Shows")
		p.Summary = formatCount(0, "series", "series")
		return p
	}
	nSeasons := 0
	var bytes int64
	for _, sr := range snap.Series {
		nSeasons += len(sr.Seasons)
		bytes += sr.Size
	}
	p.Summary = formatCount(len(snap.Series), "series", "series") + " · " + formatCount(nSeasons, "season", "seasons") + onDisk(bytes)
	p.Controls = newListControls("/tv", v, exp.seasons, "tv-expand-all")
	// "Expand all" keeps the search and sort; it is there even when the
	// search matches nothing, so the swapped-in page head keeps its place.
	p.ExpandAll = &Button{Label: "Expand all", Icon: "expand-all", ID: "tv-expand-all",
		Get: viewURL("/tv", v, expansion{seasons: "all"}), Target: "main", Swap: "innerHTML", PushURL: true}
	if exp.seasons == "all" {
		p.ExpandAll.Label, p.ExpandAll.Icon, p.ExpandAll.Get = "Collapse all", "collapse-all", viewURL("/tv", v, expansion{seasons: "none"})
	}

	series := library.ArrangeSeries(snap.Series, v)
	if v.Search != "" {
		p.Summary = matchSummary(len(series), len(snap.Series), "series", "series")
		if len(series) == 0 {
			p.Empty = noMatches("TV shows", v.Search)
		}
	}
	tags := newUserTags(snap.Users)
	p.Series = make([]SeriesRow, 0, len(series))
	for _, sr := range series {
		p.Series = append(p.Series, newSeriesRow(sr, tags, len(snap.Users), v, exp))
	}
	return p
}

// PasswordPage is the view model for the password screen.
type PasswordPage struct {
	Title string
	Form  PasswordForm
}

// PasswordForm is the view model for the password-form component. Error,
// when set, is shown under the field ("That password is not correct…").
// The password typed is never put back into the page.
type PasswordForm struct {
	Action string
	Next   string // the dashboard path to return to
	Error  string
	Submit Button
}

func newPasswordPage(next, errMsg string) PasswordPage {
	return PasswordPage{
		Title: "Enter the password",
		Form: PasswordForm{Action: loginPath, Next: nextOrHome(next), Error: errMsg,
			Submit: Button{Label: "Open dashboard", Primary: true, Type: "submit"}},
	}
}

const (
	wrongPassword   = "That password is not correct. Check it and try again."
	tooManyPassword = "Too many wrong passwords. Wait a minute and try again."
)
