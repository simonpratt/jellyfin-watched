# Jellyfin-watched

A small, self-hosted, **read-only** dashboard for a Jellyfin server. It lists every movie and TV season with its cover art and size on disk, and shows which users have finished it or are partway through.

## Deploy

Jellyfin-watched is a single container, configured entirely through environment variables. It needs a Jellyfin admin API key (Jellyfin Dashboard → API Keys).

```bash
cp .env.example .env      # set JELLYFIN_URL and JELLYFIN_API_KEY
docker compose up -d      # serves on http://<host>:8080
```

`docker-compose.yml` pulls `ghcr.io/owner/jellyfin-watched:latest`. Replace `owner` with the GitHub account that publishes the image. To build it yourself instead, use `build: .`.

CI publishes multi-arch images (amd64 and arm64) on pushes to `main` and on `v*` tags.

### Configuration

| Variable | Default | |
|---|---|---|
| `JELLYFIN_URL` | required | Base URL of your Jellyfin server, with no trailing slash |
| `JELLYFIN_API_KEY` | required | Admin API key. Stays on the server and is never sent to the browser |
| `DASHBOARD_PASSWORD` | empty | Set it to put the dashboard behind a password |
| `REFRESH_MINUTES` | `30` | Minutes between data refreshes. "Refresh now" is always available |
| `INCLUDE_SPECIALS` | `false` | Show Season 0 (Specials) |
| `JELLYFIN_TIMEOUT_SECONDS` | `10` | Timeout for each request to Jellyfin |
| `LISTEN_ADDR` | `:8080` | Address and port inside the container |
| `TZ` | UTC | Timezone for the "last updated" time |

### Notes

- **Data freshness:** data is cached in memory and refreshed on a schedule. If Jellyfin can't be reached, the last good data stays on screen with a notice.
- **Sessions:** the login session lasts 30 days, but restarting the container signs everyone out.
- **Exposure:** it's built for a home network. Put it behind your reverse proxy for TLS before exposing it anywhere else.

## Development

Run `go run .` with the variables above set, and `go test ./...` for the tests. The design and requirements are in `DESIGN.md` and `REQUIREMENTS.md`.
