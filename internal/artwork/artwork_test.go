package artwork

import (
	"context"
	"errors"
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

func (f *fakeSource) PrimaryImage(ctx context.Context, id string) (jellyfin.Image, error) {
	f.calls.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	if f.err != nil {
		return jellyfin.Image{}, f.err
	}
	return jellyfin.Image{Data: append([]byte(id+":"), f.img.Data...), ContentType: f.img.ContentType}, nil
}

func TestRepeatViewsDoNotRefetch(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{Data: []byte("webp"), ContentType: "image/webp"}}
	c := New(src)
	for i := 0; i < 3; i++ {
		data, ct, err := c.Poster(context.Background(), "a", "t1")
		if err != nil || string(data) != "a:webp" || ct != "image/webp" {
			t.Fatalf("Poster = %q %q %v", data, ct, err)
		}
	}
	if _, _, err := c.Poster(context.Background(), "b", "t1"); err != nil {
		t.Fatal(err)
	}
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2 (once per item)", n)
	}
}

// A new tag means Jellyfin has a new poster for the item.
func TestNewTagRefetches(t *testing.T) {
	src := &fakeSource{img: jellyfin.Image{ContentType: "image/jpeg"}}
	c := New(src)
	_, _, _ = c.Poster(context.Background(), "a", "t1")
	_, _, _ = c.Poster(context.Background(), "a", "t2")
	_, _, _ = c.Poster(context.Background(), "a", "t2")
	if n := src.calls.Load(); n != 2 {
		t.Errorf("fetched %d times, want 2", n)
	}
}

// Failures aren't cached, so a later view can succeed.
func TestErrorsAreNotCached(t *testing.T) {
	src := &fakeSource{err: errors.New("unreachable")}
	c := New(src)
	if _, _, err := c.Poster(context.Background(), "a", "t"); err == nil {
		t.Fatal("want error")
	}
	src.err = nil
	src.img = jellyfin.Image{ContentType: "image/jpeg"}
	if _, _, err := c.Poster(context.Background(), "a", "t"); err != nil {
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
			if _, _, err := c.Poster(context.Background(), "a", "t"); err != nil {
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
	go func() { _, _, _ = c.Poster(context.Background(), "a", "t") }()
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := c.Poster(ctx, "a", "t"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}
