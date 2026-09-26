# 04: Movie size and who completed it

**What to build:** Each movie row shows its size on disk and the users who have completed it, rendered with the designed size and user-tag components. A movie is completed by a user when Jellyfin reports `Played = true` for them. This includes movies marked watched manually.

**Blocked by:** 02 (Design foundation and page shell)

**Status:** ready-for-agent

- [ ] Users are fetched from `GET /Users`, and each user's played movies come from a request with `isPlayed=true` (never both IsPlayed and IsUnplayed filters)
- [ ] The size is the sum of `MediaSources[].Size` across all versions; confirm this field on Jellyfin 10.10 and 10.11
- [ ] Sizes use binary units consistently (1 GB = 1024³ bytes), e.g. "14.6 GB"
- [ ] A movie with no reported size shows "Unknown", not 0
- [ ] A movie shows exactly the users Jellyfin reports as having played it, and "Not completed by anyone" when there are none
- [ ] User tags are consistent across the app, don't rely on colour alone, and carry no good/bad meaning
- [ ] Nothing styles large sizes or unwatched items differently from other rows (DESIGN.md §3)
- [ ] Tests against a fake Jellyfin cover multiple versions, unknown size, zero users and all users
