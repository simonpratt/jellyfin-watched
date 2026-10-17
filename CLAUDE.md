# Jellyfin-watched

A self-hosted, read-only dashboard showing which Jellyfin users have completed each movie and TV season. A Go server renders `html/template` pages with htmx, in one Docker image on the LAN-only homelab. `REQUIREMENTS.md` is the spec.

**Read `CONTEXT.md` before any work.** It is the domain glossary. Use its terms in code, API, UI copy, tests and tickets, and never the words listed under _Avoid_ (a **User** is a Jellyfin user whose watch state is shown; the person using the dashboard is the **Administrator**). If a term is missing or no longer fits, raise it with the user rather than inventing one.

Before building significant new behaviour or a new feature, run the `/domain-modeling` skill: it sharpens the terms the work introduces in `CONTEXT.md` and records any hard-to-reverse decision as an ADR.

## Sources, and which one wins

- `REQUIREMENTS.md`: functional and technical requirements, with the exact completion and size rules in §3 Definitions.
- Linear tickets: what to build. A ticket's acceptance criteria are its definition of done.
- `DESIGN.md`: the design brief. The app is dark only, so skip the brief's light-theme requirements.
- `designs/`: the designer's mockups (`*.png`, dark, @2x) and `tokens.css`. The mockups call the app "Spool"; its name is now "Jellyfin-watched". Read the PNG for any screen or component you touch before writing markup; `Components@2x.png` and `States@2x.png` cover the variants and states.

When they disagree, `CONTEXT.md` wins on terms, the ticket wins on behaviour, `designs/` wins on layout, colour and copy, and `REQUIREMENTS.md` wins everything else. Mention each disagreement you resolve in the ticket or the commit message.

## Tickets

Tickets are in Linear, project `jellyfin-watched` (team SIM). `/linear-refine` and `/linear-implement` carry the status flow and commit format. For any other change, commit to `master` with the subject `SIM-<n>: <summary>` when it belongs to a ticket.

Done means every acceptance criterion is met. Automated criteria get their test in the same commit. Manual ones (on a phone, in devtools) are checked, or reported as unchecked with the steps to check them.

Jellyfin API behaviour confirmed against the real server (version 12.1.0) is recorded in code comments next to each call in `internal/jellyfin/client.go`. Read them before relying on a field or flag (completion is worked out per episode, because the season-level `Played` flag is wrong on this server), and record any behaviour you newly confirm there in the same change.

## Commands

`go run .` with the variables from `.env.example` set (serves on `LISTEN_ADDR`, default `:8080`), and `go test -race ./...` (everything, including app-level tests against a fake Jellyfin). `.env` points at the real Jellyfin server.

- **`go test -race`** needs a C compiler (gcc).
- **Gallery:** `internal/web/gallery_test.go` renders every component with edge-case fixtures to `tmp/gallery.html`. Add fixtures for any new component or variant.
- **Viewing the app** in this sandbox: start `go run .` with `.env` loaded as a background task, with `LISTEN_ADDR=0.0.0.0:8080`, and wait until `/movies` returns 200. Then ask the user to run `sbx ports <sandbox-name> --publish 8080:8080` on their host, with the name from `$SANDBOX_NAME`, and to log in with `DASHBOARD_PASSWORD` if it is set. Restart after a Go or template change. Stop the task when asked.

## Invariants

- **Read-only:** the Jellyfin client sends GET only, through its single `Get` method. A test fails if the client gains other methods. The app's own routes may use POST (e.g. `POST /refresh`).
- **Secrets stay on the server:** the API key and dashboard password appear only in server memory, never in HTML, htmx fragments, image responses, headers or logs. Tests fail if an error or log line contains them.
- **Neutral UI:** the app shows completion only. Nothing recommends, ranks, flags or highlights items for deletion (`DESIGN.md` §3, `REQUIREMENTS.md` § Out of scope).
- **Password gate:** every new route goes behind it; only `/login` and `/static/*` are public. A test checks every route.
- **Standard library first:** add a dependency only when the standard library is clearly inadequate, and say why.
- **Fake Jellyfin:** tests use one built with `net/http/httptest`, returning JSON shaped like the real API.

## UI

Follow the `homelab-frontend` skill (htmx branch). In this repo the templates are `html/template`, not templ: each component is one `{{define "name"}}` block in `internal/web/templates/components/`, one file each, taking one view-model struct from `internal/web/views.go`, called as `{{template "user-tag" .User}}`. `internal/web/static/css/components.css` has one commented section per component, in the same order, and the gallery test stands in for the catalogue page.

- **Class names** are prefixed with the component name: `.user-tag`, `.user-tag__code`, `.user-tag--progress`.
- **Untokened sizes:** a size the design gives no token for is a `calc()` of space tokens (e.g. `calc(6 * var(--space-12))`), with a comment saying why. A test fails on raw hex outside `tokens.css`.
- **View state** is `q`, `sort`, `dir`, and on TV `seasons` plus the series shown (`show`) or, under `seasons=all`, hidden (`hide`). Build URLs with `viewURL`, so they survive the page's self-reload after a refresh (`#app` in `layout.tmpl`).
- **Tokens:** `internal/web/static/css/tokens.css` is `designs/tokens.css` with `color-scheme: dark` set and the `@media (prefers-color-scheme: light)` block deleted. When the designer updates `designs/tokens.css`, re-copy it with the same two changes. A test fails if the copy drifts or anything else uses `prefers-color-scheme`.
- **User identity:** each user gets hue `--user-1`…`--user-10`, in Jellyfin's `/Users` order, wrapping after 10, and a two-letter mono code: the first two letters of a one-word name, or initials for a multi-word one. The code tells users apart; the hue only speeds scanning. The server has 6 users with distinct codes, so code collisions need no handling.
- **Accent** is for interaction only (focus, the selected tab or sort, the primary button, the loading hairline). Data stays neutral: nothing is green, red or amber.
- **Numbers** are all in the mono font, including sizes, years, counts and "x / y".
- **Depth** comes from the surface tokens and hairlines; the app has no shadows.
- **Reduced motion** sets `--duration: 0ms`. The loading hairline and the refresh arc stay still.

## Done means

All of these pass:

```bash
gofmt -l .           # prints nothing
go vet ./...
go test -race ./...
go build ./...
```

Then prove the behaviour in the running app, not just in tests: `curl` the changed pages and fragments, check they show the new behaviour and don't contain the API key, and report what you checked. For UI changes, screenshot the gallery and the changed pages with headless Chrome at desktop width and at 380 px, with no horizontal scroll, and compare them against the matching `designs/*.png`. For container changes, `docker build` and run the image with only environment variables set.
