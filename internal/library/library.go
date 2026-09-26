// Package library holds the in-memory snapshot of the Jellyfin library that
// pages read from. Pages never call Jellyfin directly (REQUIREMENTS.md §9.3).
package library

import (
	"context"
	"sync/atomic"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

// Movie is one movie in the library.
type Movie struct {
	ID    string
	Title string
	Year  int       // 0 when Jellyfin has no production year
	Added time.Time // when it was added to the library; zero if unknown
}

// Snapshot is a complete, immutable view of the library at one moment.
type Snapshot struct {
	Movies   []Movie
	LoadedAt time.Time
}

// Source is where the cache gets its data; *jellyfin.Client satisfies it.
type Source interface {
	Movies(ctx context.Context) ([]jellyfin.Item, error)
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
	items, err := c.src.Movies(ctx)
	if err != nil {
		return err
	}
	movies := make([]Movie, 0, len(items))
	for _, it := range items {
		movies = append(movies, Movie{ID: it.ID, Title: it.Name, Year: it.ProductionYear, Added: it.DateCreated})
	}
	c.snap.Store(&Snapshot{Movies: movies, LoadedAt: c.now()})
	return nil
}
