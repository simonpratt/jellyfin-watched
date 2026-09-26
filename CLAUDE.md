# Jellyfin Watch-Completion Dashboard

A read-only Go + htmx dashboard showing which Jellyfin users have completed each movie and TV season.

## Sources of truth

- `REQUIREMENTS.md`: functional and technical requirements. Wins any conflict.
- `DESIGN.md`: design brief. The designer's deliverables (tokens, components, states) are implemented against it. **Exception: the app is dark mode only.** Ignore the brief's light-theme requirements.
- `.scratch/watch-dashboard/issues/NN-*.md`: the tickets. Each has "Blocked by" and acceptance criteria.
- `designs/tokens.css`: the designer's tokens. This file is the source of truth for every colour, size and space value.
- `designs/*.png`: the designer's mockups (dark theme, @2x). The app is named **Spool**. Read the PNG for any screen or component you touch before writing markup.
  - `Tokens, type and favicon@2x.png`: a visual reference for `tokens.css`, the user hues, the type scale, spacing, radii and the favicon.
  - `Components@2x.png`: every component and its variants.
  - `States@2x.png`: first load, refreshing, stale data, no results, empty library, image loading, wrong password.
  - `Movies · dark@2x.png`, `TV shows · dark@2x.png`, `Password · dark@2x.png`: full screens.

## Working a ticket

You are given one ticket. Work only on that ticket.

1. Read the ticket, then the sections of `REQUIREMENTS.md` and `DESIGN.md` it touches.
2. Check that every ticket in its "Blocked by" has `**Status:** done`. If one doesn't, stop and report back.
3. Read the existing code before adding to it. Extend what's there rather than building something parallel.
4. Work test-first, one acceptance criterion at a time.
5. Don't implement other tickets' criteria, even small ones. If you spot a gap, note it in your final report.
6. When everything passes validation (below), tick the ticket's checkboxes, set `**Status:** done`, and commit with a message that starts `NN: <ticket title>`.
7. Report back: what was built, anything deviating from the ticket, anything left for a later ticket, and any Jellyfin API facts you confirmed or corrected.

## Hard rules

- **Read-only.** The Jellyfin client exposes only a GET method. No POST/PUT/DELETE to Jellyfin, anywhere.
- **The API key never leaves the server:** not in HTML, htmx fragments, image responses, headers or logs.
- **Neutral UI.** Nothing may recommend, rank, flag or highlight items for deletion (`DESIGN.md` §3).
- **No runtime external resources.** Everything is embedded with `embed`; no CDNs.
- **Standard library first.** Add a dependency only if the standard library is clearly inadequate, and say why in the report.
- **No real Jellyfin in tests.** Tests use a fake Jellyfin built with `net/http/httptest`, returning JSON shaped like the real API.

## UI components

### Where they live

Everything the browser receives lives in one package, `internal/web`, and is embedded with `embed`:

```
internal/web/
  templates/
    layout.tmpl           page shell: header, Movies / TV shows tabs, refresh control, loading hairline
    pages/*.tmpl          one per screen: movies, tv, password
    components/*.tmpl     one file per component, one or more {{define}} blocks each
  static/
    css/tokens.css        copy of designs/tokens.css, dark only (see below)
    css/components.css    one commented section per component, in the same order as components/
    fonts/*.woff2         Instrument Sans, IBM Plex Mono (SIL OFL 1.1; licence file alongside)
    favicon.svg           the Spool reel, light stroke for dark browser tabs
    htmx.min.js
  views.go                view-model structs that components take
  format.go               size, count, date and "N minutes ago" formatting
```

Keep the other packages free of HTML and CSS (the Jellyfin client, the cache/snapshot and the domain logic). They hand plain Go data to `internal/web`.

### How to use them

- **One component = one `{{define "name"}}` block that takes one view-model struct.** Call it with `{{template "user-tag" .User}}`. Don't pass a bare map or a whole domain object.
- **Formatting happens in Go, not in templates.** Handlers and view-model constructors in `views.go` do it, using `format.go`. Templates only print fields and use `if`/`range`. For example, a size view model already holds `Value "14.6"`, `Unit "GB"`, `Unknown bool` and `UnknownCount int`.
- **Fragments reuse components.** Every htmx response (search results, sort, refresh, expanding a series) renders the same `define` blocks as the full page. Never duplicate markup for a fragment.
- **Before building, reuse or extend.** If a component already exists, add a variant field to its view model instead of making a near-copy.
- **CSS uses tokens only.** No raw hex, px font sizes or ad-hoc spacing in `components.css`; everything is `var(--…)`. If the design needs a value that isn't a token, stop and flag it; don't invent one.
- **Class names are prefixed with the component name**, e.g. `.user-tag`, `.user-tag__code`, `.user-tag--progress`. There's no utility-class framework.

