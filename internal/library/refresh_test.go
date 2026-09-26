package library

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

// gatedSource serves one movie per refresh, named after the refresh number.
// While gate is set, each refresh blocks in Users until the gate is closed,
// so tests can look at the cache mid-refresh.
type gatedSource struct {
	mu      sync.Mutex
	gate    chan struct{}
	started chan struct{} // receives once per refresh that reaches Users
	err     error
	n       atomic.Int32 // refreshes started
	running atomic.Int32
	overlap atomic.Bool // two refreshes ran at once
}

func (g *gatedSource) Users(ctx context.Context) ([]jellyfin.User, error) {
	g.n.Add(1)
	if g.running.Add(1) > 1 {
		g.overlap.Store(true)
	}
	g.mu.Lock()
	gate, started, err := g.gate, g.started, g.err
	g.mu.Unlock()
	if started != nil {
		started <- struct{}{}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			g.running.Add(-1)
			return nil, ctx.Err()
		}
	}
	if err != nil {
		g.running.Add(-1)
		return nil, err
	}
	return []jellyfin.User{{ID: "u", Name: "sally"}}, nil
}

func (g *gatedSource) Movies(context.Context) ([]jellyfin.Item, error) {
	defer g.running.Add(-1)
	return []jellyfin.Item{{ID: "m", Name: "Refresh " + string(rune('0'+g.n.Load()))}}, nil
}

func (g *gatedSource) PlayedMovieIDs(context.Context, string) ([]string, error) { return nil, nil }
func (g *gatedSource) Series(context.Context) ([]jellyfin.Item, error)          { return nil, nil }
func (g *gatedSource) Seasons(context.Context) ([]jellyfin.Item, error)         { return nil, nil }
func (g *gatedSource) Episodes(context.Context) ([]jellyfin.Item, error)        { return nil, nil }
func (g *gatedSource) UserEpisodes(context.Context, string) ([]jellyfin.Item, error) {
	return nil, nil
}

func (g *gatedSource) setGate(gate chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gate = gate
}

func (g *gatedSource) setErr(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.err = err
}

// waitStatus polls the cache until ok returns true.
func waitStatus(t *testing.T, c *Cache, what string, ok func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := c.Status()
		if ok(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; status %+v", what, st)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStatusBeforeFirstLoad(t *testing.T) {
	st := NewCache(&gatedSource{}, Options{}).Status()
	if st.Loaded || st.Refreshing || st.Failed || st.Generation != 0 {
		t.Errorf("new cache status = %+v", st)
	}
}

func TestSuccessfulRefreshUpdatesStatus(t *testing.T) {
	c := NewCache(&gatedSource{}, Options{})
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	if !st.Loaded || st.Failed || st.Refreshing || st.Generation != 1 || !st.Snapshot.LoadedAt.Equal(now) {
		t.Errorf("status = %+v", st)
	}
	if len(st.Snapshot.Movies) != 1 || st.Snapshot.Movies[0].Title != "Refresh 1" {
		t.Errorf("movies = %+v", st.Snapshot.Movies)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := c.Status(); st.Generation != 2 || st.Snapshot.Movies[0].Title != "Refresh 2" {
		t.Errorf("after second refresh: %+v", st)
	}
}

func TestFailedRefreshKeepsDataAndReportsFailure(t *testing.T) {
	src := &gatedSource{}
	c := NewCache(src, Options{})
	loaded := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return loaded }
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	src.setErr(errors.New("jellyfin: GET /Users: connection refused"))
	failed := loaded.Add(3 * time.Hour)
	c.now = func() time.Time { return failed }
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("want error")
	}
	st := c.Status()
	if !st.Loaded || !st.Failed || st.Generation != 1 || !st.Snapshot.LoadedAt.Equal(loaded) || !st.FailedAt.Equal(failed) {
		t.Errorf("status = %+v", st)
	}
	if len(st.Snapshot.Movies) != 1 || st.Snapshot.Movies[0].Title != "Refresh 1" {
		t.Errorf("last good data lost: %+v", st.Snapshot.Movies)
	}
	// Recovering clears the failure.
	src.setErr(nil)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := c.Status(); st.Failed || st.Generation != 2 {
		t.Errorf("after recovery: %+v", st)
	}
}

