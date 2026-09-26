# Jellyfin Watch-Completion Dashboard

A read-only Go + htmx dashboard showing which Jellyfin users have completed each movie and TV season.

## Sources of truth

- `REQUIREMENTS.md`: functional and technical requirements. Wins any conflict.
- `DESIGN.md`: design brief. The designer's deliverables (tokens, components, states) are implemented against it.
- `.scratch/watch-dashboard/issues/NN-*.md`: the tickets. Each has "Blocked by" and acceptance criteria.

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
- For UI tickets, check both light and dark themes and a 380 px viewport where possible.
- For the container ticket, `docker build` and run the image with only environment variables set.

If you had to assume something about the Jellyfin API (marked "to confirm" in `REQUIREMENTS.md` §5), record the confirmed behaviour in your report and in a code comment next to the call.
