// Package config reads the dashboard's settings from environment variables
// (REQUIREMENTS.md §8). There is no config file and no required flag.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Secret holds a value that must never be printed or logged. Every
// formatting path (fmt verbs, slog) renders it as a placeholder; call Reveal
// to get the underlying value where it is genuinely needed.
type Secret struct{ value string }

// Reveal returns the secret value.
func (s Secret) Reveal() string { return s.value }

// IsSet reports whether the secret is non-empty.
func (s Secret) IsSet() bool { return s.value != "" }

func (s Secret) String() string {
	if s.value == "" {
		return ""
	}
	return "[redacted]"
}

// GoString keeps %#v from printing the value.
func (s Secret) GoString() string { return s.String() }

// Format keeps every fmt verb, including %+v on a containing struct, from
// printing the value.
func (s Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(s.String())) }

// LogValue keeps slog from printing the value.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// Config is the dashboard's configuration.
type Config struct {
	JellyfinURL       string // no trailing slash
	APIKey            Secret
	ListenAddr        string
	RefreshInterval   time.Duration
	IncludeSpecials   bool
	DashboardPassword Secret
	JellyfinTimeout   time.Duration
}

// Load reads the configuration using getenv (normally os.Getenv). It returns
// an error naming every missing required variable and every invalid value.
func Load(getenv func(string) string) (Config, error) {
	get := func(name string) string { return strings.TrimSpace(getenv(name)) }

	var missing []string
	var errs []error

	cfg := Config{
		JellyfinURL:       strings.TrimRight(get("JELLYFIN_URL"), "/"),
		APIKey:            Secret{get("JELLYFIN_API_KEY")},
		ListenAddr:        get("LISTEN_ADDR"),
		DashboardPassword: Secret{getenv("DASHBOARD_PASSWORD")},
	}

	if cfg.JellyfinURL == "" {
		missing = append(missing, "JELLYFIN_URL")
	} else if u, err := url.Parse(cfg.JellyfinURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("JELLYFIN_URL must be an http:// or https:// URL, e.g. http://jellyfin:8096"))
	}
	if !cfg.APIKey.IsSet() {
		missing = append(missing, "JELLYFIN_API_KEY")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	refresh, err := positiveInt(get("REFRESH_MINUTES"), 30)
	if err != nil {
		errs = append(errs, fmt.Errorf("REFRESH_MINUTES %w", err))
	}
	cfg.RefreshInterval = time.Duration(refresh) * time.Minute

	timeout, err := positiveInt(get("JELLYFIN_TIMEOUT_SECONDS"), 10)
	if err != nil {
		errs = append(errs, fmt.Errorf("JELLYFIN_TIMEOUT_SECONDS %w", err))
	}
	cfg.JellyfinTimeout = time.Duration(timeout) * time.Second

	if v := get("INCLUDE_SPECIALS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("INCLUDE_SPECIALS must be true or false"))
		}
		cfg.IncludeSpecials = b
	}

	if len(missing) > 0 {
		errs = append([]error{fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))}, errs...)
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func positiveInt(v string, def int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("must be a whole number greater than 0")
	}
	return n, nil
}
