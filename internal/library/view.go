package library

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// SortKey is what a list is sorted by. Every key is an ordinary sort: none
// is a ranking or a recommendation (REQUIREMENTS.md §3).
type SortKey string

const (
	SortTitle SortKey = "title"
	SortYear  SortKey = "year"
	SortAdded SortKey = "added" // date added to the library
	SortSize  SortKey = "size"
)

// SortKeys are the keys in the order the sort control shows them.
var SortKeys = []SortKey{SortTitle, SortYear, SortAdded, SortSize}

// ParseSortKey returns the key named s, or SortTitle for anything else.
func ParseSortKey(s string) SortKey {
	for _, k := range SortKeys {
		if string(k) == s {
			return k
		}
	}
	return SortTitle
}

// View is how the dashboard shows a list: the titles it is narrowed to and
// the order it is in.
//
// Search keeps items whose title contains it, ignoring case and surrounding
// space. Sort orders by the key, ascending unless Desc; the zero key is
// SortTitle.
//
// Ordering rules:
//   - Title order is Jellyfin's sort-name order (the snapshot's order), so
//     "The Simpsons" sorts under S as it does in Jellyfin.
//   - Items with the same value sort by title order, ascending whichever the
//     direction.
//   - Items whose value is unknown (no year, no date added, no known size)
//     always sort last, in title order, whichever the direction.
//   - A series sorts by its total size: the sum of the episodes that report
//     a size. A series none of whose episodes reports one is unknown.
type View struct {
	Search string
	Sort   SortKey
	Desc   bool
}

// ArrangeMovies returns the movies v shows, in its order. ms is in the
// snapshot's (title) order and is left as it is.
func ArrangeMovies(ms []Movie, v View) []Movie {
	return arrange(ms, v, func(m Movie) facts {
		return facts{title: m.Title, year: m.Year, added: m.Added, size: m.Size}
	})
}

// ArrangeSeries returns the series v shows, in its order. ss is in the
// snapshot's (title) order and is left as it is.
func ArrangeSeries(ss []Series, v View) []Series {
	return arrange(ss, v, func(s Series) facts {
		return facts{title: s.Title, year: s.Year, added: s.Added, size: s.Size}
	})
}

// facts are what search and sort look at. Zero means unknown.
type facts struct {
	title string
	year  int
	added time.Time
	size  int64
}

func arrange[T any](items []T, v View, factsOf func(T) facts) []T {
	type entry struct {
		item T
		rank int // position in title order
		f    facts
	}
	needle := strings.ToLower(strings.TrimSpace(v.Search))
	entries := make([]entry, 0, len(items))
	for i, it := range items {
		f := factsOf(it)
		if needle != "" && !strings.Contains(strings.ToLower(f.title), needle) {
			continue
		}
		entries = append(entries, entry{it, i, f})
	}

	// value compares a and b by the sort key; known reports whether each
	// has a value to compare.
	value := func(a, b facts) (c int, aKnown, bKnown bool) {
		switch v.Sort {
		case SortYear:
			return cmp.Compare(a.year, b.year), a.year > 0, b.year > 0
		case SortAdded:
			return a.added.Compare(b.added), !a.added.IsZero(), !b.added.IsZero()
		case SortSize:
			return cmp.Compare(a.size, b.size), a.size > 0, b.size > 0
		}
		return 0, true, true
	}
	slices.SortFunc(entries, func(a, b entry) int {
		c, aKnown, bKnown := value(a.f, b.f)
		switch {
		case aKnown && !bKnown:
			return -1
		case !aKnown && bKnown:
			return 1
		case !aKnown && !bKnown:
			return cmp.Compare(a.rank, b.rank)
		}
		if c == 0 && (v.Sort == SortTitle || v.Sort == "") {
			c = cmp.Compare(a.rank, b.rank)
		}
		if v.Desc {
			c = -c
		}
		if c != 0 {
			return c
		}
		return cmp.Compare(a.rank, b.rank)
	})

	out := make([]T, len(entries))
	for i, e := range entries {
		out[i] = e.item
	}
	return out
}
