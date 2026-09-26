package library

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

type fakeSource struct {
	movies []jellyfin.Item
	err    error
	calls  int
}

func (f *fakeSource) Movies(context.Context) ([]jellyfin.Item, error) {
	f.calls++
	return f.movies, f.err
}

func TestCacheEmptyUntilRefreshed(t *testing.T) {
	c := NewCache(&fakeSource{})
	if _, ok := c.Snapshot(); ok {
		t.Error("new cache reports a snapshot")
	}
}

func TestRefreshStoresSnapshot(t *testing.T) {
	added := time.Date(2023, 11, 9, 8, 0, 0, 0, time.UTC)
	src := &fakeSource{movies: []jellyfin.Item{
		{ID: "a", Name: "Alien", Type: "Movie", ProductionYear: 1979, DateCreated: added},
		{ID: "b", Name: "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", Type: "Movie", ProductionYear: 1964},
	}}
	c := NewCache(src)
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, ok := c.Snapshot()
	if !ok {
		t.Fatal("no snapshot after refresh")
	}
	want := []Movie{
		{ID: "a", Title: "Alien", Year: 1979, Added: added},
		{ID: "b", Title: "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb", Year: 1964},
	}
	if !reflect.DeepEqual(snap.Movies, want) || !snap.LoadedAt.Equal(now) {
		t.Errorf("snapshot = %+v", snap)
	}

	// Reading the snapshot does not touch the source.
	c.Snapshot()
	c.Snapshot()
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1", src.calls)
	}
}

func TestFailedRefreshKeepsPreviousSnapshot(t *testing.T) {
	src := &fakeSource{movies: []jellyfin.Item{{ID: "a", Name: "Alien"}}}
	c := NewCache(src)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	src.err = errors.New("boom")
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("want error")
	}
	snap, ok := c.Snapshot()
	if !ok || len(snap.Movies) != 1 {
		t.Errorf("previous snapshot lost: %+v %v", snap, ok)
	}
}
