package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

const gib = 1 << 30

// tvEpisode is one episode in the fake library. Missing episodes are
// placeholders Jellyfin lists for episodes not on disk.
type tvEpisode struct {
	id, series, season string // season "" when Jellyfin couldn't place it
	size               int64
	missing            bool
	playedBy           []string // user IDs
	hiddenFrom         []string // user IDs without access to it
}

// tvServer is a fake Jellyfin answering every request a refresh makes, with
// JSON shaped like 12.1.0's. It honours isMissing=false and userId the way
// Jellyfin does, and records every request.
type tvServer struct {
	mu       sync.Mutex
	requests []string // method and URL
}

func (f *tvServer) client(t *testing.T, users string, series, seasons []map[string]any, eps []tvEpisode) *jellyfin.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.String())
		f.mu.Unlock()
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/Users" {
			_, _ = w.Write([]byte(users))
			return
		}
		q := r.URL.Query()
		var items []map[string]any
		switch q.Get("includeItemTypes") {
		case "Movie":
		case "Series":
			items = series
		case "Season":
			items = seasons
		case "Episode":
			items = episodeItems(eps, q)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
		if items == nil {
			items = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
	}))
	t.Cleanup(srv.Close)
	c, err := jellyfin.New(srv.URL, "key", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func episodeItems(eps []tvEpisode, q map[string][]string) []map[string]any {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	user := get("userId")
	var out []map[string]any
	for _, e := range eps {
		if e.missing && get("isMissing") == "false" {
			continue
		}
		if user != "" && contains(e.hiddenFrom, user) {
			continue
		}
		it := map[string]any{"Name": "Episode " + e.id, "Id": e.id, "Type": "Episode", "SeriesId": e.series,
			"SeasonName": "Season 1", "LocationType": "FileSystem"}
		if e.season != "" {
			it["SeasonId"] = e.season
		} else {
			it["SeasonName"] = "Season Unknown"
		}
		if e.missing {
			it["LocationType"] = "Virtual"
		}
		if strings.Contains(get("fields"), "MediaSources") && !e.missing {
			src := map[string]any{"Id": e.id, "Name": e.id}
			if e.size > 0 {
				src["Size"] = e.size
			}
			it["MediaSources"] = []any{src}
		}
		if user != "" && get("enableUserData") == "true" {
			it["UserData"] = map[string]any{"PlaybackPositionTicks": 0, "Played": contains(e.playedBy, user), "Key": e.id}
		}
		out = append(out, it)
	}
	return out
}

func season(id, series, name string, index any, posterTag string) map[string]any {
	s := map[string]any{"Name": name, "Id": id, "Type": "Season", "SeriesId": series, "SeriesName": series,
		"ImageTags": map[string]string{}, "LocationType": "FileSystem"}
	if index != nil {
		s["IndexNumber"] = index
	} else {
		s["LocationType"] = "Virtual"
	}
	if posterTag != "" {
		s["ImageTags"] = map[string]string{"Primary": posterTag}
	}
	return s
}

const tvUsers = `[{"Name":"sally","Id":"u-sally"},{"Name":"Anastasia Kowalczyk","Id":"u-ak"},{"Name":"H","Id":"u-h"}]`

var (
	sally = User{ID: "u-sally", Name: "sally"}
	ak    = User{ID: "u-ak", Name: "Anastasia Kowalczyk"}
	h     = User{ID: "u-h", Name: "H"}
)

var tvSeries = []map[string]any{
	{"Name": "Andor", "Id": "sr-andor", "Type": "Series", "ProductionYear": 2022, "DateCreated": "2025-01-02T03:04:05.1234567Z",
		"ImageTags": map[string]string{"Primary": "tag-andor", "Logo": "l"}},
	{"Name": "Fleabag", "Id": "sr-flea", "Type": "Series", "ProductionYear": 2016},
	{"Name": "Nobody Watched This", "Id": "sr-none", "Type": "Series"},
	{"Name": "PAW Patrol The Movie (2021)", "Id": "sr-orphan", "Type": "Series", "ImageTags": map[string]string{"Primary": "tag-orphan"}},
	{"Name": "No Episodes Yet", "Id": "sr-empty", "Type": "Series"},
	{"Name": "The Wire", "Id": "sr-wire", "Type": "Series"},
}

var tvSeasons = []map[string]any{
	// Seasons out of order, as Jellyfin may list them.
	season("s-andor-2", "sr-andor", "Season 2", 2, ""),
	season("s-andor-1", "sr-andor", "Season 1", 1, "tag-a1"),
	season("s-andor-0", "sr-andor", "Specials", 0, "tag-sp"),
	// A duplicate, empty "Season 1" folder (seen on the real server).
	season("s-andor-1b", "sr-andor", "Season 1", 1, ""),
	season("s-flea-1", "sr-flea", "Season 1", 1, ""),
	season("s-none-1", "sr-none", "Season 1", 1, ""),
	// The Virtual season Jellyfin lists for episodes it couldn't place.
	season("s-orphan-v", "sr-orphan", "Season Unknown", nil, ""),
	season("s-empty-1", "sr-empty", "Season 1", 1, ""),
	season("s-wire-1", "sr-wire", "Season 1", 1, ""),
	season("s-wire-2", "sr-wire", "Season 2", 2, ""),
	season("s-wire-3", "sr-wire", "Season 3", 3, ""),
}

var tvEpisodes = append([]tvEpisode{
	// Andor Season 1: sally watched all; AK one of three, and can't see the
	// third episode (library access), which still counts as available.
	{id: "e1a", series: "sr-andor", season: "s-andor-1", size: gib, playedBy: []string{"u-sally", "u-ak"}},
	{id: "e1b", series: "sr-andor", season: "s-andor-1", size: gib, playedBy: []string{"u-sally"}},
	{id: "e1c", series: "sr-andor", season: "s-andor-1", size: gib, playedBy: []string{"u-sally"}, hiddenFrom: []string{"u-ak"}},
	// Andor Season 2: one episode with no size, and a missing episode
	// nobody has played, which mustn't stop sally completing the season.
	{id: "e2a", series: "sr-andor", season: "s-andor-2", size: 2 * gib, playedBy: []string{"u-sally", "u-h"}},
	{id: "e2b", series: "sr-andor", season: "s-andor-2", size: 2 * gib, playedBy: []string{"u-sally", "u-h"}},
	{id: "e2c", series: "sr-andor", season: "s-andor-2", size: 2 * gib, playedBy: []string{"u-sally", "u-h"}},
	{id: "e2d", series: "sr-andor", season: "s-andor-2", playedBy: []string{"u-sally"}},
	{id: "e2m", series: "sr-andor", season: "s-andor-2", missing: true},
	// Andor Specials: hidden by default, but in the series size.
	{id: "e0a", series: "sr-andor", season: "s-andor-0", size: 5 * gib, playedBy: []string{"u-h"}},
	// Fleabag: everyone has watched everything.
	{id: "f1", series: "sr-flea", season: "s-flea-1", size: gib, playedBy: []string{"u-sally", "u-ak", "u-h"}},
	{id: "f2", series: "sr-flea", season: "s-flea-1", size: gib, playedBy: []string{"u-sally", "u-ak", "u-h"}},
	// Nobody has watched anything, and no size is known.
	{id: "n1", series: "sr-none", season: "s-none-1"},
	{id: "n2", series: "sr-none", season: "s-none-1"},
	// An episode with no SeasonId.
	{id: "o1", series: "sr-orphan", size: gib, playedBy: []string{"u-h"}},
	// Only a missing episode.
	{id: "x1", series: "sr-empty", season: "s-empty-1", missing: true},
}, wireEpisodes()...)

// wireEpisodes is The Wire: three seasons of ten episodes, plus a missing
// one in Season 3. sally has watched everything; AK is halfway through
// Season 3; H has watched Seasons 1 and 3 but only three episodes of
// Season 2.
func wireEpisodes() []tvEpisode {
	var eps []tvEpisode
	for s := 1; s <= 3; s++ {
		for e := 1; e <= 10; e++ {
			played := []string{"u-sally"}
			if s < 3 || e <= 5 {
				played = append(played, "u-ak")
			}
			if s != 2 || e <= 3 {
				played = append(played, "u-h")
			}
			eps = append(eps, tvEpisode{id: fmt.Sprintf("w%d-%d", s, e), series: "sr-wire",
				season: fmt.Sprintf("s-wire-%d", s), size: gib, playedBy: played})
		}
	}
	return append(eps, tvEpisode{id: "w3-m", series: "sr-wire", season: "s-wire-3", missing: true})
}

func refreshTV(t *testing.T, opts Options) (Snapshot, *tvServer) {
	t.Helper()
	f := &tvServer{}
	cache := NewCache(f.client(t, tvUsers, tvSeries, tvSeasons, tvEpisodes), opts)
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, _ := cache.Snapshot()
	return snap, f
}

func seriesByID(snap Snapshot) map[string]Series {
	m := map[string]Series{}
	for _, s := range snap.Series {
		m[s.ID] = s
	}
	return m
}

func TestTVSeasonCompletionFromEpisodes(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	andor := seriesByID(snap)["sr-andor"]
	if andor.Title != "Andor" || andor.Year != 2022 || andor.PosterTag != "tag-andor" ||
		!andor.Added.Equal(time.Date(2025, 1, 2, 3, 4, 5, 123456700, time.UTC)) {
		t.Errorf("Andor = %+v", andor)
	}
	if len(andor.Seasons) != 2 {
		t.Fatalf("Andor seasons = %+v, want Season 1 and Season 2 (no Specials, no empty duplicate)", andor.Seasons)
	}
	s1, s2 := andor.Seasons[0], andor.Seasons[1]
	if s1.ID != "s-andor-1" || s2.ID != "s-andor-2" || s1.Name != "Season 1" || s2.Name != "Season 2" {
		t.Fatalf("seasons out of order: %q %q", s1.ID, s2.ID)
	}

	// AC3: a user who watched every available episode completed it. AC4:
	// one unwatched available episode makes a user in progress with the
	// right count. AC5: the missing episode in Season 2 doesn't count.
	if !reflect.DeepEqual(s1.CompletedBy, []User{sally}) || !reflect.DeepEqual(s1.InProgress, []Progress{{User: ak, Played: 1, Available: 3}}) {
		t.Errorf("Season 1 completed %+v, in progress %+v", s1.CompletedBy, s1.InProgress)
	}
	if !reflect.DeepEqual(s2.CompletedBy, []User{sally}) || !reflect.DeepEqual(s2.InProgress, []Progress{{User: h, Played: 3, Available: 4}}) {
		t.Errorf("Season 2 completed %+v, in progress %+v", s2.CompletedBy, s2.InProgress)
	}
	if s1.Episodes != 3 || s2.Episodes != 4 {
		t.Errorf("episodes = %d, %d; want 3, 4 (missing episode left out)", s1.Episodes, s2.Episodes)
	}
}

func TestTVSizesSumEpisodesAndSeasons(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	byID := seriesByID(snap)
	andor := byID["sr-andor"]
	// AC6: seasons sum their episodes, leaving out and counting unknowns.
	if s := andor.Seasons[0]; s.Size != 3*gib || s.UnknownSizes != 0 {
		t.Errorf("Season 1 size = %d + %d unknown", s.Size, s.UnknownSizes)
	}
	if s := andor.Seasons[1]; s.Size != 6*gib || s.UnknownSizes != 1 {
		t.Errorf("Season 2 size = %d + %d unknown", s.Size, s.UnknownSizes)
	}
	// The series sums every season, including the hidden Specials.
	if andor.Size != 14*gib || andor.UnknownSizes != 1 {
		t.Errorf("Andor size = %d + %d unknown, want 14 GiB + 1", andor.Size, andor.UnknownSizes)
	}
	none := byID["sr-none"]
	if none.Size != 0 || none.UnknownSizes != 2 || none.Seasons[0].Size != 0 || none.Seasons[0].UnknownSizes != 2 {
		t.Errorf("unknown sizes: series %+v", none)
	}
}

func TestTVEveryoneAndNobody(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	byID := seriesByID(snap)
	flea := byID["sr-flea"].Seasons[0]
	if !reflect.DeepEqual(flea.CompletedBy, []User{sally, ak, h}) || flea.InProgress != nil {
		t.Errorf("Fleabag completed %+v, in progress %+v; want everyone, in /Users order", flea.CompletedBy, flea.InProgress)
	}
	// Users who haven't started aren't listed at all.
	none := byID["sr-none"].Seasons[0]
	if none.CompletedBy != nil || none.InProgress != nil {
		t.Errorf("nobody: completed %+v, in progress %+v", none.CompletedBy, none.InProgress)
	}
}

func TestTVSpecialsShownOnlyWhenIncluded(t *testing.T) {
	snap, _ := refreshTV(t, Options{IncludeSpecials: true})
	andor := seriesByID(snap)["sr-andor"]
	if len(andor.Seasons) != 3 || andor.Seasons[0].ID != "s-andor-0" {
		t.Fatalf("seasons = %+v, want Specials first", andor.Seasons)
	}
	sp := andor.Seasons[0]
	if sp.Name != "Specials" || sp.Size != 5*gib || !reflect.DeepEqual(sp.CompletedBy, []User{h}) {
		t.Errorf("Specials = %+v", sp)
	}
	if andor.Size != 14*gib {
		t.Errorf("series size changed with INCLUDE_SPECIALS: %d", andor.Size)
	}
}

func TestTVSeasonPosterFallsBackToSeries(t *testing.T) {
	snap, _ := refreshTV(t, Options{IncludeSpecials: true})
	andor := seriesByID(snap)["sr-andor"]
	got := map[string][2]string{}
	for _, s := range andor.Seasons {
		got[s.ID] = [2]string{s.PosterID, s.PosterTag}
	}
	want := map[string][2]string{
		"s-andor-0": {"s-andor-0", "tag-sp"},
		"s-andor-1": {"s-andor-1", "tag-a1"},
		"s-andor-2": {"sr-andor", "tag-andor"}, // no poster of its own
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("season posters = %v, want %v", got, want)
	}
	// The poster route can serve series and season posters, and nothing
	// without one.
	for id, tag := range map[string]string{"sr-andor": "tag-andor", "s-andor-1": "tag-a1"} {
		if got, ok := snap.PosterTag(id); !ok || got != tag {
			t.Errorf("PosterTag(%q) = %q, %v", id, got, ok)
		}
	}
	for _, id := range []string{"s-andor-2", "sr-flea"} {
		if got, ok := snap.PosterTag(id); ok {
			t.Errorf("PosterTag(%q) = %q, true", id, got)
		}
	}
}

// Real Jellyfin: an episode without a SeasonId, listed with SeasonName
// "Season Unknown", under a Virtual season that has no episodes.
func TestTVEpisodeWithoutSeasonGetsItsOwnSeason(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	orphan := seriesByID(snap)["sr-orphan"]
	if len(orphan.Seasons) != 1 {
		t.Fatalf("seasons = %+v", orphan.Seasons)
	}
	s := orphan.Seasons[0]
	if s.Name != "Season Unknown" || s.Episodes != 1 || s.Size != gib || !reflect.DeepEqual(s.CompletedBy, []User{h}) ||
		s.PosterID != "sr-orphan" {
		t.Errorf("season = %+v", s)
	}
	if orphan.Size != gib {
		t.Errorf("series size = %d", orphan.Size)
	}
}

func TestTVSeriesWithNoEpisodesStillListed(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	if len(snap.Series) != len(tvSeries) {
		t.Fatalf("got %d series, want %d", len(snap.Series), len(tvSeries))
	}
	for i, s := range snap.Series {
		if s.ID != tvSeries[i]["Id"] {
			t.Errorf("series %d = %s, want Jellyfin's order", i, s.ID)
		}
	}
	empty := seriesByID(snap)["sr-empty"]
	if len(empty.Seasons) != 0 || empty.Size != 0 || empty.UnknownSizes != 0 {
		t.Errorf("empty series = %+v", empty)
	}
}

// Series and seasons are fetched once; episode sizes once, without a user;
// each user's episodes in one request, never per season.
func TestTVRefreshRequests(t *testing.T) {
	_, f := refreshTV(t, Options{})
	count := func(match ...string) int {
		n := 0
	next:
		for _, r := range f.requests {
			for _, m := range match {
				if !strings.Contains(r, m) {
					continue next
				}
			}
			n++
		}
		return n
	}
	if n := count("includeItemTypes=Series"); n != 1 {
		t.Errorf("%d series requests", n)
	}
	if n := count("includeItemTypes=Season"); n != 1 {
		t.Errorf("%d season requests", n)
	}
	if n := count("includeItemTypes=Episode", "MediaSources"); n != 1 {
		t.Errorf("%d episode size requests, want 1", n)
	}
	for _, u := range []string{"u-sally", "u-ak", "u-h"} {
		if n := count("includeItemTypes=Episode", "userId="+u, "enableUserData=true", "isMissing=false"); n != 1 {
			t.Errorf("%d episode requests for %s, want 1", n, u)
		}
		if n := count("userId="+u, "MediaSources"); n != 0 {
			t.Errorf("%s: sizes fetched per user", u)
		}
	}
	for _, r := range f.requests {
		if !strings.HasPrefix(r, "GET ") || strings.Contains(r, "seasonId=") || strings.Contains(r, "parentId=") {
			t.Errorf("request %s", r)
		}
	}
}

// Each series rolls its shown seasons' completion up: completed when every
// counted episode is played, in progress with played / available across the
// series otherwise.
func TestTVSeriesRollUp(t *testing.T) {
	snap, _ := refreshTV(t, Options{})
	byID := seriesByID(snap)
	for _, tc := range []struct {
		series     string
		completed  []User
		inProgress []Progress
	}{
		// Everyone has watched everything.
		{"sr-flea", []User{sally, ak, h}, nil},
		// Nobody has watched anything.
		{"sr-none", nil, nil},
		// sally has watched every season; AK is halfway through the last
		// ("25 / 30"); H completed Seasons 1 and 3 but not 2. The missing
		// episode doesn't count.
		{"sr-wire", []User{sally}, []Progress{{User: ak, Played: 25, Available: 30}, {User: h, Played: 23, Available: 30}}},
		// H's played Special is left out while Specials are hidden: 3 of
		// the 7 episodes in Seasons 1 and 2. AK's hidden episode still
		// counts as available.
		{"sr-andor", []User{sally}, []Progress{{User: ak, Played: 1, Available: 7}, {User: h, Played: 3, Available: 7}}},
		// A series with no counted episodes has no roll-up.
		{"sr-empty", nil, nil},
	} {
		sr := byID[tc.series]
		if !reflect.DeepEqual(sr.CompletedBy, tc.completed) || !reflect.DeepEqual(sr.InProgress, tc.inProgress) {
			t.Errorf("%s: completed %+v, in progress %+v; want %+v, %+v", tc.series, sr.CompletedBy, sr.InProgress, tc.completed, tc.inProgress)
		}
	}
	if w := byID["sr-wire"]; w.Episodes != 30 || byID["sr-andor"].Episodes != 7 || byID["sr-empty"].Episodes != 0 {
		t.Errorf("series episodes = %d, %d, %d; want 30, 7, 0", w.Episodes, byID["sr-andor"].Episodes, byID["sr-empty"].Episodes)
	}
}

// With Specials shown, they count towards the roll-up like any season.
func TestTVSeriesRollUpIncludesShownSpecials(t *testing.T) {
	snap, _ := refreshTV(t, Options{IncludeSpecials: true})
	andor := seriesByID(snap)["sr-andor"]
	want := []Progress{{User: sally, Played: 7, Available: 8}, {User: ak, Played: 1, Available: 8}, {User: h, Played: 4, Available: 8}}
	if andor.CompletedBy != nil || !reflect.DeepEqual(andor.InProgress, want) {
		t.Errorf("Andor with Specials: completed %+v, in progress %+v; want %+v", andor.CompletedBy, andor.InProgress, want)
	}
}
