package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"jellyfin-watched/internal/library"
)

// stubLibrary is a library whose status the test sets, recording "Refresh
// now" clicks. RefreshNow behaves like the cache's: it starts a refresh
// only when none is running.
type stubLibrary struct {
	mu      sync.Mutex
	status  library.Status
	clicks  int
	started int
}

func (s *stubLibrary) Status() library.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *stubLibrary) RefreshNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clicks++
	if s.status.Refreshing {
		return false
	}
	s.started++
	s.status.Refreshing = true
	return true
}

func (s *stubLibrary) set(f func(*library.Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.status)
}

// testNow is the clock every refresh test runs at: 14:42 on 26 Sep 2026.
var testNow = time.Date(2026, 9, 26, 14, 42, 0, 0, time.UTC)

func newClockedHandler(t *testing.T, lib Library) http.Handler {
	t.Helper()
	h, err := newHandler(lib, &stubPosters{}, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return testNow })
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(t *testing.T, h http.Handler, method, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loadedLibrary(ago time.Duration, movies ...library.Movie) *stubLibrary {
	return &stubLibrary{status: library.Status{Loaded: true, Generation: 3,
		Snapshot: library.Snapshot{Movies: movies, LoadedAt: testNow.Add(-ago)}}}
}

func assertContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func assertNotContains(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, s := range unwanted {
		if strings.Contains(body, s) {
			t.Errorf("unexpectedly contains %s", s)
		}
	}
}

func TestFirstLoadShowsLoadingStateOnEveryPage(t *testing.T) {
	h := newClockedHandler(t, &stubLibrary{status: library.Status{Refreshing: true}})
	for _, path := range []string{"/movies", "/tv"} {
		body := do(t, h, "GET", path, nil).Body.String()
		assertContains(t, body,
			`<h2 class="empty-state__headline">Loading your library. This can take a minute on first start.</h2>`,
			`<p class="empty-state__line">The page will show your movies and TV shows as soon as Jellyfin has sent them.</p>`,
			`class="empty-state empty-state--loading"`,
			// The header's hairline slides, and the control checks back
			// often so the page updates once the data arrives.
			`<span class="refresh-control__hairline" aria-hidden="true"></span>`,
			`hx-get="/refresh?since=g0" hx-trigger="every 2s"`,
		)
		// Nothing to refresh or date yet, and no list furniture.
		assertNotContains(t, body, "Refresh now", "Updated", "page-head", "list-head")
	}
}

func TestLoadedPageShowsUpdatedAgoAndRefreshButton(t *testing.T) {
	h := newClockedHandler(t, loadedLibrary(12*time.Minute+30*time.Second, library.Movie{ID: "a", Title: "Alien"}))
	body := do(t, h, "GET", "/movies", nil).Body.String()
	assertContains(t, body,
		`<div class="app-header__actions"><div class="refresh-control" id="refresh-control" hx-get="/refresh?since=g3" hx-trigger="every 30s" hx-swap="outerHTML">`,
		`<span class="refresh-control__updated">Updated 12 minutes ago</span>`,
		`<button type="button" id="refresh-now" class="button" hx-post="/refresh" hx-target="#refresh-control" hx-swap="outerHTML">`,
		`Refresh now</button>`,
		"Alien",
	)
	assertNotContains(t, body, "refresh-control__hairline", "Refreshing…", "notice", "Loading your library")
}

// "Refresh now" starts a refresh and swaps in the busy control; clicks
// while it runs start nothing more. The list isn't touched.
func TestRefreshNowShowsRefreshingAndIgnoresRepeatClicks(t *testing.T) {
	lib := loadedLibrary(12*time.Minute, library.Movie{ID: "a", Title: "Alien"})
	h := newClockedHandler(t, lib)
	for i := range 3 {
		rec := do(t, h, "POST", "/refresh", map[string]string{"HX-Request": "true"})
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %d: status %d", i, rec.Code)
		}
		body := rec.Body.String()
		if !strings.HasPrefix(body, `<div class="refresh-control" id="refresh-control"`) {
			t.Errorf("POST %d: not a refresh-control fragment:\n%s", i, body)
		}
		assertContains(t, body,
			`<span class="refresh-control__updated">Updated 12 minutes ago</span>`,
			`<button type="button" id="refresh-now" class="button button--busy" aria-disabled="true">`,
			`button__icon--spin`,
			`Refreshing…</button>`,
			`<span class="refresh-control__hairline" aria-hidden="true"></span>`,
			`hx-trigger="every 2s"`,
		)
		assertNotContains(t, body, "hx-post", "Alien", "<html")
	}
	if lib.clicks != 3 || lib.started != 1 {
		t.Errorf("clicks %d, refreshes started %d; want 3 and 1", lib.clicks, lib.started)
	}
	if rec := do(t, h, "GET", "/movies", nil); !strings.Contains(rec.Body.String(), "Refreshing…") {
		t.Error("a page loaded mid-refresh doesn't show it")
	}
}