### Component catalogue

| Component (`define` name) | Variants from `Components@2x.png` | Built in ticket |
|---|---|---|
| `poster` | image; generated placeholder (initial on a `--bg-hover` block); missing (dashed `--line-strong` box + icon). Fixed 48×72, lazy, alt = title | 02 (placeholder), 05 (images) |
| `size` | value + small unit, mono, right-aligned, one decimal; "Unknown"; sum with "+ N unknown" on a second line | 04 |
| `user-tag` | name tag; in-progress tag with "x / y" and a thin bar | 04, 06 |
| `user-group` | 5 or more users collapse to a stack of codes + "All 10 users" / "6 users", which expands to the full tags | 04 |
| `completion` | completed tags, then each in-progress user on its own line; otherwise "Not completed by anyone" | 04, 06 |
| `button` | default, hover, focus, primary (password screen only), disclosure ("Show 35 seasons" / "Hide seasons"), "Expand all" | 02, 06 |
| `search` | empty, focused with typed text and a clear (×) button; 200 ms debounce | 08 |
| `sort-control` | segmented Title / Year / Date added / Size, plus a direction button | 08 |
| `refresh-control` | "Updated N ago" + "Refresh now" / "Refreshing…" (spinning arc, still under reduced motion) | 07 |
| `notice` | the one notice style: raised surface, strong hairline, info mark, headline then detail | 07 |
| `empty-state` | no search results (with "Clear search"), empty library, first load | 07, 08 |
| `movie-row` | poster, title, year + "Added …", size, completion | 02, then 04, 05 |
| `series-row` / `season-row` | series totals with a disclosure; seasons indented on a rail with "N episodes" | 06 |

### Design rules to enforce

- **User identity:** each user gets hue `--user-1`…`--user-10`, following Jellyfin's `/Users` order, plus a two-letter code in mono (the first two letters for a one-word name, initials for a multi-word one, e.g. "SA" for Sam and "AK" for Anastasia Kowalczyk). The code is what tells users apart; the hue only speeds scanning. The server has 3 users with different first letters, so don't build handling for code collisions. Past 10 users, hues simply wrap (user 11 gets `--user-1`).
- **The accent is for interaction only** (focus, the selected tab or sort, the primary button, the loading hairline). Data never takes the accent, and nothing is ever green, red or amber.
- **Every number is in the mono font**, including sizes, years, counts and "x / y".
- **No shadows.** Depth comes from the surface tokens and hairlines.
- **Dark mode only.** Copy `designs/tokens.css` into `internal/web/static/css/tokens.css` with two changes: set `color-scheme: dark`, and delete the `@media (prefers-color-scheme: light)` block. Keep the reduced-motion block. Nothing else may use `prefers-color-scheme`. Don't edit `designs/tokens.css`; if the designer updates it, re-copy it with the same two changes.
- **Reduced motion** sets `--duration: 0ms`. The loading hairline and refresh arc stay still.

## Validation

Go tools may need a login shell here: `bash -l -c "..."`.

Before marking a ticket done, all of these must pass:

```bash
gofmt -l .           # must print nothing
go vet ./...
go test -race ./...
go build ./...
```

Then prove the ticket's behaviour in the running app, not just in tests:

- Run the binary with the environment variables from `.env.example`. Point `JELLYFIN_URL` at a fake server, or at a real one if `.env` has real values.
- `curl` the affected pages and fragments. Check the output shows the new behaviour and doesn't contain the API key.
- For UI tickets:
  - Keep a component gallery up to date. A test in `internal/web` renders every component with edge-case fixtures (a 20-character name, all 10 users, "1 / 200", an unknown size, a missing poster, the Dr. Strangelove title) and writes `tmp/gallery.html` (git-ignored). The test fails if any template errors.
  - Screenshot the gallery and the changed pages with headless Chrome, at desktop width and at 380 px. Compare them side by side with the matching `designs/*.png`, and state any visible differences in your report.
  - `grep` `components.css` for raw hex values (`#[0-9a-fA-F]{3,6}`); only `tokens.css` may contain them.
- For the container ticket, `docker build` and run the image with only environment variables set.

If you had to assume something about the Jellyfin API (marked "to confirm" in `REQUIREMENTS.md` §5), record the confirmed behaviour in your report and in a code comment next to the call.