func TestFailedFirstLoad(t *testing.T) {
	src := &gatedSource{err: errors.New("boom")}
	c := NewCache(src, Options{})
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if st := c.Status(); st.Loaded || !st.Failed || st.Generation != 0 {
		t.Errorf("status = %+v", st)
	}
}

// Pages read the old snapshot, whole, until the new one is complete.
func TestRefreshSwapsSnapshotWhole(t *testing.T) {
	src := &gatedSource{}
	c := NewCache(src, Options{})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	src.setGate(gate)
	done := make(chan error)
	go func() { done <- c.Refresh(context.Background()) }()
	waitStatus(t, c, "refreshing", func(s Status) bool { return s.Refreshing })
	st := c.Status()
	if st.Generation != 1 || len(st.Snapshot.Movies) != 1 || st.Snapshot.Movies[0].Title != "Refresh 1" || len(st.Snapshot.Users) != 1 {
		t.Errorf("mid-refresh status = %+v", st)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if st := c.Status(); st.Refreshing || st.Snapshot.Movies[0].Title != "Refresh 2" {
		t.Errorf("after refresh: %+v", st)
	}
}

func TestRunLoadsAtOnceThenWaitsTheInterval(t *testing.T) {
	src := &gatedSource{}
	c := NewCache(src, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Hour, slog.New(slog.DiscardHandler)); close(done) }()
	waitStatus(t, c, "first load", func(s Status) bool { return s.Loaded })
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if n := src.n.Load(); n != 1 {
		t.Errorf("%d refreshes within the hour, want 1", n)
	}
}

func TestRunRefreshesEveryInterval(t *testing.T) {
	src := &gatedSource{}
	c := NewCache(src, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, 20*time.Millisecond, slog.New(slog.DiscardHandler)); close(done) }()
	waitStatus(t, c, "three refreshes", func(s Status) bool { return s.Generation >= 3 })
	cancel()
	<-done
}

// "Refresh now" starts one refresh; clicks while it runs don't start more.
func TestRefreshNowIsSingleFlight(t *testing.T) {
	src := &gatedSource{}
	c := NewCache(src, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx, time.Hour, slog.New(slog.DiscardHandler))
	waitStatus(t, c, "first load", func(s Status) bool { return s.Loaded && !s.Refreshing })

	gate := make(chan struct{})
	src.setGate(gate)
	if !c.RefreshNow() {
		t.Fatal("RefreshNow did not start a refresh")
	}
	// The status says so at once, before the refresh has even begun.
	if !c.Status().Refreshing {
		t.Error("not refreshing straight after RefreshNow")
	}
	var wg sync.WaitGroup
	var started atomic.Int32
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.RefreshNow() {
				started.Add(1)
			}
		}()
	}
	wg.Wait()
	if started.Load() != 0 {
		t.Errorf("%d concurrent clicks started a refresh", started.Load())
	}
	close(gate)
	waitStatus(t, c, "refresh done", func(s Status) bool { return s.Generation == 2 && !s.Refreshing })
	if n := src.n.Load(); n != 2 || src.overlap.Load() {
		t.Errorf("%d refreshes (want 2), overlap %v", n, src.overlap.Load())
	}
	// Once it has finished, another click starts another.
	src.setGate(nil)
	if !c.RefreshNow() {
		t.Error("RefreshNow after a finished refresh did not start one")
	}
	waitStatus(t, c, "third refresh", func(s Status) bool { return s.Generation == 3 && !s.Refreshing })
}

func TestRunLogsFailuresAndLoads(t *testing.T) {
	src := &gatedSource{err: errors.New("jellyfin: GET /Users: unexpected status 401 Unauthorized")}
	c := NewCache(src, Options{})
	var buf syncBuf
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, time.Hour, slog.New(slog.NewTextHandler(&buf, nil))); close(done) }()
	waitStatus(t, c, "failure", func(s Status) bool { return s.Failed && !s.Refreshing })
	src.setErr(nil)
	c.RefreshNow()
	waitStatus(t, c, "load", func(s Status) bool { return s.Loaded })
	cancel()
	<-done
	logs := buf.String()
	for _, want := range []string{"library refresh failed", "401", "library loaded", "movies=1"} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs missing %q:\n%s", want, logs)
		}
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
