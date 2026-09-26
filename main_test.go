package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "k3y-that-must-never-leak-8f2a"

const posterBytes = "\xff\xd8\xff\xe0 a poster"

// fakeJellyfin mimics the parts of the Jellyfin API the app uses, records
// every request, and rejects requests without the right key.
type fakeJellyfin struct {
	mu      sync.Mutex
	methods []string
	fail    bool
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.methods = append(f.methods, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	if f.fail || r.Header.Get("Authorization") != "MediaBrowser Token="+testKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/Items/1/Images/Primary" {
		// Headers a proxy mustn't pass on, one of them carrying the key, to
		// prove the dashboard builds its own response.
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Set-Cookie", "session="+testKey)
		w.Header().Set("X-Echo-Token", testKey)
		w.Header().Set("Content-Disposition", "attachment")
		_, _ = w.Write([]byte(posterBytes))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/Users" {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "u1", "Name": "sally"},
			{"Id": "u2", "Name": "Anastasia Kowalczyk"},
		})
		return
	}
	items := []map[string]any{
		{"Id": "1", "Name": "Alien", "Type": "Movie", "ProductionYear": 1979,
			"ImageTags":    map[string]string{"Primary": "tag1"},
			"MediaSources": []map[string]any{{"Id": "1", "Size": 15676630630}}},
		{"Id": "2", "Name": "Dr. Strangelove", "Type": "Movie", "ProductionYear": 1964,
			"MediaSources": []map[string]any{{"Id": "2"}}},
	}
	switch r.URL.Query().Get("userId") {
	case "":
	case "u2": // Anastasia has played Alien
		items = items[:1]
	default:
		items = nil
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Items":            items,
		"TotalRecordCount": len(items),
		"StartIndex":       0,
	})
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startApp runs the app against fake and returns its base URL, its log
// output and a stop function that waits for a clean shutdown.
func startApp(t *testing.T, fake *fakeJellyfin) (string, *syncBuffer, func()) {
	t.Helper()
	jf := httptest.NewServer(fake)
	t.Cleanup(jf.Close)

	env := map[string]string{
		"JELLYFIN_URL":     jf.URL,
		"JELLYFIN_API_KEY": testKey,
		"LISTEN_ADDR":      "127.0.0.1:0",
	}
	logs := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, func(k string) string { return env[k] }, logs, func(a net.Addr) { addr <- a })
	}()
	var base string
	select {
	case a := <-addr:
		base = "http://" + a.String()
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("app did not start")
	}
	stop := func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	}
	return base, logs, stop
}

func fetch(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var headers strings.Builder
	_ = resp.Header.Write(&headers)
	return headers.String() + "\n" + string(b)
}

// waitFor polls url until its body contains want.
func waitFor(t *testing.T, url, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		body := fetch(t, url)
		if strings.Contains(body, want) {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never contained %q; last body:\n%s", url, want, body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAppServesMoviesFromJellyfinWithGETOnly(t *testing.T) {
	fake := &fakeJellyfin{}
	base, logs, stop := startApp(t, fake)

	page := waitFor(t, base+"/movies", "Dr. Strangelove")
	for _, want := range []string{"Alien", "1979", "1964",
		`<span class="size__value">14.6</span> <span class="size__unit">GB</span>`, "Unknown",
		`<span class="user-tag__name">Anastasia Kowalczyk</span>`, "Not completed by anyone"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// The page reads from the cache: loading it again doesn't call Jellyfin.
	fake.mu.Lock()
	before := len(fake.methods)
	fake.mu.Unlock()
	fetch(t, base+"/movies")
	fetch(t, base+"/movies")

	var everything strings.Builder
	everything.WriteString(page)
	for _, p := range []string{"/", "/movies", "/static/htmx.min.js", "/static/css/components.css", "/missing"} {
		everything.WriteString(fetch(t, base+p))
	}
	stop()

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.methods) != before {
		t.Errorf("page loads called Jellyfin: %v", fake.methods[before:])
	}
	if len(fake.methods) == 0 {
		t.Fatal("app never called Jellyfin")
	}
	for _, m := range fake.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("non-GET request to Jellyfin: %s", m)
		}
	}
	if strings.Contains(everything.String(), testKey) {
		t.Error("API key appears in a response")
	}
	if strings.Contains(logs.String(), testKey) {
		t.Errorf("API key appears in logs:\n%s", logs)
	}
	if !strings.Contains(logs.String(), "library loaded") {
		t.Errorf("logs missing load message:\n%s", logs)
	}
}

