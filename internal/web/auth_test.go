package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"jellyfin-watched/internal/library"
)

const testPassword = "correct horse battery staple"

// clock is a settable test clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// gatedLibrary has a movie with a poster and a series, so every route has
// something to serve.
func gatedLibrary() *stubLibrary {
	lib := loadedLibrary(12*time.Minute, library.Movie{ID: "a1", Title: "Alien", Year: 1979, PosterTag: "t1"})
	lib.status.Snapshot.Series = []library.Series{{ID: "s1", Title: "Andor", Year: 2022}}
	return lib
}

// newGatedHandler returns a handler with password set ("" for no gate), its
// clock and its log output.
func newGatedHandler(t *testing.T, password string) (http.Handler, *clock, *bytes.Buffer) {
	t.Helper()
	c := &clock{t: testNow}
	var logs bytes.Buffer
	h, err := newHandler(gatedLibrary(), &stubPosters{}, slog.New(slog.NewTextHandler(&logs, nil)), c.now, password)
	if err != nil {
		t.Fatal(err)
	}
	return h, c, &logs
}

// gatedRoutes is every route the app serves, bar the password screen and
// static assets: pages, htmx fragments, actions and images.
var gatedRoutes = []struct{ method, path string }{
	{"GET", "/"},
	{"GET", "/movies"},
	{"GET", "/movies?q=alien&sort=year"},
	{"GET", "/tv"},
	{"GET", "/tv?seasons=all"},
	{"GET", "/tv/series/s1?seasons=show"},
	{"GET", "/refresh?since=g3"},
	{"POST", "/refresh"},
	{"GET", "/posters/a1/t1"},
	{"HEAD", "/posters/a1/t1"},
	{"GET", "/no-such-page"},
}

// login posts password to the password screen and returns the response.
func login(t *testing.T, h http.Handler, password, next string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"password": {password}}
	if next != "" {
		form.Set("next", next)
	}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// sessionCookie logs in and returns the session cookie.
func sessionCookie(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	rec := login(t, h, testPassword, "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("no session cookie after logging in: status %d", rec.Code)
	return nil
}

func doWithCookie(t *testing.T, h http.Handler, method, path string, c *http.Cookie, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNoPasswordNoGate(t *testing.T) {
	h, _, _ := newGatedHandler(t, "")
	for _, r := range gatedRoutes {
		rec := doWithCookie(t, h, r.method, r.path, nil, nil)
		if rec.Code == http.StatusUnauthorized || strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			t.Errorf("%s %s: status %d Location %q: gated without a password", r.method, r.path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := doWithCookie(t, h, "GET", "/movies", nil, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Alien") {
		t.Errorf("/movies: status %d", rec.Code)
	}
	// There's no password screen to show.
	rec := doWithCookie(t, h, "GET", "/login", nil, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/movies" {
		t.Errorf("/login: status %d Location %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := login(t, h, "anything", ""); rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 0 {
		t.Errorf("POST /login: status %d cookies %v", rec.Code, rec.Result().Cookies())
	}
}

// Without a session, a normal request is sent to the password screen,
// which returns to it afterwards; an htmx request gets a 401 and tells htmx
// to load the password screen as a whole page, so the screen never lands in
// a fragment's slot.
func TestGateRedirectsOrRefusesEveryRoute(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	for _, r := range gatedRoutes {
		rec := doWithCookie(t, h, r.method, r.path, nil, nil)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s %s: status %d, want 303", r.method, r.path, rec.Code)
		}
		want := "/login?next=" + url.QueryEscape(r.path)
		if r.method != "GET" && r.method != "HEAD" {
			want = "/login"
		}
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("%s %s: Location %q, want %q", r.method, r.path, got, want)
		}
		assertNotContains(t, rec.Body.String(), "Alien", "Andor", "RIFF")
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("%s %s: Cache-Control %q", r.method, r.path, cc)
		}

		rec = doWithCookie(t, h, r.method, r.path, nil, map[string]string{
			"HX-Request": "true", "HX-Current-URL": "http://dash.local:8080/tv?q=an"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("htmx %s %s: status %d, want 401", r.method, r.path, rec.Code)
		}
		if got := rec.Header().Get("HX-Redirect"); got != "/login?next=%2Ftv%3Fq%3Dan" {
			t.Errorf("htmx %s %s: HX-Redirect %q", r.method, r.path, got)
		}
		if rec.Header().Get("HX-Trigger") != "" || strings.Contains(rec.Body.String(), "<") {
			t.Errorf("htmx %s %s: response has something to swap or trigger: %q", r.method, r.path, rec.Body)
		}
	}
}

// The password screen's own stylesheets, fonts and icon load without a
// session.
func TestPasswordScreenAssetsArePublic(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	rec := doWithCookie(t, h, "GET", "/login?next=%2Ftv", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/login: status %d", rec.Code)
	}
	for _, path := range []string{"/static/css/tokens.css", "/static/css/components.css", "/static/favicon.svg",
		"/static/fonts/instrument-sans-latin-wght-normal.woff2"} {
		if !strings.Contains(rec.Body.String(), strings.TrimPrefix(path, "/static/")) && !strings.Contains(path, "fonts") {
			t.Errorf("password screen doesn't reference %s", path)
		}
		if rec := doWithCookie(t, h, "GET", path, nil, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s without a session: status %d", path, rec.Code)
		}
	}
}

func TestPasswordScreen(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	rec := doWithCookie(t, h, "GET", "/login?next=%2Ftv%3Fq%3Dan", nil, nil)
	body := rec.Body.String()
	assertContains(t, body,
		"<title>Enter the password · Spool</title>",
		`<form class="password-form" method="post" action="/login"`,
		"Enter the password",
		"This dashboard is password protected. The password is set in the server configuration.",
		`<label class="password-form__label" for="password">Password</label>`,
		`type="password"`, `name="password"`, `autocomplete="current-password"`, "autofocus", "required",
		`<input type="hidden" name="next" value="/tv?q=an">`,
		`<button type="submit" class="button button--primary"`, "Open dashboard</button>")
	assertNotContains(t, body, "password-form__error", "aria-invalid", "app-header", "htmx.min.js", testPassword)
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q", cc)
	}
}

func TestWrongPasswordShowsErrorState(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	for _, wrong := range []string{"", "correct horse", testPassword + " ", "CORRECT HORSE BATTERY STAPLE"} {
		rec := login(t, h, wrong, "/tv")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%q: status %d, want 401", wrong, rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%q: set a cookie", wrong)
		}
		body := rec.Body.String()
		assertContains(t, body, `aria-invalid="true"`, `aria-describedby="password-error"`,
			`<p class="password-form__error" id="password-error">`, "That password is not correct. Check it and try again.",
			`<input type="hidden" name="next" value="/tv">`)
		// What was typed isn't echoed back into the page.
		if wrong != "" {
			assertNotContains(t, body, `value="`+wrong)
		}
	}
	// Still no way in.
	if rec := doWithCookie(t, h, "GET", "/movies", nil, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("/movies after wrong passwords: status %d", rec.Code)
	}
}

