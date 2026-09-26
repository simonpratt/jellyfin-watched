// Package artwork caches posters fetched from Jellyfin, so the dashboard can
// serve them to the browser without the browser ever needing the API key
// (REQUIREMENTS.md §9.2), and repeat views don't hit Jellyfin (§9.3).
//
// The cache doesn't decide which items may be fetched; the caller only asks
// for items in the library snapshot.
package artwork

import (
	"context"
	"sync"

	"jellyfin-watched/internal/jellyfin"
)

// Source fetches posters; *jellyfin.Client satisfies it.
type Source interface {
	PrimaryImage(ctx context.Context, itemID string) (jellyfin.Image, error)
}

// Cache holds one poster per item, in memory. A 96×144 poster is 5–15 KB, so
// even a 3,000-movie library stays within a few tens of MB.
type Cache struct {
	src Source
	mu  sync.Mutex
	m   map[string]*entry // by item ID
}

// entry is one item's poster. done is closed once the fetch has finished;
// until then img and err must not be read.
type entry struct {
	tag  string
	done chan struct{}
	img  jellyfin.Image
	err  error
}

// New returns an empty cache backed by src.
func New(src Source) *Cache {
	return &Cache{src: src, m: map[string]*entry{}}
}

// Poster returns the item's poster and its media type. tag is the poster's
// current Jellyfin image tag: a cached poster with a different tag is stale
// and is fetched again. Simultaneous requests for one poster share a fetch,
// and a failed fetch isn't cached.
func (c *Cache) Poster(ctx context.Context, itemID, tag string) ([]byte, string, error) {
	c.mu.Lock()
	e, ok := c.m[itemID]
	if !ok || e.tag != tag {
		e = &entry{tag: tag, done: make(chan struct{})}
		c.m[itemID] = e
		// The fetch outlives this request, since others may be waiting on
		// it; the client's timeout bounds it.
		go c.fetch(context.WithoutCancel(ctx), itemID, e)
	}
	c.mu.Unlock()

	select {
	case <-e.done:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	if e.err != nil {
		return nil, "", e.err
	}
	return e.img.Data, e.img.ContentType, nil
}

func (c *Cache) fetch(ctx context.Context, itemID string, e *entry) {
	e.img, e.err = c.src.PrimaryImage(ctx, itemID)
	if e.err != nil {
		c.mu.Lock()
		if c.m[itemID] == e {
			delete(c.m, itemID)
		}
		c.mu.Unlock()
	}
	close(e.done)
}
