# 11: Collapsed TV shows with a series completion roll-up

**What to build:** The TV shows tab starts with every series collapsed, and each series row rolls its seasons' completion up into one line in the "Completed by" column. A user who has played every available episode of the series shows as completed. A user who has played some shows as in progress, with played / available episodes across the whole series. For example, someone halfway through season 3 of a show with three 10-episode seasons shows "25 / 30". When a series is expanded, its seasons show their own completion as now, and the series row's roll-up is hidden so the same facts aren't shown twice.

**Blocked by:** 06 (TV shows list with season completion)

**Status:** ready-for-agent

- [ ] Every series starts collapsed, whatever its season count (this replaces the "more than 5 seasons" threshold). "Expand all", `?seasons=all` and expanding a single series behave as before.
- [ ] A collapsed series row shows the `completion` component for the whole series: completed users' tags, then each in-progress user's tag with "played / available" and the bar, or "Not completed by anyone".
- [ ] Roll-up counts use the same episodes as the visible seasons: available (non-missing) episodes only, and Season 0 only when `INCLUDE_SPECIALS=true`.
- [ ] A user is completed for a series when every counted episode is played, which matches being completed on every visible season. A user is in progress with at least one but not all played. Users who haven't started aren't listed.
- [ ] A series with no counted episodes shows no roll-up.
- [ ] Expanding a series hides its row's roll-up and shows the season rows. Collapsing it shows the roll-up again. Both work with the keyboard, without JavaScript, and after the self-reload that follows a refresh.
- [ ] Sizes, search, sort and the series disclosure are otherwise unchanged. The roll-up is worked out in `internal/library` from the per-episode data already fetched, with no extra Jellyfin requests.
- [ ] Tests against a fake Jellyfin cover:
  - a series completed by everyone
  - one nobody has watched
  - a user partway through a later season (the "25 / 30" case)
  - a user who completed some seasons but not others
  - a series whose hidden Specials are left out of the counts
- [ ] The gallery shows a collapsed series row with completed and in-progress roll-ups, including "1 / 200" and all 10 users, and an expanded row with the roll-up hidden. Screenshots at desktop and 380 px show no horizontal scroll.
