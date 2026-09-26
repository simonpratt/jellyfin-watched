package library

import (
	"reflect"
	"testing"
	"time"
)

func day(d int) time.Time { return time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC) }

// viewMovies are in Jellyfin's sort-name order, which ignores "The": the
// title sort keeps that order rather than sorting the raw titles.
var viewMovies = []Movie{
	{ID: "alien", Title: "Alien", Year: 1979, Added: day(3), Size: 30},
	{ID: "arrival", Title: "Arrival", Year: 2016, Added: day(1), Size: 0}, // size unknown
	{ID: "blade", Title: "Blade Runner", Year: 1982, Added: day(2), Size: 50},
	{ID: "blade2049", Title: "Blade Runner 2049", Year: 2017, Added: time.Time{}, Size: 80}, // added unknown
	{ID: "matrix", Title: "The Matrix", Year: 1999, Added: day(5), Size: 30},
	{ID: "nowhere", Title: "Nowhere", Year: 0, Added: day(4), Size: 0}, // year and size unknown
}

func movieIDs(ms []Movie) []string {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}

func TestArrangeMoviesSearch(t *testing.T) {
	for _, tc := range []struct {
		search string
		want   []string
	}{
		{"", []string{"alien", "arrival", "blade", "blade2049", "matrix", "nowhere"}},
		{"blade", []string{"blade", "blade2049"}},
		{"BLADE runner", []string{"blade", "blade2049"}}, // case-insensitive
		{"  matrix ", []string{"matrix"}},                // surrounding space ignored
		{"the", []string{"matrix"}},
		{"xyz", []string{}},
	} {
		got := movieIDs(ArrangeMovies(viewMovies, View{Search: tc.search}))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q: got %v, want %v", tc.search, got, tc.want)
		}
	}
}

func TestSearchIsCaseInsensitiveBeyondASCII(t *testing.T) {
	ms := []Movie{{ID: "a", Title: "Amélie"}, {ID: "b", Title: "СУПЕР ФИЛЬМ"}}
	if got := movieIDs(ArrangeMovies(ms, View{Search: "AMÉL"})); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("AMÉL: %v", got)
	}
	if got := movieIDs(ArrangeMovies(ms, View{Search: "фильм"})); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("фильм: %v", got)
	}
}

// Every key sorts both ways. Ties fall back to title order, ascending
// whichever the direction. Unknown values sort last in both directions.
func TestArrangeMoviesSort(t *testing.T) {
	for _, tc := range []struct {
		sort SortKey
		desc bool
		want []string
	}{
		{SortTitle, false, []string{"alien", "arrival", "blade", "blade2049", "matrix", "nowhere"}},
		{SortTitle, true, []string{"nowhere", "matrix", "blade2049", "blade", "arrival", "alien"}},
		{"", false, []string{"alien", "arrival", "blade", "blade2049", "matrix", "nowhere"}},
		{SortYear, false, []string{"alien", "blade", "matrix", "arrival", "blade2049", "nowhere"}},
		{SortYear, true, []string{"blade2049", "arrival", "matrix", "blade", "alien", "nowhere"}},
		{SortAdded, false, []string{"arrival", "blade", "alien", "nowhere", "matrix", "blade2049"}},
		{SortAdded, true, []string{"matrix", "nowhere", "alien", "blade", "arrival", "blade2049"}},
		{SortSize, false, []string{"alien", "matrix", "blade", "blade2049", "arrival", "nowhere"}},
		{SortSize, true, []string{"blade2049", "blade", "alien", "matrix", "arrival", "nowhere"}},
	} {
		got := movieIDs(ArrangeMovies(viewMovies, View{Sort: tc.sort, Desc: tc.desc}))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("sort %q desc=%v: got %v, want %v", tc.sort, tc.desc, got, tc.want)
		}
	}
}

func TestArrangeMoviesSearchesThenSorts(t *testing.T) {
	got := movieIDs(ArrangeMovies(viewMovies, View{Search: "blade", Sort: SortSize, Desc: true}))
	if want := []string{"blade2049", "blade"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestArrangeMoviesLeavesInputAlone(t *testing.T) {
	in := append([]Movie(nil), viewMovies...)
	ArrangeMovies(in, View{Sort: SortSize, Desc: true})
	if !reflect.DeepEqual(in, viewMovies) {
		t.Error("input reordered")
	}
}

// Series sort by their total size: the sum of the episodes that report one.
// A series with no known size at all sorts last.
func TestArrangeSeries(t *testing.T) {
	series := []Series{
		{ID: "andor", Title: "Andor", Year: 2022, Added: day(2), Size: 90},
		{ID: "fleabag", Title: "Fleabag", Year: 2016, Added: day(1), Size: 12},
		{ID: "simpsons", Title: "The Simpsons", Year: 1989, Added: day(3), Size: 2000},
		{ID: "twin", Title: "Twin Peaks", Year: 1990, Added: day(4), Size: 38, UnknownSizes: 2},
		{ID: "unknown", Title: "Unknown Size", Year: 2001, Added: day(5), UnknownSizes: 8},
	}
	ids := func(ss []Series) []string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = s.ID
		}
		return out
	}
	for _, tc := range []struct {
		v    View
		want []string
	}{
		{View{Sort: SortSize}, []string{"fleabag", "twin", "andor", "simpsons", "unknown"}},
		{View{Sort: SortSize, Desc: true}, []string{"simpsons", "andor", "twin", "fleabag", "unknown"}},
		{View{Sort: SortYear}, []string{"simpsons", "twin", "unknown", "fleabag", "andor"}},
		{View{Sort: SortAdded, Desc: true}, []string{"unknown", "twin", "simpsons", "andor", "fleabag"}},
		{View{Sort: SortTitle, Desc: true}, []string{"unknown", "twin", "simpsons", "fleabag", "andor"}},
		{View{Search: "simp"}, []string{"simpsons"}},
	} {
		if got := ids(ArrangeSeries(series, tc.v)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%+v: got %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestParseSortKey(t *testing.T) {
	for in, want := range map[string]SortKey{"title": SortTitle, "year": SortYear, "added": SortAdded, "size": SortSize, "": SortTitle, "bogus": SortTitle, "SIZE": SortTitle} {
		if got := ParseSortKey(in); got != want {
			t.Errorf("ParseSortKey(%q) = %q, want %q", in, got, want)
		}
	}
}
