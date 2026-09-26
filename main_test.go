package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// renamed, if set, is Alien's title: a change made in Jellyfin.
	renamed string
}

func (f *fakeJellyfin) set(fn func(*fakeJellyfin)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.methods = append(f.methods, r.Method+" "+r.URL.Path)
	fail, alien := f.fail, f.renamed
	f.mu.Unlock()
	if alien == "" {
		alien = "Alien"
	}
	if fail || r.Header.Get("Authorization") != "MediaBrowser Token="+testKey {
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
	q := r.URL.Query()
	var items []map[string]any
	switch q.Get("includeItemTypes") {
	case "Movie":
		items = []map[string]any{
			{"Id": "1", "Name": alien, "Type": "Movie", "ProductionYear": 1979,
				"ImageTags":    map[string]string{"Primary": "tag1"},
				"MediaSources": []map[string]any{{"Id": "1", "Size": 15676630630}}},
			{"Id": "2", "Name": "Dr. Strangelove", "Type": "Movie", "ProductionYear": 1964,
				"MediaSources": []map[string]any{{"Id": "2"}}},
		}
		switch q.Get("userId") {
		case "":
		case "u2": // Anastasia has played Alien
			items = items[:1]
		default:
			items = nil
		}
	case "Series":
		items = []map[string]any{{"Id": "sr1", "Name": "Andor", "Type": "Series", "ProductionYear": 2022,
			"ImageTags": map[string]string{"Primary": "tag-sr1"}}}
	case "Season":
		items = []map[string]any{
			{"Id": "se1", "Name": "Season 1", "Type": "Season", "SeriesId": "sr1", "IndexNumber": 1},
			{"Id": "se0", "Name": "Specials", "Type": "Season", "SeriesId": "sr1", "IndexNumber": 0},
		}
	case "Episode":
		// Anastasia has watched Season 1; sally one of its two episodes.
		played := map[string][]bool{"": {false, false, false}, "u1": {true, false, false}, "u2": {true, true, false}}[q.Get("userId")]
		for i, ep := range []map[string]any{
			{"Id": "e1", "SeriesId": "sr1", "SeasonId": "se1"},
			{"Id": "e2", "SeriesId": "sr1", "SeasonId": "se1"},
			{"Id": "e0", "SeriesId": "sr1", "SeasonId": "se0"},
		} {
			ep["Type"] = "Episode"
			if q.Get("userId") != "" {
				ep["UserData"] = map[string]any{"Played": played[i]}
			} else {
				ep["MediaSources"] = []map[string]any{{"Size": 1 << 30}}
			}
			items = append(items, ep)
		}
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
	return startAppWithEnv(t, fake, nil)
}

// startAppWithEnv is startApp with extra environment variables.
func startAppWithEnv(t *testing.T, fake *fakeJellyfin, extra map[string]string) (string, *syncBuffer, func()) {
	t.Helper()
	jf := httptest.NewServer(fake)
	t.Cleanup(jf.Close)

	env := map[string]string{
		"JELLYFIN_URL":     jf.URL,
		"JELLYFIN_API_KEY": testKey,
		"LISTEN_ADDR":      "127.0.0.1:0",
	}
	for k, v := range extra {
		env[k] = v
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

func TestAppServesTVShowsWithGETOnly(t *testing.T) {
	fake := &fakeJellyfin{}
	base, logs, stop := startApp(t, fake)

	page := waitFor(t, base+"/tv?seasons=all", "Andor")
	fragment := fetch(t, base+"/tv/series/sr1?seasons=hide")
	stop()
	for _, want := range []string{
		`<h3 class="season-row__name">Season 1</h3>`, `<span class="season-row__episodes">2 episodes</span>`,
		// Season 1 is 2 GiB; the series adds the hidden Specials.
		`<span class="size__value">2.0</span>`, `<span class="size__value">3.0</span>`,
		`<span class="user-tag__name">Anastasia Kowalczyk</span>`,
		`<span class="user-tag__count" aria-hidden="true">1 / 2</span>`,
		`src="/posters/sr1/tag-sr1"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("TV page missing %q", want)
		}
	}
	if strings.Contains(page, "Specials") {
		t.Error("Specials shown without INCLUDE_SPECIALS")
	}
	if !strings.Contains(fragment, "Show 1 season") {
		t.Errorf("fragment:\n%s", fragment)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, m := range fake.methods {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("non-GET request to Jellyfin: %s", m)
		}
	}
	if strings.Contains(page+fragment+logs.String(), testKey) {
		t.Error("API key appears in a response or the logs")
	}
}

func TestAppShowsSpecialsWhenIncluded(t *testing.T) {
	base, _, stop := startAppWithEnv(t, &fakeJellyfin{}, map[string]string{"INCLUDE_SPECIALS": "true"})
	page := waitFor(t, base+"/tv?seasons=all", "Andor")
	stop()
	if !strings.Contains(page, `<h3 class="season-row__name">Specials</h3>`) {
		t.Error("Specials hidden with INCLUDE_SPECIALS=true")
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

func post(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Post(url, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status %d", url, resp.StatusCode)
	}
	return string(b)
}

// "Refresh now" fetches changes from Jellyfin; when Jellyfin then goes
// away, the last good data stays with a notice, and the failure is logged
// without the key.
func TestAppRefreshNowAndStaleData(t *testing.T) {
	fake := &fakeJellyfin{}
	base, logs, stop := startApp(t, fake)
	defer stop()
	waitFor(t, base+"/movies", "Alien")
	waitFor(t, base+"/refresh?since=g1", "Refresh now")

	fake.set(func(f *fakeJellyfin) { f.renamed = "Aliens" })
	if body := post(t, base+"/refresh"); !strings.Contains(body, "Refresh") {
		t.Errorf("POST /refresh:\n%s", body)
	}
	waitFor(t, base+"/movies", ">Aliens<")
	// The page's poll learns of the new data.
	resp, err := http.Get(base + "/refresh?since=g1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("HX-Trigger") != "library-updated" {
		t.Errorf("poll after refresh: HX-Trigger %q", resp.Header.Get("HX-Trigger"))
	}

	fake.set(func(f *fakeJellyfin) { f.fail, f.renamed = true, "Alien³" })
	post(t, base+"/refresh")
	page := waitFor(t, base+"/movies", "Can&#39;t reach Jellyfin. Showing data from less than a minute ago.")
	if !strings.Contains(page, ">Aliens<") {
		t.Error("last good data not kept")
	}
	if strings.Contains(page+logs.String(), testKey) {
		t.Error("API key in page or logs")
	}
	if n := strings.Count(logs.String(), "library refresh failed"); n != 1 {
		t.Errorf("%d failures logged, want 1:\n%s", n, logs)
	}
}

// With DASHBOARD_PASSWORD set, nothing is served without the password;
// with it, the dashboard works as before. Neither the password nor the key
// reaches a response or the logs.
func TestAppPasswordGate(t *testing.T) {
	const password = "s3cret-dashboard-pw-71c"
	fake := &fakeJellyfin{}
	base, logs, stop := startAppWithEnv(t, fake, map[string]string{"DASHBOARD_PASSWORD": password})
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var everything strings.Builder
	record := func(resp *http.Response) string {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		_ = resp.Header.Write(&everything)
		everything.Write(b)
		return string(b)
	}
	for _, p := range []string{"/movies", "/tv", "/posters/1/tag1", "/refresh?since=g1", "/tv/series/sr1"} {
		resp, err := noRedirect.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		record(resp)
		if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login?next=") {
			t.Errorf("GET %s without a session: status %d Location %q", p, resp.StatusCode, resp.Header.Get("Location"))
		}
	}

	resp, err := noRedirect.PostForm(base+"/login", url.Values{"password": {"wrong"}, "next": {"/movies"}})
	if err != nil {
		t.Fatal(err)
	}
	if body := record(resp); resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "That password is not correct.") {
		t.Errorf("wrong password: status %d", resp.StatusCode)
	}

	resp, err = noRedirect.PostForm(base+"/login", url.Values{"password": {password}, "next": {"/movies"}})
	if err != nil {
		t.Fatal(err)
	}
	record(resp)
	cookies := resp.Cookies()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/movies" || len(cookies) != 1 {
		t.Fatalf("right password: status %d Location %q cookies %v", resp.StatusCode, resp.Header.Get("Location"), cookies)
	}
	withSession := func(p string) string {
		t.Helper()
		req, _ := http.NewRequest("GET", base+p, nil)
		req.AddCookie(cookies[0])
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body := record(resp)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with a session: status %d", p, resp.StatusCode)
		}
		return body
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(withSession("/movies"), "Dr. Strangelove") {
		if time.Now().After(deadline) {
			t.Fatal("library never loaded")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if withSession("/posters/1/tag1") != posterBytes {
		t.Error("poster with a session")
	}
	stop()
	stopped = true

	if !strings.Contains(logs.String(), "password_required=true") {
		t.Errorf("logs don't say a password is required:\n%s", logs)
	}
	for _, secret := range []string{password, testKey} {
		if strings.Contains(everything.String(), secret) {
			t.Errorf("%q appears in a response", secret)
		}
		if strings.Contains(logs.String(), secret) {
			t.Errorf("%q appears in the logs:\n%s", secret, logs)
		}
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
