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
	LoadedAt time.Time
}

// PosterTag returns the poster tag of the item with this ID, and false if
// the item isn't in the snapshot or has no poster. It is how the dashboard
// limits poster requests to items it knows about.
func (s Snapshot) PosterTag(id string) (string, bool) {
	for _, m := range s.Movies {
		if m.ID == id {
			return m.PosterTag, m.PosterTag != ""
		}
	}
	return "", false
}

// Source is where the cache gets its data; *jellyfin.Client satisfies it.
type Source interface {
	Users(ctx context.Context) ([]jellyfin.User, error)
	Movies(ctx context.Context) ([]jellyfin.Item, error)
	PlayedMovieIDs(ctx context.Context, userID string) ([]string, error)
}

// Cache holds the latest snapshot. Refresh builds a new snapshot and swaps it
// in whole, so readers never see partial data.
type Cache struct {
	src  Source
	snap atomic.Pointer[Snapshot]
	now  func() time.Time
}

// NewCache returns an empty cache backed by src.
func NewCache(src Source) *Cache {
	return &Cache{src: src, now: time.Now}
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
	c.snap.Store(&Snapshot{Users: users, Movies: movies, LoadedAt: c.now()})
	return nil
}

// totalSize adds up the sizes of a movie's versions. If any version has no
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
