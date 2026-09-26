package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "test-api-key-0123456789"

// fakeJellyfin serves /Items like the real API, paginated by startIndex and
// limit, and records every request it receives.
type fakeJellyfin struct {
	mu       sync.Mutex
	requests []*http.Request
	movies   []Item
	users    []User
	played   map[string][]Item // by user ID
	status   int
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(context.Background()))
	f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/Users":
		// Real Jellyfin returns a bare JSON array here, not {Items: …}.
		_ = json.NewEncoder(w).Encode(f.users)
		return
	case "/Items":
	default:
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	items := f.movies
	if uid := q.Get("userId"); uid != "" {
		if q.Get("isPlayed") != "true" {
			http.Error(w, "fake only serves played items per user", http.StatusBadRequest)
			return
		}
		items = f.played[uid]
	} else if q.Get("isPlayed") != "" {
		// Jellyfin 12.1.0 answers isPlayed without userId with a 500.
		http.Error(w, "Error processing request.", http.StatusInternalServerError)
		return
	}
	start, _ := strconv.Atoi(q.Get("startIndex"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	page := []Item{}
	if start < len(items) {
		page = items[start:end]
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items":            page,
		"TotalRecordCount": len(items),
		"StartIndex":       start,
	})
}

func movies(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: fmt.Sprintf("id%d", i), Name: fmt.Sprintf("Movie %d", i), Type: "Movie", ProductionYear: 2000 + i,
			DateCreated: time.Date(2024, time.Month(i%12+1), 14, 10, 22, 33, 786570500, time.UTC)}
	}
	return out
}

func newTestClient(t *testing.T, f *fakeJellyfin) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, testKey, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMoviesPaginatesAndSendsOnlyAuthorizedGETs(t *testing.T) {
	f := &fakeJellyfin{movies: movies(7)}
	c := newTestClient(t, f)
	c.pageSize = 3

	got, err := c.Movies(context.Background())
	if err != nil {
		t.Fatalf("Movies: %v", err)
	}
	if !reflect.DeepEqual(got, f.movies) {
		t.Errorf("Movies = %+v, want %+v", got, f.movies)
	}
	if len(f.requests) != 3 {
		t.Errorf("made %d requests, want 3 pages", len(f.requests))
	}
	for i, r := range f.requests {
		if r.Method != http.MethodGet {
			t.Errorf("request %d method = %s, want GET", i, r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "MediaBrowser Token="+testKey {
			t.Errorf("request %d Authorization = %q", i, got)
		}
		q := r.URL.Query()
		if q.Get("includeItemTypes") != "Movie" || q.Get("recursive") != "true" {
			t.Errorf("request %d query = %s", i, r.URL.RawQuery)
		}
		for _, field := range []string{"DateCreated", "MediaSources"} {
			if !strings.Contains(","+q.Get("fields")+",", ","+field+",") {
				t.Errorf("request %d does not ask for %s: %s", i, field, r.URL.RawQuery)
			}
		}
		if q.Get("startIndex") != strconv.Itoa(i*3) || q.Get("limit") != "3" {
			t.Errorf("request %d pagination = %s", i, r.URL.RawQuery)
		}
		if strings.Contains(r.URL.String(), testKey) {
			t.Errorf("API key sent in URL: %s", r.URL)
		}
	}
}

// Jellyfin 12.1.0 only includes DateCreated when fields=DateCreated is asked
// for, and sends it with seven fractional digits.
func TestItemDecodesJellyfinDateCreated(t *testing.T) {
	var it Item
	if err := json.Unmarshal([]byte(`{"Id":"x","DateCreated":"2026-04-05T21:22:25.7865705Z"}`), &it); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 5, 21, 22, 25, 786570500, time.UTC)
	if !it.DateCreated.Equal(want) {
		t.Errorf("DateCreated = %v, want %v", it.DateCreated, want)
	}
	if err := json.Unmarshal([]byte(`{"Id":"y"}`), &it); err != nil {
		t.Fatal(err)
	}
}

func TestMoviesEmptyLibrary(t *testing.T) {
	c := newTestClient(t, &fakeJellyfin{})
	got, err := c.Movies(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("Movies = %v, %v; want empty, nil", got, err)
	}
}

func TestErrorsDoNotContainAPIKey(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		c := newTestClient(t, &fakeJellyfin{status: status})
		_, err := c.Movies(context.Background())
		if err == nil {
			t.Fatalf("status %d: want error", status)
		}
		if !strings.Contains(err.Error(), strconv.Itoa(status)) {
			t.Errorf("error %q does not mention status %d", err, status)
		}
		if strings.Contains(err.Error(), testKey) {
			t.Errorf("error leaks API key: %q", err)
		}
	}

	// Unreachable server.
	c, _ := New("http://127.0.0.1:1", testKey, time.Second)
	_, err := c.Movies(context.Background())
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Errorf("unreachable: err = %v", err)
	}
}

