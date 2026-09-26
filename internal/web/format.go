package web

import "strconv"

// formatYear renders a production year, or "" when Jellyfin has none.
func formatYear(y int) string {
	if y <= 0 {
		return ""
	}
	return strconv.Itoa(y)
}

// formatCount renders "1 movie" / "28 movies".
func formatCount(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}
