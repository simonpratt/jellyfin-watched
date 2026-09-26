// Package library holds the in-memory snapshot of the Jellyfin library that
// pages read from. Pages never call Jellyfin directly (REQUIREMENTS.md §9.3).
package library

import (
	"context"
	"sync/atomic"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

// User is one Jellyfin user.
type User struct {
	ID   string
	Name string
}

// Movie is one movie in the library.
type Movie struct {
	ID    string
	Title string
	Year  int       // 0 when Jellyfin has no production year
	Added time.Time // when it was added to the library; zero if unknown
	// Size is the total bytes of every version of the movie, or 0 when
	// Jellyfin doesn't report a size for all of them.
	Size int64
	// PosterTag identifies the current version of the movie's poster in
	// Jellyfin; "" when it has none.
	PosterTag string
	// CompletedBy is the users Jellyfin reports as having played the movie
	// (UserData.Played), in Snapshot.Users order; nil when nobody has.
	CompletedBy []User
}

// Snapshot is a complete, immutable view of the library at one moment.
type Snapshot struct {
	Users    []User // in Jellyfin's /Users order
	Movies   []Movie
	Series   []Series // in Jellyfin's sort-name order
	LoadedAt time.Time
}

// PosterTag returns the poster tag of the movie, series or season with this
// ID, and false if the item isn't in the snapshot or has no poster of its
// own. It is how the dashboard limits poster requests to items it knows
// about.
func (s Snapshot) PosterTag(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	for _, m := range s.Movies {
		if m.ID == id {
			return m.PosterTag, m.PosterTag != ""
		}
	}
	for _, sr := range s.Series {
		if sr.ID == id {
			return sr.PosterTag, sr.PosterTag != ""
		}
		for _, se := range sr.Seasons {
			if se.PosterID == id {
				return se.PosterTag, se.PosterTag != ""
			}
		}
	}
	return "", false
}

// SeriesByID returns the series with this ID, and false if there is none.
func (s Snapshot) SeriesByID(id string) (Series, bool) {
	for _, sr := range s.Series {
		if sr.ID == id {
			return sr, true
		}
	}
	return Series{}, false
}

// Source is where the cache gets its data; *jellyfin.Client satisfies it.
type Source interface {
	Users(ctx context.Context) ([]jellyfin.User, error)
	Movies(ctx context.Context) ([]jellyfin.Item, error)
	PlayedMovieIDs(ctx context.Context, userID string) ([]string, error)
	Series(ctx context.Context) ([]jellyfin.Item, error)
	Seasons(ctx context.Context) ([]jellyfin.Item, error)
	Episodes(ctx context.Context) ([]jellyfin.Item, error)
	UserEpisodes(ctx context.Context, userID string) ([]jellyfin.Item, error)
}

// Options are the library settings from the environment.
type Options struct {
	// IncludeSpecials shows Season 0 (INCLUDE_SPECIALS). Specials count
	// towards a series' size either way.
	IncludeSpecials bool
}

// Cache holds the latest snapshot. Refresh builds a new snapshot and swaps it
// in whole, so readers never see partial data.
type Cache struct {
	src  Source
	opts Options
	snap atomic.Pointer[Snapshot]
	now  func() time.Time
}

// NewCache returns an empty cache backed by src.
func NewCache(src Source, opts Options) *Cache {
	return &Cache{src: src, opts: opts, now: time.Now}
}

// Snapshot returns the current snapshot, or false if none has loaded yet.
func (c *Cache) Snapshot() (Snapshot, bool) {
	s := c.snap.Load()
	if s == nil {
		return Snapshot{}, false
	}
	return *s, true
}

// Refresh fetches the library from the source and replaces the snapshot. On
// error the previous snapshot is kept.
func (c *Cache) Refresh(ctx context.Context) error {
	jfUsers, err := c.src.Users(ctx)
	if err != nil {
		return err
	}
	items, err := c.src.Movies(ctx)
	if err != nil {
		return err
	}
	users := make([]User, len(jfUsers))
	for i, u := range jfUsers {
		users[i] = User{ID: u.ID, Name: u.Name}
	}
	movies := make([]Movie, len(items))
	index := make(map[string]int, len(items))
	for i, it := range items {
		movies[i] = Movie{ID: it.ID, Title: it.Name, Year: it.ProductionYear, Added: it.DateCreated, Size: totalSize(it.MediaSources), PosterTag: it.PrimaryImageTag()}
		index[it.ID] = i
	}
	// Users are visited in /Users order, so each CompletedBy ends up in it too.
	for _, u := range users {
		ids, err := c.src.PlayedMovieIDs(ctx, u.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if i, ok := index[id]; ok {
				movies[i].CompletedBy = append(movies[i].CompletedBy, u)
			}
		}
	}
	series, err := fetchTV(ctx, c.src, users, c.opts)
	if err != nil {
		return err
	}
	c.snap.Store(&Snapshot{Users: users, Movies: movies, Series: series, LoadedAt: c.now()})
	return nil
}

// totalSize adds up the sizes of an item's versions (a movie's, or an
// episode's). If any version has no
// reported size the total is unknown (0), rather than a sum that silently
// leaves a file out.
func totalSize(sources []jellyfin.MediaSource) int64 {
	var total int64
	for _, s := range sources {
		if s.Size <= 0 {
			return 0
		}
		total += s.Size
	}
	return total
}
