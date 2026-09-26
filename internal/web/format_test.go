package web

import (
	"testing"
	"time"
)

func TestFormatNumber(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 1284: "1,284", 1234567: "1,234,567"} {
		if got := formatNumber(n); got != want {
			t.Errorf("formatNumber(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 movies", 1: "1 movie", 1284: "1,284 movies"} {
		if got := formatCount(n, "movie", "movies"); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatAdded(t *testing.T) {
	if got := formatAdded(time.Date(2025, 6, 3, 1, 0, 0, 0, time.UTC)); got != "Added 3 Jun 2025" {
		t.Errorf("formatAdded = %q", got)
	}
	if got := formatAdded(time.Time{}); got != "" {
		t.Errorf("formatAdded(zero) = %q, want empty", got)
	}
}

func TestPosterInitial(t *testing.T) {
	for title, want := range map[string]string{
		"2001: A Space Odyssey":  "2",
		"aftersun":               "A",
		"The Simpsons":           "S", // Jellyfin's sort name drops leading articles
		"A Quiet Place":          "Q",
		"An Education":           "E",
		"Theodore":               "T",
		"...And Justice for All": "A",
		"Élite":                  "É",
		"":                       "",
		"!!!":                    "",
	} {
		if got := posterInitial(title); got != want {
			t.Errorf("posterInitial(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestFormatSizeUsesBinaryUnitsAndOneDecimal(t *testing.T) {
	const kib, mib, gib, tib = 1 << 10, 1 << 20, 1 << 30, 1 << 40
	for _, c := range []struct {
		bytes       int64
		value, unit string
	}{
		{146 * gib / 10, "14.6", "GB"},
		{798 * gib / 10, "79.8", "GB"},
		{12 * gib, "12.0", "GB"},
		{gib, "1.0", "GB"},
		{gib - 1, "1.0", "GB"},              // 1023.99… MB rounds up into the next unit
		{1000 * 1000 * 1000, "953.7", "MB"}, // a decimal GB is not a GB
		{194 * tib / 100, "1.9", "TB"},
		{700 * mib, "700.0", "MB"},
		{1536, "1.5", "KB"},
		{512, "512", "B"},
		{20559445148, "19.1", "GB"}, // a real Jellyfin 12.1.0 file
		{kib * tib, "1024.0", "TB"}, // TB is the largest unit
	} {
		v, u := formatSize(c.bytes)
		if v != c.value || u != c.unit {
			t.Errorf("formatSize(%d) = %q %q, want %q %q", c.bytes, v, u, c.value, c.unit)
		}
	}
}

func TestUserCode(t *testing.T) {
	for name, want := range map[string]string{
		"Sam":                   "SA",
		"sally":                 "SA",
		"Anastasia Kowalczyk":   "AK",
		"mary jane watson":      "MW",
		"H":                     "H",
		"home-tv":               "HO",
		"  Priya  ":             "PR",
		"Élodie":                "ÉL",
		"Christopher-Alexander": "CH",
		"":                      "",
	} {
		if got := userCode(name); got != want {
			t.Errorf("userCode(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFormatAgo(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "less than a minute ago"}, // clock skew
		{0, "less than a minute ago"},
		{59 * time.Second, "less than a minute ago"},
		{time.Minute, "1 minute ago"},
		{12*time.Minute + 59*time.Second, "12 minutes ago"},
		{59 * time.Minute, "59 minutes ago"},
		{time.Hour, "1 hour ago"},
		{3*time.Hour + 59*time.Minute, "3 hours ago"},
		{23 * time.Hour, "23 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{50 * time.Hour, "2 days ago"},
	} {
		if got := formatAgo(tc.d); got != tc.want {
			t.Errorf("formatAgo(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	now := time.Date(2026, 9, 26, 14, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 9, 26, 11, 42, 0, 0, time.UTC), "today at 11:42"},
		{time.Date(2026, 9, 26, 0, 5, 0, 0, time.UTC), "today at 00:05"},
		{time.Date(2026, 9, 25, 23, 59, 0, 0, time.UTC), "yesterday at 23:59"},
		{time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC), "on 2 Sep at 09:00"},
		{time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC), "on 31 Dec 2025 at 09:00"},
	} {
		if got := formatClock(tc.t, now); got != tc.want {
			t.Errorf("formatClock(%v) = %q, want %q", tc.t, got, tc.want)
		}
	}
}