// The control polls for its own state. Once the data the page shows has
// been replaced, or a refresh has failed, it tells the page to reload
// itself (HX-Trigger), which swaps in the new rows.
func TestRefreshPollTellsThePageWhenDataChanges(t *testing.T) {
	lib := loadedLibrary(0, library.Movie{ID: "a", Title: "Alien"})
	lib.status.Refreshing = true
	h := newClockedHandler(t, lib)

	rec := do(t, h, "GET", "/refresh?since=g3", map[string]string{"HX-Request": "true"})
	if rec.Header().Get("HX-Trigger") != "" || !strings.Contains(rec.Body.String(), "Refreshing…") {
		t.Errorf("mid-refresh poll: HX-Trigger %q body %s", rec.Header().Get("HX-Trigger"), rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	lib.set(func(s *library.Status) { s.Refreshing, s.Generation, s.Snapshot.LoadedAt = false, 4, testNow })
	rec = do(t, h, "GET", "/refresh?since=g3", map[string]string{"HX-Request": "true"})
	if rec.Header().Get("HX-Trigger") != "library-updated" {
		t.Errorf("after refresh: HX-Trigger %q", rec.Header().Get("HX-Trigger"))
	}
	assertContains(t, rec.Body.String(), "Updated less than a minute ago", "Refresh now", `hx-get="/refresh?since=g4"`)

	rec = do(t, h, "GET", "/refresh?since=g4", map[string]string{"HX-Request": "true"})
	if rec.Header().Get("HX-Trigger") != "" {
		t.Errorf("unchanged: HX-Trigger %q", rec.Header().Get("HX-Trigger"))
	}

	lib.set(func(s *library.Status) { s.Failed, s.FailedAt = true, testNow })
	rec = do(t, h, "GET", "/refresh?since=g4", map[string]string{"HX-Request": "true"})
	if rec.Header().Get("HX-Trigger") != "library-updated" {
		t.Errorf("after a failure: HX-Trigger %q", rec.Header().Get("HX-Trigger"))
	}
	assertContains(t, rec.Body.String(), `hx-get="/refresh?since=g4f"`)
}

// The page wraps its header and content in #app, which reloads itself from
// the current URL when told the data has changed.
func TestPageReloadsItselfWhenLibraryUpdates(t *testing.T) {
	h := newClockedHandler(t, loadedLibrary(time.Minute))
	for _, path := range []string{"/movies", "/tv"} {
		body := do(t, h, "GET", path, nil).Body.String()
		assertContains(t, body, `<div id="app" class="app" hx-get="" hx-trigger="library-updated from:body" hx-select="#app" hx-target="this" hx-swap="outerHTML" hx-disinherit="*">`)
		// The reload asks for the whole page, even of /tv, whose other htmx
		// requests get the content alone.
		full := do(t, h, "GET", path, map[string]string{"HX-Request": "true", "HX-Target": "app"}).Body.String()
		assertContains(t, full, `<div id="app"`, `class="app-header"`)
	}
	frag := do(t, h, "GET", "/tv?seasons=all", map[string]string{"HX-Request": "true"}).Body.String()
	assertNotContains(t, frag, `<div id="app"`)
}

func TestStaleDataKeepsListWithNotice(t *testing.T) {
	lib := loadedLibrary(3*time.Hour, library.Movie{ID: "a", Title: "Alien"})
	lib.status.Failed, lib.status.FailedAt = true, testNow
	h := newClockedHandler(t, lib)
	body := do(t, h, "GET", "/movies", nil).Body.String()
	assertContains(t, body,
		`<p class="notice__headline">Can&#39;t reach Jellyfin. Showing data from 3 hours ago.</p>`,
		`<p class="notice__detail">Last updated today at 11:42. Refresh now tries the connection again.</p>`,
		`<span class="refresh-control__updated">Updated 3 hours ago</span>`,
		"Refresh now</button>",
		"Alien",
	)
	// The notice sits between the page head and the list.
	if i, j, k := strings.Index(body, "page-head"), strings.Index(body, `class="notice"`), strings.Index(body, "list-head"); !(i < j && j < k) {
		t.Errorf("notice not between page head and list (%d, %d, %d)", i, j, k)
	}
	tv := do(t, h, "GET", "/tv", nil).Body.String()
	assertContains(t, tv, "Can&#39;t reach Jellyfin. Showing data from 3 hours ago.")
}

func TestFailedFirstLoad(t *testing.T) {
	h := newClockedHandler(t, &stubLibrary{status: library.Status{Failed: true, FailedAt: testNow}})
	body := do(t, h, "GET", "/movies", nil).Body.String()
	assertContains(t, body,
		`<p class="notice__headline">Can&#39;t reach Jellyfin, so the library hasn&#39;t loaded yet.</p>`,
		`<p class="notice__detail">Refresh now tries the connection again.</p>`,
		"Refresh now</button>",
		`hx-get="/refresh?since=g0f"`,
	)
	assertNotContains(t, body, "Loading your library", "Updated", "refresh-control__hairline")
}

func TestEmptyLibraryStates(t *testing.T) {
	h := newClockedHandler(t, loadedLibrary(12*time.Minute))
	movies := do(t, h, "GET", "/movies", nil).Body.String()
	assertContains(t, movies,
		`<h2 class="empty-state__headline">There are no movies on this server</h2>`,
		`<p class="empty-state__line">Jellyfin reported no movies at the last refresh, 12 minutes ago.</p>`,
		`<p class="empty-state__line">Movies added in Jellyfin appear here after the next refresh.</p>`,
		`<p class="page-head__summary">0 movies</p>`,
	)
	tv := do(t, h, "GET", "/tv", nil).Body.String()
	assertContains(t, tv,
		`<h2 class="empty-state__headline">There are no TV shows on this server</h2>`,
		`<p class="empty-state__line">Jellyfin reported no series at the last refresh, 12 minutes ago.</p>`,
		`<p class="empty-state__line">Shows added in Jellyfin appear here after the next refresh.</p>`,
		`<p class="page-head__summary">0 series</p>`,
	)
	assertNotContains(t, movies+tv, "list-head", "empty-state--loading")
}
