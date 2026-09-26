package library

import (
	"context"
	"sort"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

// Series is one TV series in the library.
type Series struct {
	ID    string
	Title string
	Year  int       // 0 when Jellyfin has no production year
	Added time.Time // when it was added to the library; zero if unknown
	// PosterTag identifies the current version of the series poster in
	// Jellyfin; "" when it has none.
	PosterTag string
	// Size is the total bytes of every available episode of every season,
	// including Specials even when they're hidden (REQUIREMENTS.md §3).
	// Episodes with no reported size are left out and counted in
	// UnknownSizes.
	Size         int64
	UnknownSizes int
	// Seasons are the seasons to show, in season order: those with at least
	// one available episode, without Specials unless Options.IncludeSpecials.
	Seasons []Season
}

// Season is one season of a series.
type Season struct {
	ID   string
	Name string // e.g. "Season 2", "Specials", "Season Unknown"
	// PosterID and PosterTag are the season's poster: its own if it has
	// one, otherwise the series poster (tag "" when neither has one).
	PosterID, PosterTag string
	// Episodes is how many episodes are available: in the library, not
	// missing placeholders.
	Episodes int
	// Size is the total bytes of the available episodes that report a size;
	// UnknownSizes counts those that don't.
	Size         int64
	UnknownSizes int
	// CompletedBy is the users who have played every available episode, in
	// Snapshot.Users order; nil when nobody has.
	CompletedBy []User
	// InProgress is the users who have played some but not all available
	// episodes, in Snapshot.Users order. Users who haven't played any aren't
	// listed.
	InProgress []Progress
}

// Progress is how far one user is through a season.
type Progress struct {
	User      User
	Played    int // available episodes the user has played; at least 1
	Available int // the season's available episodes
}

// specials is the season number Jellyfin gives Specials.
const specials = 0

// seasonBuild is a season while a refresh assembles it.
type seasonBuild struct {
	Season
	series  int  // index into the series slice
	number  *int // Jellyfin's IndexNumber; nil when it has none
	special bool
}

// fetchTV fetches everything the TV shows list needs: series and seasons
// once, every available episode with its size once, and each user's episodes
// with their watch state in one paginated request per user.
func fetchTV(ctx context.Context, src Source, users []User, opts Options) ([]Series, error) {
	seriesItems, err := src.Series(ctx)
	if err != nil {
		return nil, err
	}
	seasonItems, err := src.Seasons(ctx)
	if err != nil {
		return nil, err
	}
	episodes, err := src.Episodes(ctx)
	if err != nil {
		return nil, err
	}
	b := newTVBuilder(seriesItems, seasonItems, episodes)
	for _, u := range users {
		eps, err := src.UserEpisodes(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		b.addUser(u, eps)
	}
	return b.result(opts), nil
}

// tvBuilder turns Jellyfin's flat series, season and episode lists into
// Series with their Seasons.
type tvBuilder struct {
	series    []Series
	seasons   []*seasonBuild
	seasonIdx map[string]*seasonBuild // by season key
	episodeOf map[string]*seasonBuild // available episode ID → its season
}

// seasonKey is the season an episode is grouped under: its SeasonId, or, for
// an episode Jellyfin couldn't place in a season (no SeasonId, seen on
// 12.1.0), one stand-in season per series.
func seasonKey(ep jellyfin.Item) string {
	if ep.SeasonID != "" {
		return ep.SeasonID
	}
	return "unplaced:" + ep.SeriesID
}

func newTVBuilder(seriesItems, seasonItems, episodes []jellyfin.Item) *tvBuilder {
	b := &tvBuilder{
		series:    make([]Series, len(seriesItems)),
		seasonIdx: map[string]*seasonBuild{},
		episodeOf: make(map[string]*seasonBuild, len(episodes)),
	}
	seriesIdx := make(map[string]int, len(seriesItems))
	for i, it := range seriesItems {
		b.series[i] = Series{ID: it.ID, Title: it.Name, Year: it.ProductionYear, Added: it.DateCreated, PosterTag: it.PrimaryImageTag()}
		seriesIdx[it.ID] = i
	}
	addSeason := func(key, name string, series int, number *int, poster jellyfin.Item) *seasonBuild {
		s := &seasonBuild{Season: Season{ID: key, Name: name}, series: series, number: number,
			special: number != nil && *number == specials}
		if tag := poster.PrimaryImageTag(); tag != "" {
			s.PosterID, s.PosterTag = poster.ID, tag
		} else if sr := b.series[series]; sr.PosterTag != "" {
			s.PosterID, s.PosterTag = sr.ID, sr.PosterTag
		}
		b.seasons = append(b.seasons, s)
		b.seasonIdx[key] = s
		return s
	}
	for _, it := range seasonItems {
		if i, ok := seriesIdx[it.SeriesID]; ok {
			addSeason(it.ID, it.Name, i, it.IndexNumber, it)
		}
	}
	for _, ep := range episodes {
		i, ok := seriesIdx[ep.SeriesID]
		if !ok {
			continue
		}
		key := seasonKey(ep)
		s, ok := b.seasonIdx[key]
		if !ok {
			name := ep.SeasonName
			if name == "" {
				name = "Season Unknown"
			}
			s = addSeason(key, name, i, nil, jellyfin.Item{})
		}
		s.Episodes++
		if size := totalSize(ep.MediaSources); size > 0 {
			s.Size += size
		} else {
			s.UnknownSizes++
		}
		b.episodeOf[ep.ID] = s
	}
	return b
}

// addUser records one user's progress through every season. Users must be
// added in Snapshot.Users order. Only episodes in the library-wide list
// count, so a season's available episodes are the same for every user: one
// who can't see an episode (library access) can't complete its season.
func (b *tvBuilder) addUser(u User, eps []jellyfin.Item) {
	played := map[*seasonBuild]int{}
	for _, ep := range eps {
		if ep.UserData == nil || !ep.UserData.Played {
			continue
		}
		if s, ok := b.episodeOf[ep.ID]; ok {
			played[s]++
		}
	}
	for _, s := range b.seasons {
		switch n := played[s]; {
		case n == 0:
		case n >= s.Episodes:
			s.CompletedBy = append(s.CompletedBy, u)
		default:
			s.InProgress = append(s.InProgress, Progress{User: u, Played: n, Available: s.Episodes})
		}
	}
}

// result totals each series and picks the seasons to show, in season order:
// by number, with unnumbered seasons last.
func (b *tvBuilder) result(opts Options) []Series {
	seasons := append([]*seasonBuild(nil), b.seasons...)
	sort.SliceStable(seasons, func(i, j int) bool {
		a, c := seasons[i].number, seasons[j].number
		if a == nil || c == nil {
			return a != nil && c == nil
		}
		return *a < *c
	})
	for _, s := range seasons {
		sr := &b.series[s.series]
		sr.Size += s.Size
		sr.UnknownSizes += s.UnknownSizes
		// A season with no available episodes (e.g. an empty duplicate
		// "Season 1" folder, or only missing episodes) has nothing to show.
		if s.Episodes == 0 || (s.special && !opts.IncludeSpecials) {
			continue
		}
		sr.Seasons = append(sr.Seasons, s.Season)
	}
	return b.series
}
