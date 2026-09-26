# 01: Walking skeleton: movie titles from Jellyfin

**What to build:** Running the binary with `JELLYFIN_URL` and `JELLYFIN_API_KEY` set serves a Movies page listing every movie's title and year, fetched from Jellyfin and held in an in-memory cache. This slice sets up the Go module, env-var configuration, a GET-only Jellyfin client, `html/template` rendering, and embedded assets (templates, CSS, htmx). Unstyled output is fine here. The design system arrives in 02.

**Blocked by:** None (can start immediately)

**Status:** done

- [x] All settings from REQUIREMENTS.md §8 are read from environment variables with the documented defaults; there is no config file and no required flag
- [x] Startup exits with a clear error naming any missing required variable
- [x] The Jellyfin client exposes only a GET method and sends `Authorization: MediaBrowser Token=<key>`
- [x] The movie list is fetched with pagination and stored in an in-memory cache; the page reads from the cache, never directly from Jellyfin
- [x] htmx, templates and CSS are embedded in the binary; the page loads nothing from a CDN
- [x] Tests against a fake Jellyfin (`httptest`) prove every outbound request is a GET
- [x] Tests prove the API key never appears in rendered HTML or log output
- [x] `go vet` and `go test` pass
