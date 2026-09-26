package web

import (
	"reflect"
	"testing"

	"jellyfin-watched/internal/library"
)

func users(names ...string) []library.User {
	out := make([]library.User, len(names))
	for i, n := range names {
		out[i] = library.User{ID: "id-" + n, Name: n}
	}
	return out
}

func TestUserTagsFollowJellyfinOrderAndWrapHues(t *testing.T) {
	all := users("u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u9", "u10", "u11", "u12")
	tags := newUserTags(all)
	for i, u := range all {
		want := i%10 + 1
		if got := tags[u.ID].Hue; got != want {
			t.Errorf("%s hue = %d, want %d", u.Name, got, want)
		}
	}
	if got := tags["id-u1"]; got.Name != "u1" || got.Code != "U1" {
		t.Errorf("tag = %+v", got)
	}
}

func TestCompletionGroupsFiveOrMoreUsers(t *testing.T) {
	all := users("Sam", "Priya", "Jonas", "Mei", "Alex", "Theo")
	tags := newUserTags(all)

	none := newCompletion(nil, tags, len(all))
	if len(none.Completed.Tags) != 0 || none.Completed.Collapsed {
		t.Errorf("none = %+v", none)
	}

	four := newCompletion(all[:4], tags, len(all))
	if four.Completed.Collapsed || len(four.Completed.Tags) != 4 {
		t.Errorf("four = %+v", four)
	}

	five := newCompletion(all[1:], tags, len(all))
	if g := five.Completed; !g.Collapsed || g.CountPrefix != "" || g.Count != "5" || g.CountSuffix != " users" {
		t.Errorf("five = %+v", g)
	}

	every := newCompletion(all, tags, len(all))
	if g := every.Completed; !g.Collapsed || g.CountPrefix != "All " || g.Count != "6" || g.CountSuffix != " users" {
		t.Errorf("all = %+v", g)
	}
	wantNames := []string{"Sam", "Priya", "Jonas", "Mei", "Alex", "Theo"}
	var got []string
	for _, tg := range every.Completed.Tags {
		got = append(got, tg.Name)
	}
	if !reflect.DeepEqual(got, wantNames) {
		t.Errorf("tags = %v", got)
	}
}

func TestNewSize(t *testing.T) {
	if s := newSize(0); !s.Unknown || s.Value != "" {
		t.Errorf("newSize(0) = %+v, want Unknown", s)
	}
	if s := newSize(15676630630); s.Unknown || s.Value != "14.6" || s.Unit != "GB" {
		t.Errorf("newSize = %+v", s)
	}
}
