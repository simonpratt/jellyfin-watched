package artwork

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jellyfin-watched/internal/jellyfin"
)

type fakeSource struct {
	calls atomic.Int32
	img   jellyfin.Image
	err   error
	gate  chan struct{} // if set, each fetch waits for it to close
}

func (f *fakeSource) PrimaryImage(ctx context.Context, id string, width, height int) (jellyfin.Image, error) {
	f.calls.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	if f.err != nil {
		return jellyfin.Image{}, f.err
	}
	prefix := fmt.Sprintf("%s@%dx%d:", id, width, height)
	return jellyfin.Image{Data: append([]byte(prefix), f.img.Data...), ContentType: f.img.ContentType}, nil
}

func TestRepeatViewsDoNotRefetch(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{Data: []byte("webp"), ContentType: "image/webp"}}
	c := New(src)
	for i := 0; i < 3; i++ {
		data, ct, err := c.Poster(context.Background(), "a", "t1", 96, 144)
		if err != nil || string(data) != "a@96x144:webp" || ct != "image/webp" {
			t.Fatalf("Poster = %q %q %v", data, ct, err)
		}
	}
	if _, _, err := c.Poster(context.Background(), "b", "t1", 96, 144); err != nil {
		t.Fatal(err)
	}
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2 (once per item)", n)
	}
}

// Each size of an item's poster is fetched and kept on its own: the list's
// small slot and the grid's larger card.
func TestSizesAreCachedSeparately(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{Data: []byte("webp"), ContentType: "image/webp"}}
	c := New(src)
	for i := 0; i < 2; i++ {
		small, _, err := c.Poster(context.Background(), "a", "t1", 96, 144)
		if err != nil || string(small) != "a@96x144:webp" {
			t.Fatalf("small = %q %v", small, err)
		}
		large, _, err := c.Poster(context.Background(), "a", "t1", 320, 480)
		if err != nil || string(large) != "a@320x480:webp" {
			t.Fatalf("large = %q %v", large, err)
		}
	}
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2 (once per size)", n)
	}
}

// A new tag means Jellyfin has a new poster for the item.
func TestNewTagRefetches(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{ContentType: "image/jpeg"}}
	c := New(src)
	_, _, _ = c.Poster(context.Background(), "a", "t1", 96, 144)
	_, _, _ = c.Poster(context.Background(), "a", "t2", 96, 144)
	_, _, _ = c.Poster(context.Background(), "a", "t2", 96, 144)
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2", n)
	}
}

// Failures aren't cached, so a later view can succeed.
func TestErrorsAreNotCached(t *testing.T) {
	src := &fakeSource{err: errors.New("unreachable")}
	c := New(src)
	if _, _, err := c.Poster(context.Background(), "a", "t", 96, 144); err == nil {
		t.Fatal("want error")
	}
	src.err = nil
	src.img = jellyfin.Image{ContentType: "image/jpeg"}
	if _, _, err := c.Poster(context.Background(), "a", "t", 96, 144); err != nil {
		t.Fatal(err)
	}
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2", n)
	}
}

// Simultaneous requests for one poster share a single fetch.
func TestConcurrentRequestsShareOneFetch(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{ContentType: "image/jpeg"}, gate: make(chan struct{})}
	c := New(src)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.Poster(context.Background(), "a", "t", 96, 144); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(src.gate)
	wg.Wait()
	if n := src.calls.Load(); n != 1 {
		t.Errorf("fetched %d times, want 1", n)
	}
}

// A waiting request gives up when its own context ends.
func TestWaiterHonoursContext(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{ContentType: "image/jpeg"}, gate: make(chan struct{})}
	defer close(src.gate)
	c := New(src)
	go func() { _, _, _ = c.Poster(context.Background(), "a", "t", 96, 144) }()
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := c.Poster(ctx, "a", "t", 96, 144); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}
