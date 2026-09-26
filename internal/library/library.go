// Package library holds the in-memory snapshot of the Jellyfin library that
// pages read from. Pages never call Jellyfin directly (REQUIREMENTS.md §9.3).
package library

import (
	"context"
	"log/slog"
	"sync"
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

// Cache holds the latest snapshot and the state of its refreshes. Refresh
// builds a new snapshot and swaps it in whole, so readers never see partial
// data.
type Cache struct {
	src  Source
	opts Options
	now  func() time.Time

	refreshing sync.Mutex // held for the whole of a refresh: one at a time
	wake       chan struct{}

	mu     sync.Mutex // guards status
	status Status
}

// Status is the cache's current snapshot and what its refreshes are doing.
type Status struct {
	Snapshot Snapshot // the last good snapshot; zero until Loaded
	Loaded   bool     // a refresh has succeeded at least once
	// Generation counts successful refreshes, so a page can tell whether
	// the data it shows has been replaced.
	Generation int
	Refreshing bool // a refresh is running or about to start
	// Failed reports that the latest refresh failed; the last good
	// snapshot, if any, is kept. FailedAt is when it failed.
	Failed   bool
	FailedAt time.Time
}

// NewCache returns an empty cache backed by src.
func NewCache(src Source, opts Options) *Cache {
	return &Cache{src: src, opts: opts, now: time.Now, wake: make(chan struct{}, 1)}
}

// Snapshot returns the current snapshot, or false if none has loaded yet.
func (c *Cache) Snapshot() (Snapshot, bool) {
	st := c.Status()
	return st.Snapshot, st.Loaded
}

// Status returns the current snapshot and refresh state.
func (c *Cache) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Run refreshes the cache straight away, then again every interval after
// the previous refresh finished, until ctx is done. RefreshNow starts one
// sooner. Each outcome is logged; errors from the Jellyfin client never
// contain the API key.
func (c *Cache) Run(ctx context.Context, every time.Duration, log *slog.Logger) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-c.wake:
		}
		start := time.Now()
		err := c.Refresh(ctx)
		took := time.Since(start).Round(time.Millisecond)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			log.Error("library refresh failed", "err", err, "took", took)
		default:
			st := c.Status()
			log.Info("library loaded", "movies", len(st.Snapshot.Movies), "series", len(st.Snapshot.Series), "took", took)
		}
		// Timers from Go 1.23 on can be reset without draining.
		timer.Reset(every)
	}
}

// RefreshNow asks Run to refresh straight away and reports whether it will.
// It returns false, starting nothing, while a refresh is already running,
// so repeated clicks never start concurrent refreshes. The status shows
// Refreshing from the moment it returns true.
func (c *Cache) RefreshNow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.Refreshing {
		return false
	}
	c.status.Refreshing = true
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return true
}

// Refresh fetches the library from the source and replaces the snapshot. On
// error the previous snapshot is kept and the status reports the failure.
// Refreshes run one at a time.
func (c *Cache) Refresh(ctx context.Context) error {
	c.refreshing.Lock()
	defer c.refreshing.Unlock()
	c.mu.Lock()
	c.status.Refreshing = true
	c.mu.Unlock()

	snap, err := c.load(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Refreshing = false
	if err != nil {
		c.status.Failed, c.status.FailedAt = true, c.now()
		return err
	}
	c.status.Snapshot, c.status.Loaded, c.status.Failed, c.status.FailedAt = *snap, true, false, time.Time{}
	c.status.Generation++
	return nil
}

// load fetches the whole library into a new snapshot. Nothing is shared
// with the current one, which pages go on reading until it is swapped.
func (c *Cache) load(ctx context.Context) (*Snapshot, error) {
	jfUsers, err := c.src.Users(ctx)
	if err != nil {
		return nil, err
	}
	items, err := c.src.Movies(ctx)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		for _, id := range ids {
			if i, ok := index[id]; ok {
				movies[i].CompletedBy = append(movies[i].CompletedBy, u)
			}
		}
	}
	series, err := fetchTV(ctx, c.src, users, c.opts)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Users: users, Movies: movies, Series: series, LoadedAt: c.now()}, nil
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
