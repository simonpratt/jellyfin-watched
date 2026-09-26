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

	none := newCompletion(nil, nil, tags, len(all))
	if len(none.Completed.Tags) != 0 || none.Completed.Collapsed {
		t.Errorf("none = %+v", none)
	}

	four := newCompletion(all[:4], nil, tags, len(all))
	if four.Completed.Collapsed || len(four.Completed.Tags) != 4 {
		t.Errorf("four = %+v", four)
	}

	five := newCompletion(all[1:], nil, tags, len(all))
	if g := five.Completed; !g.Collapsed || g.CountPrefix != "" || g.Count != "5" || g.CountSuffix != " users" {
		t.Errorf("five = %+v", g)
	}

	every := newCompletion(all, nil, tags, len(all))
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

func TestProgressTag(t *testing.T) {
	tags := newUserTags(users("Priya", "Kwame"))
	for _, tc := range []struct {
		played, available int
		want              UserTag
	}{
		{9, 12, UserTag{Name: "Priya", Code: "PR", Hue: 1, Played: "9", Available: "12", Percent: 75}},
		{1, 200, UserTag{Name: "Priya", Code: "PR", Hue: 1, Played: "1", Available: "200", Percent: 1}},
		{1199, 1200, UserTag{Name: "Priya", Code: "PR", Hue: 1, Played: "1,199", Available: "1,200", Percent: 99}},
	} {
		got := newProgressTag(tags["id-Priya"], library.Progress{User: library.User{ID: "id-Priya"}, Played: tc.played, Available: tc.available})
		if got != tc.want {
			t.Errorf("%d / %d = %+v, want %+v", tc.played, tc.available, got, tc.want)
		}
	}
	c := newCompletion(nil, []library.Progress{{User: library.User{ID: "id-Kwame"}, Played: 3, Available: 4}}, tags, 2)
	if len(c.InProgress) != 1 || c.InProgress[0].Name != "Kwame" || c.InProgress[0].Hue != 2 || c.InProgress[0].Played != "3" {
		t.Errorf("InProgress = %+v", c.InProgress)
	}
}

func TestNewSumSize(t *testing.T) {
	for _, tc := range []struct {
		bytes   int64
		unknown int
		want    Size
	}{
		{38 << 30, 0, Size{Value: "38.0", Unit: "GB"}},
		{38 << 30, 2, Size{Value: "38.0", Unit: "GB", UnknownCount: 2}},
		{0, 3, Size{Unknown: true}},
		{0, 0, Size{Value: "0", Unit: "B"}}, // nothing on disk at all
	} {
		if got := newSumSize(tc.bytes, tc.unknown); got != tc.want {
			t.Errorf("newSumSize(%d, %d) = %+v, want %+v", tc.bytes, tc.unknown, got, tc.want)
		}
	}
}
