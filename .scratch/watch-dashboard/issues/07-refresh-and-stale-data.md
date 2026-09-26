# 07: Scheduled refresh, "Refresh now" and stale data

**What to build:** Library data stays current without blocking the page. The cache refreshes in the background at most every `REFRESH_MINUTES`. A "Refresh now" button starts a refresh while the list stays usable, and the header shows when the data was last updated. First start and Jellyfin outages get the calm notices from the design.

**Blocked by:** 02 (Design foundation and page shell)

**Status:** ready-for-agent

- [ ] A background refresh runs at most every `REFRESH_MINUTES`; each Jellyfin request uses `JELLYFIN_TIMEOUT_SECONDS`
- [ ] A refresh builds a new snapshot and swaps it in whole; pages never see half-refreshed data
- [ ] "Refresh now" (htmx) shows "Refreshing…" and then updates; concurrent clicks don't start concurrent refreshes
- [ ] "Updated N minutes ago" is shown and stays accurate
- [ ] Before the first load finishes, pages show "Loading your library. This can take a minute on first start." and update once data arrives
- [ ] If Jellyfin can't be reached, the last good data stays on screen with "Can't reach Jellyfin. Showing data from <age> ago."
- [ ] Refresh failures are logged without the API key
- [ ] Tests cover a successful refresh, a failed refresh with data kept, and the first-load state
