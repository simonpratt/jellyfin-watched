# Jellyfin-watched

Shows which people on a Jellyfin server have finished each movie and TV season, with its size on disk, so the administrator can decide what to delete. It shows facts and passes no judgement.

## Language

### People

**User**:
A Jellyfin user whose watch state the dashboard shows.
_Avoid_: viewer, account, profile, member

**Administrator**:
The one person who uses the dashboard to look through the library.
_Avoid_: user (that is a Jellyfin user), owner, admin user

**User tag**:
The mark that stands for a user wherever they appear: a two-letter code and a hue. The code tells users apart; the hue only speeds scanning.
_Avoid_: avatar, badge, chip

### The library

**Library**:
Every movie and series on the Jellyfin server.
_Avoid_: collection, catalogue, media

**Movie**:
One film in the library, counting all its versions as one.
_Avoid_: film, title, item

**Series**:
One TV show in the library, made up of its seasons.
_Avoid_: show, programme, item

**Season**:
One numbered season of a series, made up of its available episodes.
_Avoid_: series (that is the whole show)

**Specials**:
Season 0 of a series. Hidden unless the setting asks for it, and always counted in the series' size.
_Avoid_: extras, season zero

**Available episode**:
An episode that exists in the library, as opposed to a missing placeholder Jellyfin lists for an unaired or absent episode.
_Avoid_: aired episode, existing episode

**Poster**:
The cover image of a movie, series or season. A season with no poster of its own uses its series' poster.
_Avoid_: cover art, artwork, thumbnail

**Size**:
How much disk space a movie, season or series takes. A size Jellyfin doesn't report is unknown, never 0.
_Avoid_: file size, storage, weight

### Completion

**Played**:
Jellyfin's per-user mark that a user has watched a movie or episode, including when it was marked by hand.
_Avoid_: watched, seen, viewed

**Completed**:
A user has played a movie, or every available episode of a season or series.
_Avoid_: watched, finished, done, seen

**In progress**:
A user has played some, but not all, of the available episodes of a season or series, shown as played / available ("4 / 10").
_Avoid_: partially watched, started, watching

**Completion**:
Which users have completed an item and which are in progress on it.
_Avoid_: progress, watch status, stats

**Roll-up**:
A collapsed series' completion, worked out across the episodes of all its visible seasons.
_Avoid_: summary, total, aggregate

### Freshness

**Refresh**:
Fetching the whole library and every user's played state from Jellyfin. Pages show the result of the last refresh.
_Avoid_: sync, reload, update

**Stale**:
The last refresh failed, so the pages show the previous refresh's data with a notice.
_Avoid_: offline, out of sync, error state
