# Spool: Jellyfin watch-completion dashboard

A read-only Go + htmx dashboard showing which Jellyfin users have completed each movie and TV season.

## Sources of truth

- `REQUIREMENTS.md`: functional and technical requirements. Wins any conflict.
- `DESIGN.md`: the design brief. **The app is dark mode only**, so skip the brief's light-theme requirements.
- `designs/tokens.css`: every colour, size and space value.
- `designs/*.png`: the designer's mockups (dark, @2x). Read the PNG for any screen or component you touch before writing markup. `Components@2x.png` and `States@2x.png` cover the variants and states.
- `.scratch/watch-dashboard/issues/`: tickets, each with "Blocked by", `**Status:**` (`ready-for-agent`, `done` or `wont-do`) and acceptance criteria. A ticket marked `wont-do` stays unbuilt.
- Jellyfin API behaviour confirmed against the real server (version 12.1.0) is recorded in code comments next to each call in `internal/jellyfin/client.go`. Read them before relying on a field or flag. For example, completion is worked out per episode because the season-level `Played` flag is wrong on this server.

## Hard rules

- **Read-only.** The Jellyfin client sends GET only, through its single `Get` method; a test fails if the client gains other methods. The app's own routes may use POST (e.g. `POST /refresh`).
- **The API key and dashboard password stay on the server:** never in HTML, htmx fragments, image responses, headers or logs.
- **Neutral UI.** Nothing recommends, ranks, flags or highlights items for deletion (`DESIGN.md` §3).
- **Everything is embedded** with `embed`; no CDNs or other runtime external resources.
- **Standard library first.** Add a dependency only when the standard library is clearly inadequate, and say why.
- **Tests use a fake Jellyfin** built with `net/http/httptest`, returning JSON shaped like the real API.
- **The password gate covers every route** except `/login` and `/static/*`. Put new routes behind it.

## Code layout

- `internal/jellyfin`: the GET-only client.
- `internal/library`: the snapshot cache, refresh scheduling, completion logic, and search and sort.
- `internal/artwork`: the in-memory poster cache.
- `internal/web`: everything the browser receives. The other packages hand it plain Go data and hold no HTML or CSS.

## UI components

The components live in `internal/web/templates/components/`, one file each. `internal/web/static/css/components.css` has one commented section per component, in the same order. Before building something, look there and extend an existing component with a variant field rather than making a near-copy.

- **One component = one `{{define "name"}}` block taking one view-model struct** from `views.go`, called as `{{template "user-tag" .User}}`.
- **Formatting happens in Go** (`views.go` using `format.go`). Templates only print fields and use `if`/`range`.
- **Fragments reuse components.** Every htmx response renders the same `define` blocks as the full page.
- **View state lives in the URL**: `q`, `sort`, `dir`, and `seasons` on TV. Build URLs with `viewURL`, so they survive the page's self-reload after a refresh (`#app` in `layout.tmpl`), back and forward, and use without JavaScript.
- **CSS uses tokens only**: `var(--…)`, with no raw hex or px font sizes. A size the design gives no token for is written as a `calc()` of space tokens (e.g. `calc(6 * var(--space-12))`), with a comment saying why.
- **Class names are prefixed with the component name**, e.g. `.user-tag`, `.user-tag__code`, `.user-tag--progress`.

## Design rules

- **User identity:** each user gets hue `--user-1`…`--user-10`, in Jellyfin's `/Users` order, wrapping after 10. Each also gets a two-letter mono code: the first two letters of a one-word name, or initials for a multi-word one. The code tells users apart; the hue only speeds scanning. The server has 6 users with distinct codes, so code collisions need no handling.
- **The accent is for interaction only** (focus, the selected tab or sort, the primary button, the loading hairline). Data never takes the accent, and nothing is green, red or amber.
- **Every number is in the mono font**, including sizes, years, counts and "x / y".
- **Depth comes from the surface tokens and hairlines**, never shadows.
- **Dark mode only.** `internal/web/static/css/tokens.css` is `designs/tokens.css` with `color-scheme: dark` set and the `@media (prefers-color-scheme: light)` block deleted. Nothing else may use `prefers-color-scheme`. When the designer updates `designs/tokens.css`, re-copy it with the same two changes. A test fails if the copy drifts.
- **Reduced motion** sets `--duration: 0ms`. The loading hairline and the refresh arc stay still.

## Validation

Go tools may need a login shell: `bash -l -c "..."`. `go test -race` needs a C compiler (gcc).

Before calling work done, all of these pass:

```bash
gofmt -l .           # must print nothing
go vet ./...
go test -race ./...
go build ./...
```

Then prove the behaviour in the running app, not just in tests:

- Run the binary with the environment variables from `.env.example`. `.env` points at the real Jellyfin server; GET only.
- `curl` the affected pages and fragments. Check they show the new behaviour and don't contain the API key.
- For UI changes:
  - The gallery test in `internal/web/gallery_test.go` renders every component with edge-case fixtures to `tmp/gallery.html`. Add fixtures for any new component or variant.
  - Screenshot the gallery and the changed pages with headless Chrome, at desktop width and at 380 px, with no horizontal scroll. Compare them against the matching `designs/*.png` and report visible differences.
  - `grep` `components.css` for raw hex values (`#[0-9a-fA-F]{3,6}`); only `tokens.css` may contain them.
- For container changes, `docker build` and run the image with only environment variables set.

When you rely on an unconfirmed Jellyfin behaviour, check it against the real server and record it in a code comment next to the call.