func TestRightPasswordStartsSession(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	rec := login(t, h, testPassword, "/tv?q=an")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/tv?q=an" {
		t.Fatalf("status %d Location %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies %v", cookies)
	}
	c := cookies[0]
	if c.Name != sessionCookieName || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge <= 0 {
		t.Errorf("cookie %+v: want HttpOnly, SameSite=Lax, Path=/, a max age", c)
	}
	if c.Secure {
		t.Error("Secure over plain HTTP: the browser would never send it back")
	}
	if strings.Contains(c.Value, testPassword) || strings.Contains(c.Value, url.QueryEscape(testPassword)) {
		t.Error("cookie carries the password")
	}

	for _, r := range gatedRoutes {
		rec := doWithCookie(t, h, r.method, r.path, c, nil)
		if rec.Code == http.StatusUnauthorized || strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			t.Errorf("%s %s with a session: status %d Location %q", r.method, r.path, rec.Code, rec.Header().Get("Location"))
		}
		rec = doWithCookie(t, h, r.method, r.path, c, map[string]string{"HX-Request": "true"})
		if rec.Code == http.StatusUnauthorized || rec.Header().Get("HX-Redirect") != "" {
			t.Errorf("htmx %s %s with a session: status %d", r.method, r.path, rec.Code)
		}
	}
	if rec := doWithCookie(t, h, "GET", "/movies", c, nil); !strings.Contains(rec.Body.String(), "Alien") {
		t.Error("/movies with a session doesn't list the library")
	}
	if rec := doWithCookie(t, h, "GET", "/posters/a1/t1", c, nil); rec.Code != http.StatusOK || rec.Body.String() != "RIFF-webp-a1" {
		t.Errorf("poster with a session: status %d", rec.Code)
	}
	// Signed in already: the password screen moves on.
	if rec := doWithCookie(t, h, "GET", "/login?next=%2Ftv", c, nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/tv" {
		t.Errorf("/login with a session: status %d Location %q", rec.Code, rec.Header().Get("Location"))
	}
}

