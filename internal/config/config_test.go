package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"JELLYFIN_URL":     "http://jellyfin:8096/",
		"JELLYFIN_API_KEY": "secret-key",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.JellyfinURL != "http://jellyfin:8096" {
		t.Errorf("JellyfinURL = %q, want trailing slash trimmed", cfg.JellyfinURL)
	}
	if cfg.APIKey.Reveal() != "secret-key" {
		t.Errorf("APIKey not stored")
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.RefreshInterval != 30*time.Minute {
		t.Errorf("RefreshInterval = %v, want 30m", cfg.RefreshInterval)
	}
	if cfg.IncludeSpecials {
		t.Errorf("IncludeSpecials = true, want false")
	}
	if cfg.DashboardPassword.Reveal() != "" {
		t.Errorf("DashboardPassword should default to empty")
	}
	if cfg.JellyfinTimeout != 10*time.Second {
		t.Errorf("JellyfinTimeout = %v, want 10s", cfg.JellyfinTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"JELLYFIN_URL":             "https://jf.example",
		"JELLYFIN_API_KEY":         "k",
		"LISTEN_ADDR":              "127.0.0.1:9000",
		"REFRESH_MINUTES":          "5",
		"INCLUDE_SPECIALS":         "true",
		"DASHBOARD_PASSWORD":       "pw",
		"JELLYFIN_TIMEOUT_SECONDS": "3",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" || cfg.RefreshInterval != 5*time.Minute ||
		!cfg.IncludeSpecials || cfg.DashboardPassword.Reveal() != "pw" || cfg.JellyfinTimeout != 3*time.Second {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadMissingRequiredNamesEachVariable(t *testing.T) {
	cases := []struct {
		env     map[string]string
		missing []string
	}{
		{map[string]string{}, []string{"JELLYFIN_URL", "JELLYFIN_API_KEY"}},
		{map[string]string{"JELLYFIN_URL": "http://x"}, []string{"JELLYFIN_API_KEY"}},
		{map[string]string{"JELLYFIN_API_KEY": "k"}, []string{"JELLYFIN_URL"}},
		{map[string]string{"JELLYFIN_URL": "  ", "JELLYFIN_API_KEY": "k"}, []string{"JELLYFIN_URL"}},
	}
	for _, c := range cases {
		_, err := Load(env(c.env))
		if err == nil {
			t.Fatalf("Load(%v): want error", c.env)
		}
		for _, name := range c.missing {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name %s", err, name)
			}
		}
	}
}

func TestLoadInvalidValues(t *testing.T) {
	base := map[string]string{"JELLYFIN_URL": "http://x", "JELLYFIN_API_KEY": "k"}
	bad := map[string]string{
		"JELLYFIN_URL":             "jellyfin:8096",
		"REFRESH_MINUTES":          "soon",
		"INCLUDE_SPECIALS":         "maybe",
		"JELLYFIN_TIMEOUT_SECONDS": "0",
	}
	for name, value := range bad {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		m[name] = value
		_, err := Load(env(m))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%q: want error naming %s, got %v", name, value, name, err)
		}
	}
}

func TestSecretsNeverFormatted(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"JELLYFIN_URL":       "http://x",
		"JELLYFIN_API_KEY":   "super-secret-key",
		"DASHBOARD_PASSWORD": "super-secret-pw",
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		fmt.Sprint(cfg), fmt.Sprintf("%v %+v %#v %s", cfg, cfg, cfg, cfg.APIKey),
	} {
		if strings.Contains(s, "super-secret") {
			t.Errorf("secret leaked in formatted config: %s", s)
		}
	}
}