// The read-only guarantee (REQUIREMENTS.md §9.1): the client has no way to
// send anything but GET. Its only request primitive is Get; every other
// exported method is a typed query built on it. Adding a method here means
// checking it goes through Get.
func TestClientExposesOnlyGET(t *testing.T) {
	typ := reflect.TypeOf(&Client{})
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)
	want := []string{"Get", "Movies", "PlayedMovieIDs", "Users"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Client methods = %v, want %v", names, want)
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	if _, err := New("::not a url", testKey, time.Second); err == nil {
		t.Error("want error for bad URL")
	}
}

func TestUsersInJellyfinOrder(t *testing.T) {
	f := &fakeJellyfin{users: []User{{ID: "u2", Name: "sally"}, {ID: "u1", Name: "Anastasia Kowalczyk"}, {ID: "u3", Name: "H"}}}
	got, err := newTestClient(t, f).Users(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, f.users) {
		t.Errorf("Users = %+v, want %+v", got, f.users)
	}
	r := f.requests[0]
	if r.Method != http.MethodGet || r.URL.Path != "/Users" || r.Header.Get("Authorization") != "MediaBrowser Token="+testKey {
		t.Errorf("request = %s %s, Authorization %q", r.Method, r.URL, r.Header.Get("Authorization"))
	}
}

func TestPlayedMovieIDsUsesIsPlayedOnlyAndPaginates(t *testing.T) {
	played := movies(5)
	f := &fakeJellyfin{played: map[string][]Item{"u1": played}}
	c := newTestClient(t, f)
	c.pageSize = 2

	got, err := c.PlayedMovieIDs(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"id0", "id1", "id2", "id3", "id4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PlayedMovieIDs = %v, want %v", got, want)
	}
	if len(f.requests) != 3 {
		t.Errorf("made %d requests, want 3 pages", len(f.requests))
	}
	for i, r := range f.requests {
		q := r.URL.Query()
		if r.Method != http.MethodGet || q.Get("userId") != "u1" || q.Get("isPlayed") != "true" ||
			q.Get("includeItemTypes") != "Movie" || q.Get("recursive") != "true" {
			t.Errorf("request %d = %s %s", i, r.Method, r.URL.RawQuery)
		}
		// Never both IsPlayed and IsUnplayed in filters: Jellyfin applies
		// only one of them (REQUIREMENTS.md §5).
		if q.Has("filters") || q.Has("isUnplayed") {
			t.Errorf("request %d uses filters: %s", i, r.URL.RawQuery)
		}
	}

	none, err := c.PlayedMovieIDs(context.Background(), "nobody")
	if err != nil || len(none) != 0 {
		t.Errorf("PlayedMovieIDs(nobody) = %v, %v; want empty, nil", none, err)
	}
}

// Shapes confirmed against Jellyfin 12.1.0: each MediaSources entry carries
// Size in bytes as a JSON integer; a missing Size decodes as unknown.
func TestItemDecodesMediaSourceSizes(t *testing.T) {
	var it Item
	body := `{"Id":"x","MediaSources":[{"Id":"a","Size":20559445148},{"Id":"b","Size":1024},{"Id":"c"}]}`
	if err := json.Unmarshal([]byte(body), &it); err != nil {
		t.Fatal(err)
	}
	if len(it.MediaSources) != 3 || it.MediaSources[0].Size != 20559445148 || it.MediaSources[1].Size != 1024 || it.MediaSources[2].Size != 0 {
		t.Errorf("MediaSources = %+v", it.MediaSources)
	}
}
