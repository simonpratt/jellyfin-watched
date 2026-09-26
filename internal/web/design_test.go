package web

import (
	"io/fs"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// tokens.css is the designer's file with exactly two changes: dark only, and
// no light-theme block (CLAUDE.md "Dark mode only").
func TestTokensCSSIsDarkOnlyCopyOfDesignTokens(t *testing.T) {
	design, err := os.ReadFile("../../designs/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile(assets, "static/css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(design), "color-scheme: dark light;", "color-scheme: dark;", 1)
	want = regexp.MustCompile(`(?s)@media \(prefers-color-scheme: light\) \{\n.*?\n\}\n\n`).ReplaceAllString(want, "")
	if string(got) != want {
		t.Error("static/css/tokens.css differs from designs/tokens.css beyond the two dark-only changes; re-copy it")
	}
	if !strings.Contains(string(got), "color-scheme: dark;") {
		t.Error("tokens.css does not set color-scheme: dark")
	}
	if !regexp.MustCompile(`@media \(prefers-reduced-motion: reduce\) \{\s*:root \{ --duration: 0ms; \}`).Match(got) {
		t.Error("tokens.css lost the reduced-motion block")
	}
}

// Nothing but the designer's file may carry colours or theme switches, and
// the app never follows a light OS theme.
func TestOnlyTokensCSSHasRawColoursAndNothingUsesPrefersColorScheme(t *testing.T) {
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	err := fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".css") && !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		b, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "prefers-color-scheme") {
			t.Errorf("%s uses prefers-color-scheme", path)
		}
		if path != "static/css/tokens.css" && strings.HasSuffix(path, ".css") && hex.Match(b) {
			t.Errorf("%s contains raw hex colours: %q", path, hex.FindAll(b, -1))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// components.css uses tokens for font sizes and spacing, never raw values.
// (1px/2px hairlines are borders, not spacing, and are allowed.)
func TestComponentsCSSUsesTokens(t *testing.T) {
	b, err := fs.ReadFile(assets, "static/css/components.css")
	if err != nil {
		t.Fatal(err)
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), "")
	decl := regexp.MustCompile(`(?m)^\s*(font-size|padding[a-z-]*|margin[a-z-]*|gap|row-gap|column-gap|inset[a-z-]*|top|left|right|bottom|width|height|min-height|max-width|border-radius)\s*:\s*([^;]+);`)
	rawPx := regexp.MustCompile(`\b([3-9]|[1-9][0-9]+)(\.[0-9]+)?(px|rem|em)\b`)
	for _, m := range decl.FindAllStringSubmatch(css, -1) {
		if rawPx.MatchString(m[2]) {
			t.Errorf("components.css: %s: %s uses a raw size; use a token", m[1], m[2])
		}
	}
}

// Text colour pairs used by the components meet WCAG 2.2 AA (4.5:1 for body
// text; 3:1 for control borders against the page).
func TestTokenContrast(t *testing.T) {
	b, err := fs.ReadFile(assets, "static/css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, m := range regexp.MustCompile(`(--[a-z0-9-]+):\s*(#[0-9a-fA-F]{6})`).FindAllStringSubmatch(string(b), -1) {
		tokens[m[1]] = m[2]
	}
	pairs := []struct {
		fg, bg string
		min    float64
	}{
		{"--text", "--bg", 4.5},
		{"--text-2", "--bg", 4.5},
		{"--text-3", "--bg", 4.5},
		{"--text-3", "--bg-sunken", 4.5}, // column header
		{"--text-3", "--bg-raised", 4.5},
		{"--text", "--bg-hover", 4.5},   // button hover
		{"--text-3", "--bg-hover", 4.5}, // poster placeholder initial
		{"--on-accent", "--accent", 4.5},
		{"--on-accent", "--accent-hover", 4.5},
		{"--control-border", "--bg", 3},
		{"--accent", "--bg", 3}, // focus ring, selected tab underline
	}
	for _, p := range pairs {
		fg, bg := tokens[p.fg], tokens[p.bg]
		if fg == "" || bg == "" {
			t.Fatalf("missing token %s or %s", p.fg, p.bg)
		}
		if r := contrast(fg, bg); r < p.min {
			t.Errorf("%s on %s = %.2f:1, want ≥ %.1f:1", p.fg, p.bg, r, p.min)
		}
	}
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(0) + 0.7152*ch(1) + 0.0722*ch(2)
}

// Nothing in the stylesheet can single out large sizes or unwatched items
// (DESIGN.md §3): the size and completion blocks never take the accent or a
// user hue, and there are no modifiers keyed on either.
func TestSizeAndCompletionCSSIsNeutral(t *testing.T) {
	b, err := fs.ReadFile(assets, "static/css/components.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?s)([^{}]+)\{([^}]*)\}`)
	for _, m := range rule.FindAllStringSubmatch(string(b), -1) {
		sel, body := m[1], m[2]
		if !regexp.MustCompile(`\.(size|completion)\b`).MatchString(sel) {
			continue
		}
		if strings.Contains(body, "--accent") || strings.Contains(body, "--user-") {
			t.Errorf("%s uses the accent or a user hue", strings.TrimSpace(sel))
		}
		if regexp.MustCompile(`--(large|big|huge|warn|unwatched|none|empty)`).MatchString(sel) {
			t.Errorf("judgemental modifier %s", strings.TrimSpace(sel))
		}
	}
}
