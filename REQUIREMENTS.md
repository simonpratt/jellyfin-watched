# Jellyfin Watch-Completion Dashboard: Requirements

## 1. Purpose

A small, self-hosted, **read-only** web dashboard that lists every movie and every TV season on a Jellyfin server, with cover art and size on disk, and shows which users have completed each one.

It exists so the administrator can glance through the library and decide what to delete. The dashboard only displays information. It does not recommend, rank, flag, or delete anything.

## 2. Scope

### In scope

- A movies list and a TV shows list, styled like Radarr and Sonarr: cover art alongside the details.
- For each movie: size on disk, and which users have completed it.
- For each season: size on disk, which users have completed it, and which users are partway through it.

### Out of scope

- Per-user views or per-user statistics.
- "Watched by everyone" filters.
- Suggestions, highlighting of "safe to delete" items, or any other judgement.
- Any write action on Jellyfin, including deleting media or changing watched state.
- Music, audiobooks, photos, live TV, and home videos.

## 3. Definitions

**Completed movie.** Jellyfin reports `UserData.Played = true` for that user and movie. Movies marked watched manually count.

**Completed season.** Every episode in the season that exists in the library is `Played = true` for that user.

- Missing or placeholder episodes (`isMissing = true`) are not counted.
- Completion is worked out from the episodes, not from the season's own `Played` flag, because that flag has been reported as unreliable.
- Episodes marked watched manually count.
- Season 0 (Specials) is hidden by default and can be shown with a setting.

**In progress (seasons only).** The user has played at least one episode but not all of them. Shown as played / available, e.g. "4 / 10".

**Size.**

- Movie: the total size of its media files. If a movie has several versions (e.g. 1080p and 4K), their sizes are added together.
- Season: the sum of the sizes of its available episodes.
- Series: the sum of its seasons, including Specials even when Specials are hidden.
- Displayed in human-readable units (e.g. "4.2 GB"), using binary units (1 GB = 1024³ bytes) consistently.
- If Jellyfin does not report a size for an item, show "Unknown" rather than 0, and exclude it from sums with a note (e.g. "38.1 GB + 2 unknown").

## 4. Screens

### 4.1 Movies

A list of every movie. Each row shows:

- Cover art (poster).
- Title and year.
- Size on disk.
- The users who have completed it, as names or small tags. If nobody has, the row says so plainly (e.g. "Not completed by anyone").

### 4.2 TV shows

A list of every series. Each series row shows:

- Cover art (series poster).
- Title and year.
- Total size on disk.
- Its seasons, each showing:
  - Season name (e.g. "Season 2").
  - Size on disk.
  - Users who have completed the season.
  - Users who are in progress, with their "x / y".

Seasons can be shown inline under the series or in an expandable section. Either is acceptable, as long as all seasons of a series can be seen without leaving the list.

### 4.3 Controls

- **Must:** switch between the Movies and TV shows lists.
- **Must:** search by title.
- **Should:** sort by title, year, date added to the library, or size.
- **Should:** a "refresh now" button and a visible "last refreshed" time.
- **Could:** choose between a list layout and a poster-grid layout.

No filter or sort may be framed as a recommendation, e.g. no "deletion candidates" view.

## 5. Data source

All data comes from the Jellyfin HTTP API using an administrator API key (Dashboard → API Keys), sent as:

```
Authorization: MediaBrowser Token=<API_KEY>
```

| Purpose | Request |
|---|---|
| List users | `GET /Users` |
| All movies | `GET /Items?includeItemTypes=Movie&recursive=true` |
| Movies a user has played | `GET /Items?userId={uid}&includeItemTypes=Movie&recursive=true&isPlayed=true` |
| All series | `GET /Items?includeItemTypes=Series&recursive=true` |
| All seasons | `GET /Items?includeItemTypes=Season&recursive=true` |
| Episodes with a user's watch state | `GET /Items?userId={uid}&includeItemTypes=Episode&recursive=true&isMissing=false&enableUserData=true` |
| Cover art | `GET /Items/{itemId}/Images/Primary` *(to confirm during implementation)* |
| File sizes | Request movies and episodes with `fields=MediaSources` and read `MediaSources[].Size` *(to confirm during implementation)* |

