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
	// byType serves requests without userId by includeItemTypes (Series,
	// Season, Episode); anything else gets movies.
	byType map[string][]Item
	// userEpisodes serves each user's episodes with their UserData.
	userEpisodes map[string][]Item
	status       int
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
	if typed, ok := f.byType[q.Get("includeItemTypes")]; ok {
		items = typed
	}
	if uid := q.Get("userId"); uid != "" && q.Get("includeItemTypes") == "Episode" {
		items = f.userEpisodes[uid]
	} else if uid != "" {
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
// send anything but GET. Its only request primitive is the unexported get,
// which hard-codes GET; every exported method is built on it. Adding a method
// here means checking it goes through get.
func TestClientExposesOnlyGET(t *testing.T) {
	typ := reflect.TypeOf(&Client{})
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)
	want := []string{"Episodes", "Get", "Movies", "PlayedMovieIDs", "PrimaryImage", "Seasons", "Series", "UserEpisodes", "Users"}
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

// Confirmed against Jellyfin 12.1.0: items carry ImageTags by default, a map
// from image type to a tag that changes whenever the image does. An item with
// no poster has no "Primary" key.
func TestItemDecodesPrimaryImageTag(t *testing.T) {
	var it Item
	body := `{"Id":"x","ImageTags":{"Primary":"7916c0cf17fa49b8873fbaf2793afd27","Logo":"087a"},"BackdropImageTags":["b"]}`
	if err := json.Unmarshal([]byte(body), &it); err != nil {
		t.Fatal(err)
	}
	if got := it.PrimaryImageTag(); got != "7916c0cf17fa49b8873fbaf2793afd27" {
		t.Errorf("PrimaryImageTag = %q", got)
	}
	if err := json.Unmarshal([]byte(`{"Id":"y","ImageTags":{"Logo":"a"}}`), &it); err != nil {
		t.Fatal(err)
	}
	if got := (Item{}).PrimaryImageTag(); got != "" {
		t.Errorf("no ImageTags: PrimaryImageTag = %q", got)
	}
}

// posterServer serves /Items/{id}/Images/Primary like Jellyfin 12.1.0 and
// records each request.
func posterServer(t *testing.T, handler http.HandlerFunc) (*Client, *[]*http.Request) {
	t.Helper()
	var mu sync.Mutex
	var reqs []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.Clone(context.Background()))
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, testKey, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c, &reqs
}

func TestPrimaryImageFetchesSizedPosterWithGET(t *testing.T) {
	jpeg := []byte("\xff\xd8\xff\xe0 fake jpeg")
	c, reqs := posterServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items/abc123/Images/Primary" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Disposition", "attachment")
		_, _ = w.Write(jpeg)
	})
	img, err := c.PrimaryImage(context.Background(), "abc123", 96, 144)
	if err != nil {
		t.Fatal(err)
	}
	if string(img.Data) != string(jpeg) || img.ContentType != "image/jpeg" {
		t.Errorf("image = %q %q", img.ContentType, img.Data)
	}
	r := (*reqs)[0]
	if r.Method != http.MethodGet || r.Header.Get("Authorization") != "MediaBrowser Token="+testKey {
		t.Errorf("request = %s, Authorization %q", r.Method, r.Header.Get("Authorization"))
	}
	if strings.Contains(r.URL.String(), testKey) {
		t.Errorf("API key sent in URL: %s", r.URL)
	}
	// The size the caller asked for.
	q := r.URL.Query()
	if q.Get("fillWidth") != "96" || q.Get("fillHeight") != "144" {
		t.Errorf("query = %s", r.URL.RawQuery)
	}

	if _, err := c.PrimaryImage(context.Background(), "abc123", 320, 480); err != nil {
		t.Fatal(err)
	}
	if q := (*reqs)[1].URL.Query(); q.Get("fillWidth") != "320" || q.Get("fillHeight") != "480" {
		t.Errorf("query = %s", (*reqs)[1].URL.RawQuery)
	}
}

func TestPrimaryImageErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"404": func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
		"not an image": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>" + testKey))
		},
	} {
		c, _ := posterServer(t, h)
		_, err := c.PrimaryImage(context.Background(), "abc", 96, 144)
		if err == nil {
			t.Errorf("%s: want error", name)
			continue
		}
		if strings.Contains(err.Error(), testKey) {
			t.Errorf("%s: error leaks key: %v", name, err)
		}
	}
	// IDs are escaped, so an ID can't reach another endpoint.
	c, reqs := posterServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	_, _ = c.PrimaryImage(context.Background(), "../../Users", 96, 144)
	if got := (*reqs)[0].URL.EscapedPath(); got != "/Items/..%2F..%2FUsers/Images/Primary" {
		t.Errorf("path = %s", got)
	}
}

func items(n int, typ string) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: fmt.Sprintf("%s%d", typ, i), Name: fmt.Sprintf("%s %d", typ, i), Type: typ}
	}
	return out
}

// checkPages checks every request is an authorised GET for typ, paginated
// by pageSize, with the key only in the header.
func checkPages(t *testing.T, reqs []*http.Request, typ string, pageSize int) {
	t.Helper()
	for i, r := range reqs {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "MediaBrowser Token="+testKey {
			t.Errorf("request %d = %s, Authorization %q", i, r.Method, r.Header.Get("Authorization"))
		}
		if q.Get("includeItemTypes") != typ || q.Get("recursive") != "true" {
			t.Errorf("request %d query = %s", i, r.URL.RawQuery)
		}
		if q.Get("startIndex") != strconv.Itoa(i*pageSize) || q.Get("limit") != strconv.Itoa(pageSize) {
			t.Errorf("request %d pagination = %s", i, r.URL.RawQuery)
		}
		if strings.Contains(r.URL.String(), testKey) {
			t.Errorf("API key sent in URL: %s", r.URL)
		}
		if q.Has("filters") || q.Has("isUnplayed") {
			t.Errorf("request %d uses filters: %s", i, r.URL.RawQuery)
		}
	}
}

func TestSeriesAndSeasonsPaginate(t *testing.T) {
	f := &fakeJellyfin{byType: map[string][]Item{"Series": items(5, "Series"), "Season": items(4, "Season")}}
	c := newTestClient(t, f)
	c.pageSize = 2

	series, err := c.Series(context.Background())
	if err != nil || !reflect.DeepEqual(series, f.byType["Series"]) {
		t.Fatalf("Series = %+v, %v", series, err)
	}
	if len(f.requests) != 3 {
		t.Errorf("Series made %d requests, want 3 pages", len(f.requests))
	}
	checkPages(t, f.requests, "Series", 2)
	if q := f.requests[0].URL.Query(); !strings.Contains(q.Get("fields"), "DateCreated") || q.Get("sortBy") != "SortName" {
		t.Errorf("Series query = %s", f.requests[0].URL.RawQuery)
	}

	f.requests = nil
	seasons, err := c.Seasons(context.Background())
	if err != nil || !reflect.DeepEqual(seasons, f.byType["Season"]) {
		t.Fatalf("Seasons = %+v, %v", seasons, err)
	}
	checkPages(t, f.requests, "Season", 2)
}

// Episode sizes come from one library-wide listing per refresh, not one per
// user: it asks for MediaSources, leaves out missing episodes and sends no
// userId.
func TestEpisodesAskForSizesOnceWithoutUser(t *testing.T) {
	f := &fakeJellyfin{byType: map[string][]Item{"Episode": items(5, "Episode")}}
	c := newTestClient(t, f)
	c.pageSize = 3

	got, err := c.Episodes(context.Background())
	if err != nil || !reflect.DeepEqual(got, f.byType["Episode"]) {
		t.Fatalf("Episodes = %+v, %v", got, err)
	}
	checkPages(t, f.requests, "Episode", 3)
	for i, r := range f.requests {
		q := r.URL.Query()
		if q.Get("isMissing") != "false" || q.Has("userId") || !strings.Contains(q.Get("fields"), "MediaSources") {
			t.Errorf("request %d query = %s", i, r.URL.RawQuery)
		}
	}
}

