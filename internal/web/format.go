package web

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// formatYear renders a production year, or "" when Jellyfin has none.
func formatYear(y int) string {
	if y <= 0 {
		return ""
	}
	return strconv.Itoa(y)
}

// formatNumber renders n with thousands separators: "1,284".
func formatNumber(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// formatCount renders "1 movie" / "1,284 movies".
func formatCount(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return formatNumber(n) + " " + plural
}

// formatAdded renders the date an item was added: "Added 14 Feb 2024", or ""
// when unknown.
func formatAdded(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return "Added " + t.Format("2 Jan 2006")
}

// posterInitial is the character shown on a placeholder poster: the first
// letter or digit of the title, skipping a leading "The", "A" or "An" the way
// Jellyfin's sort name does, so initials follow the list's order.
func posterInitial(title string) string {
	for _, article := range []string{"The ", "A ", "An "} {
		if len(title) > len(article) && strings.EqualFold(title[:len(article)], article) {
			title = title[len(article):]
			break
		}
	}
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return ""
}
