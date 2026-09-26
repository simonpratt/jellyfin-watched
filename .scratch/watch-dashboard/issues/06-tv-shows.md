# 06: TV shows list with season completion

**What to build:** The TV shows tab lists every series, with its poster, title, year and total size, and all its seasons, laid out as the design specifies (inline or expandable, with "expand all" if the design has one). Each season shows its size, who completed it, and who is in progress as "x / y". Completion is worked out from episodes, not from the season's own Played flag.

**Blocked by:** 04 (Movie size and who completed it), 05 (Cover art passthrough)

**Status:** ready-for-agent

- [ ] Series and seasons are fetched once per refresh; each user's non-missing episodes (`isMissing=false`, `enableUserData=true`) come from one paginated request and are grouped by `SeasonId`
- [ ] Episode sizes are fetched once per refresh, not once per user
- [ ] A user is "completed" for a season when every available episode is played; missing episodes don't count against them
- [ ] A user with at least one, but not all, available episodes played is "in progress" with the correct played / available count; users who haven't started aren't listed
- [ ] Season size is the sum of its episode sizes; series size is the sum of its seasons, including Specials even when they're hidden
- [ ] Unknown sizes are left out of sums and noted, e.g. "38.1 GB + 2 unknown"
- [ ] Season 0 is hidden unless `INCLUDE_SPECIALS=true`
- [ ] A season with no poster of its own uses the series poster
- [ ] Every season of a series can be seen without leaving the list, and expand/collapse (if used) works with the keyboard
- [ ] Tests against a fake Jellyfin cover REQUIREMENTS.md acceptance criteria 3–6, plus a series fully completed by everyone and one nobody has watched