// Each user's episodes come from one paginated request carrying their watch
// state, never one request per season.
func TestUserEpisodesCarryWatchState(t *testing.T) {
	eps := items(5, "Episode")
	for i := range eps {
		eps[i].SeasonID = "s1"
		eps[i].SeriesID = "sr1"
		eps[i].UserData = &UserData{Played: i%2 == 0}
	}
	f := &fakeJellyfin{userEpisodes: map[string][]Item{"u1": eps}}
	c := newTestClient(t, f)
	c.pageSize = 2

	got, err := c.UserEpisodes(context.Background(), "u1")
	if err != nil || !reflect.DeepEqual(got, eps) {
		t.Fatalf("UserEpisodes = %+v, %v", got, err)
	}
	if len(f.requests) != 3 {
		t.Errorf("made %d requests, want 3 pages", len(f.requests))
	}
	checkPages(t, f.requests, "Episode", 2)
	for i, r := range f.requests {
		q := r.URL.Query()
		if q.Get("userId") != "u1" || q.Get("isMissing") != "false" || q.Get("enableUserData") != "true" ||
			q.Has("parentId") || q.Has("seasonId") || q.Has("isPlayed") {
			t.Errorf("request %d query = %s", i, r.URL.RawQuery)
		}
	}
}

// Shapes confirmed against Jellyfin 12.1.0: seasons and episodes carry
// SeriesId and IndexNumber; episodes carry SeasonId (absent for an episode
// Jellyfin couldn't place in a season) and, with a userId, UserData.Played.
func TestItemDecodesSeasonAndEpisodeFields(t *testing.T) {
	var season Item
	if err := json.Unmarshal([]byte(`{"Name":"Season 1","Id":"s","IndexNumber":1,"Type":"Season","SeriesId":"sr",
		"SeriesPrimaryImageTag":"x","ImageTags":{"Primary":"p"},"LocationType":"FileSystem"}`), &season); err != nil {
		t.Fatal(err)
	}
	if season.SeriesID != "sr" || season.IndexNumber == nil || *season.IndexNumber != 1 || season.PrimaryImageTag() != "p" {
		t.Errorf("season = %+v", season)
	}
	var specials Item
	_ = json.Unmarshal([]byte(`{"Id":"s0","IndexNumber":0,"Type":"Season"}`), &specials)
	if specials.IndexNumber == nil || *specials.IndexNumber != 0 {
		t.Errorf("Season 0 IndexNumber = %v, want 0 (not absent)", specials.IndexNumber)
	}
	var unknown Item
	_ = json.Unmarshal([]byte(`{"Name":"Season Unknown","Id":"sv","Type":"Season","LocationType":"Virtual"}`), &unknown)
	if unknown.IndexNumber != nil {
		t.Errorf("no IndexNumber decoded as %d", *unknown.IndexNumber)
	}

	var ep Item
	if err := json.Unmarshal([]byte(`{"Name":"1:23:45","Id":"e","Type":"Episode","SeriesId":"sr","SeasonId":"s",
		"SeasonName":"Season 1","ParentIndexNumber":1,"UserData":{"PlaybackPositionTicks":0,"PlayCount":2,"Played":true,"Key":"k"},
		"MediaSources":[{"Size":1073741824}]}`), &ep); err != nil {
		t.Fatal(err)
	}
	if ep.SeasonID != "s" || ep.SeasonName != "Season 1" || ep.UserData == nil || !ep.UserData.Played || ep.MediaSources[0].Size != 1<<30 {
		t.Errorf("episode = %+v", ep)
	}
}

// Every request is bounded by the timeout given to New
// (JELLYFIN_TIMEOUT_SECONDS), so a hung Jellyfin fails a refresh rather than
// stalling it forever.
func TestEachRequestUsesTheTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
	c, err := New(srv.URL, testKey, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.Users(context.Background())
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("request took %v, want about the 50ms timeout", took)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error contains the key: %v", err)
	}
}
