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
