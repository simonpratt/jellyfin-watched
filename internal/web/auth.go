package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The optional password gate (DASHBOARD_PASSWORD). With a password set,
// every route but the password screen and the static assets it needs
// requires a session: a signed cookie the password screen sets.

const (
	loginPath         = "/login"
	sessionCookieName = "jellyfin_watched_session"
	// sessionTTL is how long a session lasts before the password is asked
	// for again.
	sessionTTL = 30 * 24 * time.Hour
	// Wrong passwords are limited across all clients: loginBurst straight
	// away, then one more every loginRefill.
	loginBurst  = 10
	loginRefill = 6 * time.Second
	// maxLoginBody is far more than the form's two fields need.
	maxLoginBody = 4 << 10
)

// gate checks the password and issues and checks sessions.
type gate struct {
	// digest is the SHA-256 of the password: comparing digests in constant
	// time doesn't leak the password's length either.
	digest [sha256.Size]byte
	// key signs sessions. It is random per process, so a restart signs
	// everyone out, and a stolen cookie can't be used to guess the
	// password offline.
	key      []byte
	now      func() time.Time
	attempts *limiter
	log      *slog.Logger
}

func newGate(password string, now func() time.Time, log *slog.Logger) (*gate, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &gate{
		digest:   sha256.Sum256([]byte(password)),
		key:      key,
		now:      now,
		attempts: &limiter{tokens: loginBurst, burst: loginBurst, every: loginRefill, last: now()},
		log:      log,
	}, nil
}

// correct reports, in constant time, whether password is the password.
func (g *gate) correct(password string) bool {
	d := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(d[:], g.digest[:]) == 1
}

// A session cookie's value is "<expiry, Unix seconds>.<HMAC-SHA256 of the
// expiry>", the MAC in unpadded base64url.
func (g *gate) sign(expiry string) string {
	m := hmac.New(sha256.New, g.key)
	m.Write([]byte("jellyfin-watched-session-v1." + expiry))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (g *gate) newSession(r *http.Request) *http.Cookie {
	expires := g.now().Add(sessionTTL)
	exp := strconv.FormatInt(expires.Unix(), 10)
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    exp + "." + g.sign(exp),
		Path:     "/",
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		// Lax: following a link to the dashboard keeps the session, but
		// another site can't POST to it (e.g. "Refresh now") with it.
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	}
}

// isHTTPS reports whether the browser reached the dashboard over HTTPS,
// directly or through a reverse proxy. Trusting the proxy header only ever
// makes the cookie stricter.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (g *gate) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	exp, mac, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	want := g.sign(exp)
	if !hmac.Equal([]byte(mac), []byte(want)) {
		return false
	}
	unix, err := strconv.ParseInt(exp, 10, 64)
	return err == nil && g.now().Before(time.Unix(unix, 0))
}

// protect wraps the dashboard: only the password screen and static assets
// are served without a session.
func (g *gate) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath || strings.HasPrefix(r.URL.Path, "/static/") || g.hasSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		// htmx: refuse, and have htmx load the password screen as a whole
		// page (HX-Redirect is acted on whatever the status), returning to
		// the page the request came from. Nothing is swapped into the
		// fragment's slot.
		if r.Header.Get("HX-Request") == "true" {
			back := ""
			if u, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil {
				back = u.RequestURI()
			}
			w.Header().Set("HX-Redirect", loginURL(back))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		back := ""
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			back = r.URL.RequestURI()
		}
		http.Redirect(w, r, loginURL(back), http.StatusSeeOther)
	})
}

// loginURL is the password screen, returning to back afterwards.
func loginURL(back string) string {
	if back = safeNext(back); back == "" {
		return loginPath
	}
	return loginPath + "?next=" + url.QueryEscape(back)
}

// safeNext returns next if it is a path on the dashboard to return to
// after the password screen, or "" (anywhere else, including another host,
// or the password screen itself).
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == loginPath || strings.ContainsAny(next, "\t\n\r\\") {
		return ""
	}
	return next
}

// nextOrHome is where to go after the password screen.
func nextOrHome(next string) string {
	if next = safeNext(next); next == "" {
		return "/movies"
	}
	return next
}

// limiter is a token bucket shared by every client.
type limiter struct {
	mu     sync.Mutex
	tokens float64
	burst  float64
	every  time.Duration
	last   time.Time
}

// take spends a token, or reports how long until there is one.
func (l *limiter) take(now time.Time) (ok bool, wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.After(l.last) {
		l.tokens = math.Min(l.burst, l.tokens+float64(now.Sub(l.last))/float64(l.every))
		l.last = now
	}
	if l.tokens >= 1 {
		l.tokens--
		return true, 0
	}
	return false, time.Duration((1 - l.tokens) * float64(l.every))
}

// refund returns a token spent on the right password.
func (l *limiter) refund() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens = math.Min(l.burst, l.tokens+1)
}

// passwordScreen serves the password screen, or, with a session already,
// moves on to where it would lead.
func (h *handler) passwordScreen(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if h.gate == nil || h.gate.hasSession(r) {
		http.Redirect(w, r, nextOrHome(next), http.StatusSeeOther)
		return
	}
	h.renderPasswordPage(w, http.StatusOK, newPasswordPage(next, ""))
}

// login checks the password. The right one starts a session and returns to
// the page that asked for it; a wrong one shows the screen's error state.
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "The form couldn't be read.", http.StatusBadRequest)
		return
	}
	next := r.PostForm.Get("next")
	if h.gate == nil {
		http.Redirect(w, r, nextOrHome(next), http.StatusSeeOther)
		return
	}
	ok, wait := h.gate.attempts.take(h.now())
	if !ok {
		h.log.Warn("too many wrong dashboard passwords; refusing attempts for now", "remote", r.RemoteAddr)
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		h.renderPasswordPage(w, http.StatusTooManyRequests, newPasswordPage(next, tooManyPassword))
		return
	}
	if !h.gate.correct(r.PostForm.Get("password")) {
		h.log.Info("wrong dashboard password", "remote", r.RemoteAddr)
		h.renderPasswordPage(w, http.StatusUnauthorized, newPasswordPage(next, wrongPassword))
		return
	}
	h.gate.attempts.refund()
	http.SetCookie(w, h.gate.newSession(r))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, nextOrHome(next), http.StatusSeeOther)
}

func (h *handler) renderPasswordPage(w http.ResponseWriter, status int, p PasswordPage) {
	w.Header().Set("Cache-Control", "no-store")
	h.renderStatus(w, status, "password", "password-page", p)
}