Notes:

- Fetch each user's episodes in one paginated request (`startIndex`, `limit`) and group them by `SeasonId` in the app, rather than making one request per season.
- Episode sizes only need to be fetched once per refresh, not once per user.
- If a season has no poster of its own, use the series poster.
- Use `isPlayed=true` or `isPlayed=false`, never both `IsPlayed` and `IsUnplayed` in `filters`. Jellyfin applies only one of them.
- Confirm endpoints and fields against the Jellyfin version in use (target: 10.10 and 10.11).

## 6. Technical stack

- **Language:** Go, using the standard library where practical (`net/http`, `html/template`, `embed`).
- **Templates:** Go `html/template`. Pages are rendered on the server.
- **Interactivity:** htmx. Search, sorting, expanding a series' seasons, and "refresh now" swap in server-rendered HTML fragments. No JavaScript framework and no front-end build step.
- **Assets:** templates, CSS, and the htmx script are embedded in the binary with `embed`. Nothing is loaded from a CDN at runtime.
- **Output:** a single static binary.

## 7. Build and release

- A `Dockerfile` using a multi-stage build: compile in a Go image, then copy the binary into a minimal runtime image (e.g. `distroless/static` or `scratch`, with CA certificates if Jellyfin is reached over HTTPS).
- The container runs as a non-root user.
- A GitHub Actions workflow that:
  - Runs `go vet` and `go test` on every push and pull request.
  - Builds and pushes the image to GitHub Container Registry (`ghcr.io`) on pushes to the main branch, tagged `latest` and with the commit SHA.
  - On a version tag (e.g. `v1.2.0`), also pushes the image with that version tag.
  - Builds for `linux/amd64` and `linux/arm64`.
- An example `docker-compose.yml` in the repository.

## 8. Configuration

All configuration is through environment variables. There is no config file and no required command-line flag.

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `JELLYFIN_URL` | yes | | e.g. `http://jellyfin:8096` |
| `JELLYFIN_API_KEY` | yes | | Admin API key |
| `LISTEN_ADDR` | no | `:8080` | Address and port the dashboard listens on |
| `REFRESH_MINUTES` | no | `30` | Minimum time between data refreshes |
| `INCLUDE_SPECIALS` | no | `false` | Show Season 0 |
| `DASHBOARD_PASSWORD` | no | | Require a password if set |
| `JELLYFIN_TIMEOUT_SECONDS` | no | `10` | Timeout for each request to Jellyfin |

On startup the app checks that the required variables are set and exits with a clear error message if not.

## 9. Non-functional requirements

### 9.1 Read-only

- The app sends only `GET` requests to Jellyfin. There are no `POST`, `PUT`, or `DELETE` requests anywhere in the code.
- The Jellyfin client exposes only a `GET` method, so no other method can be called by mistake.
- A Jellyfin API key has full admin rights, so this guarantee comes from the app's code, not from the key.

### 9.2 Security

- The API key stays on the server. It is never sent to the browser or written to logs.
- The browser talks only to the dashboard. Cover art is fetched by the dashboard and passed through to the browser, so the browser never needs the API key.
- Intended for a trusted local network. An optional password (`DASHBOARD_PASSWORD`) can be set.

### 9.3 Performance

- Data is cached in memory and refreshed at most every `REFRESH_MINUTES`. Pages read from the cache.
- Cover art is cached, and images load lazily as the user scrolls.
- If Jellyfin is unreachable, the dashboard shows the last good data with a visible warning.
- No database.

## 10. Acceptance criteria

1. Every movie and every series in Jellyfin appears in the lists, with cover art and size.
2. A movie shows exactly the users for whom Jellyfin reports it as played.
3. A user who has watched every available episode of a season appears as completed for that season.
4. A user with one unwatched available episode in a season appears as in progress with the correct "x / y".
5. Missing episodes do not stop a season from counting as completed.
6. Season sizes equal the sum of their episode sizes, and series sizes equal the sum of their seasons.
7. Only `GET` requests are sent to Jellyfin.
8. The API key does not appear in the browser, the page source, or the logs.
9. Nothing in the interface suggests what to delete.
10. Pushing to the main branch produces a working image on `ghcr.io` that starts with only environment variables set.