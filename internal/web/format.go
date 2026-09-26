package web

import (
	"math"
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

// sizeUnits are binary: each is 1024 of the one before (1 GB = 1024³ bytes),
// labelled the way the design writes them.
var sizeUnits = []string{"B", "KB", "MB", "GB", "TB"}

// formatSize renders a byte count as a value with one decimal and its unit,
// e.g. "14.6", "GB". Whole bytes have no decimal. The unit is the largest in
// which the rounded value is at least 1, so 1023.99 MB becomes "1.0" GB.
func formatSize(bytes int64) (value, unit string) {
	if bytes < 1024 {
		return strconv.FormatInt(bytes, 10), sizeUnits[0]
	}
	v := float64(bytes)
	i := 0
	for i < len(sizeUnits)-1 && math.Round(v*10)/10 >= 1024 {
		v /= 1024
		i++
	}
	return strconv.FormatFloat(v, 'f', 1, 64), sizeUnits[i]
}

// userCode is a user's two-letter code: the first two letters of a one-word
// name ("Sam" → "SA"), or the initials of the first and last words of a
// longer one ("Anastasia Kowalczyk" → "AK"). A one-letter name gives one
// letter. Only spaces separate words, so "home-tv" is "HO".
func userCode(name string) string {
	words := strings.Fields(name)
	switch len(words) {
	case 0:
		return ""
	case 1:
		r := []rune(words[0])
		if len(r) > 2 {
			r = r[:2]
		}
		return strings.ToUpper(string(r))
	default:
		first := []rune(words[0])[0]
		last := []rune(words[len(words)-1])[0]
		return strings.ToUpper(string([]rune{first, last}))
	}
}
