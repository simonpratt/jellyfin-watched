package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

// fakeServer answers the three requests a movie refresh makes with JSON
// shaped like Jellyfin 12.1.0's: /Users is a bare array, /Items a page.
func fakeServer(t *testing.T, users string, movies string, played map[string]string) *jellyfin.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/Users":
			_, _ = w.Write([]byte(users))
		case r.URL.Path == "/Items" && q.Get("userId") == "":
			_, _ = w.Write([]byte(movies))
		case r.URL.Path == "/Items" && q.Get("isPlayed") == "true":
			body, ok := played[q.Get("userId")]
			if !ok {
				body = `{"Items":[],"TotalRecordCount":0,"StartIndex":0}`
			}
			_, _ = w.Write([]byte(body))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := jellyfin.New(srv.URL, "key", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const threeUsers = `[
 {"Name":"sally","Id":"u-sally","HasPassword":true,"Policy":{"IsAdministrator":false}},
 {"Name":"Anastasia Kowalczyk","Id":"u-ak","HasPassword":true,"Policy":{"IsAdministrator":false}},
 {"Name":"H","Id":"u-h","HasPassword":false,"Policy":{"IsAdministrator":true}}
]`

const fourMovies = `{"Items":[
 {"Name":"Two Versions","Id":"m-two","Type":"Movie","ProductionYear":2019,
  "ImageTags":{"Primary":"tag-two","Logo":"logo"},"BackdropImageTags":["bd"],
  "MediaSources":[{"Id":"m-two","Size":16106127360,"Name":"1080p"},{"Id":"alt","Size":69793218560,"Name":"2160p"}]},
 {"Name":"No Size","Id":"m-nosize","Type":"Movie","MediaSources":[{"Id":"m-nosize","Name":"x"}]},
 {"Name":"One Version Unreported","Id":"m-partial","Type":"Movie",
  "MediaSources":[{"Id":"m-partial","Size":1073741824},{"Id":"alt2"}]},
 {"Name":"No Sources","Id":"m-nosrc","Type":"Movie"}
],"TotalRecordCount":4,"StartIndex":0}`

func played(ids ...string) string {
	body := `{"Items":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += `{"Name":"x","Id":"` + id + `","Type":"Movie","UserData":{"Played":true}}`
	}
	return body + `],"TotalRecordCount":` + strconv.Itoa(len(ids)) + `,"StartIndex":0}`
}

func TestRefreshFromJellyfinSizesAndCompletion(t *testing.T) {
	c := fakeServer(t, threeUsers, fourMovies, map[string]string{
		// Everyone played m-two; H alone played m-partial; nobody else.
		"u-sally": played("m-two"),
		"u-ak":    played("m-two"),
		"u-h":     played("m-partial", "m-two", "not-a-movie-we-know"),
	})
	cache := NewCache(c)
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, _ := cache.Snapshot()

	sally := User{ID: "u-sally", Name: "sally"}
	ak := User{ID: "u-ak", Name: "Anastasia Kowalczyk"}
	h := User{ID: "u-h", Name: "H"}
	if want := []User{sally, ak, h}; !reflect.DeepEqual(snap.Users, want) {
		t.Errorf("Users = %+v, want %+v (Jellyfin's order)", snap.Users, want)
	}

	byID := map[string]Movie{}
	for _, m := range snap.Movies {
		byID[m.ID] = m
	}
	// Multiple versions: sizes add up.
	if got := byID["m-two"].Size; got != 16106127360+69793218560 {
		t.Errorf("two-version size = %d", got)
	}
	// Unknown sizes stay unknown (0), never a partial sum.
	for _, id := range []string{"m-nosize", "m-partial", "m-nosrc"} {
		if got := byID[id].Size; got != 0 {
			t.Errorf("%s size = %d, want 0 (unknown)", id, got)
		}
	}
	// All users, in Jellyfin's user order regardless of fetch order.
	if got := byID["m-two"].CompletedBy; !reflect.DeepEqual(got, []User{sally, ak, h}) {
		t.Errorf("m-two CompletedBy = %+v", got)
	}
	if got := byID["m-partial"].CompletedBy; !reflect.DeepEqual(got, []User{h}) {
		t.Errorf("m-partial CompletedBy = %+v", got)
	}
	// Zero users.
	if got := byID["m-nosize"].CompletedBy; len(got) != 0 {
		t.Errorf("m-nosize CompletedBy = %+v, want none", got)
	}
}

func TestRefreshWithNoUsers(t *testing.T) {
	cache := NewCache(fakeServer(t, `[]`, fourMovies, nil))
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, _ := cache.Snapshot()
	if len(snap.Users) != 0 || len(snap.Movies) != 4 {
		t.Fatalf("snapshot = %+v", snap)
	}
	for _, m := range snap.Movies {
		if len(m.CompletedBy) != 0 {
			t.Errorf("%s completed by %+v", m.Title, m.CompletedBy)
		}
	}
}

func TestRefreshKeepsPosterTags(t *testing.T) {
	cache := NewCache(fakeServer(t, `[]`, fourMovies, nil))
	if err := cache.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, _ := cache.Snapshot()
	for _, m := range snap.Movies {
		want := ""
		if m.ID == "m-two" {
			want = "tag-two"
		}
		if m.PosterTag != want {
			t.Errorf("%s PosterTag = %q, want %q", m.ID, m.PosterTag, want)
		}
	}
	if tag, ok := snap.PosterTag("m-two"); !ok || tag != "tag-two" {
		t.Errorf(`PosterTag("m-two") = %q, %v`, tag, ok)
	}
	// Items with no poster, and IDs not in the library, have none.
	for _, id := range []string{"m-nosize", "not-in-library", ""} {
		if tag, ok := snap.PosterTag(id); ok {
			t.Errorf("PosterTag(%q) = %q, true", id, tag)
		}
	}
}