// Posters come from Jellyfin through the dashboard: the browser gets the
// image bytes and the dashboard's own headers, never the key, and repeat
// views are served from the cache.
func TestAppServesPostersWithoutKey(t *testing.T) {
	fake := &fakeJellyfin{}
	base, logs, stop := startApp(t, fake)
	page := waitFor(t, base+"/movies", "Dr. Strangelove")
	if !strings.Contains(page, `src="/posters/1/tag1" alt="Alien"`) {
		t.Fatalf("page has no poster for Alien:\n%s", page)
	}

	var responses []*http.Response
	var bodies []string
	for i := 0; i < 3; i++ {
		resp, err := http.Get(base + "/posters/1/tag1")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		responses = append(responses, resp)
		bodies = append(bodies, string(b))
	}
	// Dr. Strangelove has no poster, and item 3 isn't in the library.
	for _, p := range []string{"/posters/2/anything", "/posters/3/x"} {
		resp, err := http.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", p, resp.StatusCode)
		}
	}
	stop()

	for i, resp := range responses {
		if resp.StatusCode != http.StatusOK || bodies[i] != posterBytes {
			t.Fatalf("poster %d: status %d body %q", i, resp.StatusCode, bodies[i])
		}
		var headers strings.Builder
		_ = resp.Header.Write(&headers)
		if strings.Contains(headers.String(), testKey) || strings.Contains(bodies[i], testKey) {
			t.Errorf("API key in poster response:\n%s", headers.String())
		}
		for _, h := range []string{"Set-Cookie", "X-Echo-Token", "Content-Disposition"} {
			if resp.Header.Get(h) != "" {
				t.Errorf("Jellyfin header %s passed through", h)
			}
		}
		if resp.Header.Get("Content-Type") != "image/jpeg" {
			t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
		}
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	fetched := 0
	for _, m := range fake.methods {
		if strings.Contains(m, "/Images/") {
			fetched++
			if m != "GET /Items/1/Images/Primary" {
				t.Errorf("unexpected image request %s", m)
			}
		}
	}
	if fetched != 1 {
		t.Errorf("Jellyfin served the poster %d times, want 1 (then cached)", fetched)
	}
	if strings.Contains(logs.String(), testKey) {
		t.Errorf("API key in logs:\n%s", logs)
	}
}

func TestAppLogsFailedRefreshWithoutKey(t *testing.T) {
	fake := &fakeJellyfin{fail: true}
	base, logs, stop := startApp(t, fake)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "library refresh failed") {
		if time.Now().After(deadline) {
			t.Fatalf("no failure logged:\n%s", logs)
		}
		time.Sleep(20 * time.Millisecond)
	}
	page := fetch(t, base+"/movies")
	stop()
	if !strings.Contains(logs.String(), "401") {
		t.Errorf("failure log lacks status:\n%s", logs)
	}
	if strings.Contains(logs.String(), testKey) || strings.Contains(page, testKey) {
		t.Errorf("API key leaked:\n%s\n%s", logs, page)
	}
}

func TestRunFailsNamingMissingVariables(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" }, io.Discard, nil)
	if err == nil {
		t.Fatal("want error")
	}
	for _, name := range []string{"JELLYFIN_URL", "JELLYFIN_API_KEY"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}
