package main

import (
	"os"
	"strings"
	"testing"
)

// configVars is every environment variable in REQUIREMENTS.md §8.
var configVars = []string{
	"JELLYFIN_URL",
	"JELLYFIN_API_KEY",
	"LISTEN_ADDR",
	"REFRESH_MINUTES",
	"INCLUDE_SPECIALS",
	"DASHBOARD_PASSWORD",
	"JELLYFIN_TIMEOUT_SECONDS",
}

// TestDeployExamplesListEveryVariable keeps the example docker-compose.yml
// and .env.example in step with the configuration the app reads.
func TestDeployExamplesListEveryVariable(t *testing.T) {
	for _, file := range []string{"docker-compose.yml", ".env.example"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range configVars {
			if !strings.Contains(string(b), v) {
				t.Errorf("%s does not mention %s", file, v)
			}
		}
	}
}

// TestDockerignoreExcludesSecrets keeps .env (which holds the real API key)
// out of the Docker build context, so it can never end up in an image layer.
func TestDockerignoreExcludesSecrets(t *testing.T) {
	b, err := os.ReadFile(".dockerignore")
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	for _, want := range []string{".env", "tmp"} {
		if !lines[want] {
			t.Errorf(".dockerignore does not exclude %q", want)
		}
	}
}
