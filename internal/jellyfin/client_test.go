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
	if r.URL.Path != "/Items" {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	start, _ := strconv.Atoi(q.Get("startIndex"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	end := len(f.movies)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	page := []Item{}
	if start < len(f.movies) {
		page = f.movies[start:end]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items":            page,
		"TotalRecordCount": len(f.movies),
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
		if !strings.Contains(","+q.Get("fields")+",", ",DateCreated,") {
			t.Errorf("request %d does not ask for DateCreated: %s", i, r.URL.RawQuery)
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
	want := []string{"Get", "Movies"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Client methods = %v, want %v", names, want)
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	if _, err := New("::not a url", testKey, time.Second); err == nil {
		t.Error("want error for bad URL")
	}
}