// Behind an HTTPS reverse proxy the cookie is also Secure.
func TestSessionCookieSecureBehindHTTPS(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	req := httptest.NewRequest("POST", "/login", strings.NewReader("password="+url.QueryEscape(testPassword)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if c := rec.Result().Cookies(); len(c) != 1 || !c[0].Secure {
		t.Errorf("cookies %v, want one Secure cookie", c)
	}
}

// Posters are private once there's a password: no shared cache may keep
// them.
func TestGatedPostersArePrivate(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	rec := doWithCookie(t, h, "GET", "/posters/a1/t1", sessionCookie(t, h), nil)
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=31536000, immutable" {
		t.Errorf("Cache-Control %q", cc)
	}
}

func TestSessionRejectsForgedTamperedAndExpiredCookies(t *testing.T) {
	h, c, _ := newGatedHandler(t, testPassword)
	good := sessionCookie(t, h)
	expiry, mac, _ := strings.Cut(good.Value, ".")

	other, _, _ := newGatedHandler(t, testPassword) // another run of the app: another key
	for name, value := range map[string]string{
		"empty":            "",
		"garbage":          "not-a-session",
		"no mac":           expiry,
		"empty mac":        expiry + ".",
		"extended expiry":  "9999999999." + mac,
		"flipped mac":      expiry + "." + flip(mac),
		"password":         testPassword,
		"other app's key":  sessionCookie(t, other).Value,
		"trailing garbage": good.Value + "x",
	} {
		rec := doWithCookie(t, h, "GET", "/movies", &http.Cookie{Name: sessionCookieName, Value: value}, nil)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s: status %d, want 303", name, rec.Code)
		}
	}
	if rec := doWithCookie(t, h, "GET", "/movies", good, nil); rec.Code != http.StatusOK {
		t.Fatalf("good cookie: status %d", rec.Code)
	}
	c.advance(sessionTTL - time.Minute)
	if rec := doWithCookie(t, h, "GET", "/movies", good, nil); rec.Code != http.StatusOK {
		t.Errorf("just before expiry: status %d", rec.Code)
	}
	c.advance(2 * time.Minute)
	if rec := doWithCookie(t, h, "GET", "/movies", good, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("after expiry: status %d, want 303", rec.Code)
	}
}

func flip(s string) string {
	b := []byte(s)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

// ?next= only ever leads back into the dashboard.
func TestNextStaysOnTheDashboard(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	for next, want := range map[string]string{
		"":                    "/movies",
		"/tv?q=an&sort=year":  "/tv?q=an&sort=year",
		"https://evil.test/":  "/movies",
		"//evil.test/":        "/movies",
		"/\\evil.test/":       "/movies",
		"\\\\evil.test":       "/movies",
		"/login":              "/movies",
		"/login?next=/login":  "/movies",
		"javascript:alert(1)": "/movies",
		"movies":              "/movies",
		"/\t/evil.test":       "/movies",
	} {
		rec := login(t, h, testPassword, next)
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("next %q: Location %q, want %q", next, got, want)
		}
	}
	rec := doWithCookie(t, h, "GET", "/login?next="+url.QueryEscape("//evil.test/"), nil, nil)
	assertContains(t, rec.Body.String(), `<input type="hidden" name="next" value="/movies">`)
}

// Wrong guesses are limited across all clients; once the budget is spent
// even the right password waits, and the budget refills over time.
func TestWrongPasswordsAreRateLimited(t *testing.T) {
	h, c, _ := newGatedHandler(t, testPassword)
	for i := 0; i < loginBurst; i++ {
		if rec := login(t, h, "guess", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: status %d, want 401", i+1, rec.Code)
		}
	}
	rec := login(t, h, testPassword, "")
	if rec.Code != http.StatusTooManyRequests || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("right password after the burst: status %d cookies %v, want 429 and none", rec.Code, rec.Result().Cookies())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	assertContains(t, rec.Body.String(), "Too many wrong passwords. Wait a minute and try again.", `aria-invalid="true"`)

	c.advance(loginRefill)
	if rec := login(t, h, testPassword, ""); rec.Code != http.StatusSeeOther {
		t.Errorf("after the budget refilled: status %d, want 303", rec.Code)
	}
	// Right passwords don't use up the budget.
	for i := 0; i < 2*loginBurst; i++ {
		if rec := login(t, h, testPassword, ""); rec.Code != http.StatusSeeOther {
			t.Fatalf("right password %d: status %d", i+1, rec.Code)
		}
	}
}

func TestPasswordNeverLogged(t *testing.T) {
	h, _, logs := newGatedHandler(t, testPassword)
	login(t, h, testPassword+"x", "")
	login(t, h, testPassword, "")
	for i := 0; i < loginBurst+1; i++ {
		login(t, h, testPassword+"y", "")
	}
	if !strings.Contains(logs.String(), "wrong dashboard password") {
		t.Errorf("wrong passwords weren't logged at all: %q", logs)
	}
	for _, s := range []string{"correct horse", url.QueryEscape(testPassword)} {
		if strings.Contains(logs.String(), s) {
			t.Errorf("logs contain the password: %q", logs)
		}
	}
}

// The login form is small; a huge body is refused rather than read.
func TestLoginBodyIsLimited(t *testing.T) {
	h, _, _ := newGatedHandler(t, testPassword)
	body := "password=" + strings.Repeat("a", 1<<20)
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusSeeOther || len(rec.Result().Cookies()) != 0 {
		t.Errorf("status %d cookies %v", rec.Code, rec.Result().Cookies())
	}
}
