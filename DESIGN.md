# Jellyfin Watch-Completion Dashboard: Design Brief

This brief is for the designer. It describes what the interface needs to show and do, and the constraints it has to work within. It does not prescribe a visual style. That part is yours.

The functional and technical requirements are in `REQUIREMENTS.md`. Where the two documents disagree, `REQUIREMENTS.md` wins.

## 1. What this is

A private web dashboard for the administrator of a Jellyfin media server (a self-hosted alternative to Plex). It lists every movie and every TV season on the server, with cover art and size on disk, and shows which of the server's users have finished watching each one.

The administrator uses it to scan the library and decide, on their own, what to delete to free up space. The dashboard itself never deletes anything and never suggests what to delete.

## 2. Who uses it and how

- **One person:** the server administrator. Technically comfortable, familiar with tools like Radarr and Sonarr.
- **Occasional use:** opened every few weeks when disk space is getting low, not every day.
- **The job:** scroll through a long list, take in each item at a glance (what it is, how big it is, who has finished it), and move on. The typical session is scanning, not reading.
- **Mostly on a desktop browser**, sometimes on a phone.

## 3. The design principle that matters most

**The interface is neutral. It shows facts and never passes judgement.**

This is the hardest constraint in the brief and it affects almost every decision:

- Nothing may look like a recommendation. No "safe to delete" badges, no warning colours on items nobody has watched, no highlighting of large files.
- Avoid colour meanings that imply good or bad. For example, don't show "completed" in green and "not completed" in red, and don't style large sizes in an alarming way. "Nobody has finished this" is just a fact, and it should look as calm as "three people have finished this".
- Sorting by size or by date added is a normal sort, presented the same way as sorting by title.

The administrator makes the decision. The design's job is to make the facts fast to read.

## 4. Reference point

The layout should feel familiar to someone who uses Radarr and Sonarr: a list of media where each row has a poster on the left and details beside it. Use them as a structural reference only. The design should not copy their branding, colours, or look.

## 5. Screens

There are two main screens and a password screen.

### 5.1 Movies

A long list of every movie. Each row shows:

| Element | Notes |
|---|---|
| Poster | Portrait, 2:3 ratio. |
| Title | Can be long (see §7). |
| Year | |
| Size on disk | e.g. "14.6 GB". Can be "Unknown". |
| Who completed it | Zero or more user names. If none: a plain statement, e.g. "Not completed by anyone". |

### 5.2 TV shows

A long list of every series. Each series shows:

| Element | Notes |
|---|---|
| Poster | Series poster, 2:3. |
| Title and year | |
| Total size | Sum of all seasons. |
| Seasons | Each season listed beneath the series (see below). |

Each season shows:

| Element | Notes |
|---|---|
| Season name | e.g. "Season 1". Season posters are optional in the design; if used, a season without its own poster falls back to the series poster. |
| Size on disk | |
| Completed by | Zero or more user names. |
| In progress | Zero or more user names, each with their progress, e.g. "Sam 4 / 10". |

Seasons may be shown inline under each series or in an expandable section. Choose whichever reads better, but the administrator must be able to see every season of a show without leaving the list. If you use expand/collapse, consider an "expand all" control.

A season's completion has three possible states for each user: completed, in progress (with a count), or not started. "Not started" users are not listed, so a season row only names the users who have done something.

### 5.3 Password screen

Shown only if the administrator has set a password. A single password field and a button. This is the only form in the app.

## 6. Controls

Shared by both main screens:

| Control | Priority | Notes |
|---|---|---|
| Switch between Movies and TV shows | Must | Two top-level sections. |
| Search by title | Must | Filters the list as you type. |
| Sort | Should | Title, year, date added, size. Ascending and descending. |
| Refresh now | Should | Fetches fresh data from Jellyfin. A full refresh can take up to about a minute. |
| Last refreshed | Should | e.g. "Updated 12 minutes ago". |
| List or poster grid | Could | An alternative denser grid layout. Design the list first. |

There are no filters beyond search. There is no per-user view and no settings screen. All settings are configured on the server.

## 7. Realistic content

Please design with real-feeling content at the extremes, not just ideal cases.

| What | Typical | Extremes to design for |
|---|---|---|
| Movies in the library | 300–1,000 | 3,000 |
| Series | 50–200 | 500 |
| Seasons per series | 1–5 | 30+ (long-running shows) |
| Episodes per season | 8–13 | 200+ (daily or anime series) |
| Users on the server | 2–5 | 10 |
| User name length | 4–8 characters | 20 characters |
| Users who completed one item | 0–2 | all 10 |
| Size of a movie | 2–15 GB | 80 GB (4K remux) |
| Size of a series | 10–100 GB | 2 TB |
| Title length | Short | "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb" |

Other edge cases:

- A poster that is missing entirely.
- A size that Jellyfin doesn't report ("Unknown"), and a sum that includes unknowns (e.g. "38.1 GB + 2 unknown").
- A series where every season has been completed by every user, and one where no one has watched anything.
- A season with a user at "0 / 10" never appears (they'd be "not started"), but "1 / 200" can.

**On user names:** it may help scanning if each user is visually distinct and consistent across the app (for example, the same colour or mark every time a user appears). If you do this, colour must not be the only way to tell users apart, and the colours must not carry good/bad meaning (see §3).

## 8. States to design

| State | When it happens |
|---|---|
| First load | The app has just started and is fetching data for the first time. This can take up to about a minute. |
| Refreshing | Refresh is running; the existing list stays visible and usable. |
| Stale data | Jellyfin couldn't be reached. The last good data is still shown, with a visible but non-alarming notice explaining that data may be out of date and when it was last updated. |
| No search results | e.g. "No movies match 'xyz'". |
| Empty library | The server has no movies (or no shows). |
| Wrong password | On the password screen. |
| Image loading | Posters load lazily while scrolling; the layout must not jump as they arrive. |

## 9. Technical constraints

These affect what's practical to design:

- **Server-rendered HTML.** Pages are built with Go templates on the server. Interactions (search, sort, expand, refresh) swap in chunks of HTML using htmx. There is no JavaScript framework.
- **Keep motion and client-side behaviour simple.** Small transitions are fine. Anything that needs a lot of custom JavaScript (drag and drop, virtualised scrolling, complex animation) should be discussed first.
- **No external resources at runtime.** The app runs on a home network and embeds everything it needs. Any fonts must be self-hostable (e.g. open-licensed files we can bundle), and no icons or images can come from a CDN. System font stacks are a perfectly good option.
- **Light and dark themes**, following the operating system setting. Designing both is required.
- **Long lists.** Pages can hold a thousand rows. Row designs should be compact enough to scan quickly and simple enough to render quickly.

## 10. Accessibility and responsiveness

- Text and meaningful graphics meet WCAG 2.2 AA contrast in both themes.
- Nothing relies on colour alone, including user identity and completion state.
- Visible keyboard focus on every interactive element. The whole app is usable with a keyboard.
- Respect reduced-motion settings.
- **Desktop first**, but usable on a phone around 380 px wide. On small screens, rows may stack; nothing should require sideways scrolling of the whole page.
- Posters need alt text; the design should assume it will be the item's title.

## 11. Writing

- Sentence case everywhere ("Refresh now", not "Refresh Now").
- Plain, factual wording. "Not completed by anyone", not "Unwatched!" or "Nobody cares about this one".
- Buttons say exactly what they do.
- Notices explain what happened and what the app is doing about it, without apologising or alarming.

Key strings the design needs to accommodate:

- "Movies", "TV shows"
- "Search titles"
- "Sort by: Title / Year / Date added / Size"
- "Refresh now", "Refreshing…", "Updated 12 minutes ago"
- "Completed by", "In progress", "Not completed by anyone"
- "Can't reach Jellyfin. Showing data from 3 hours ago."
- "Loading your library. This can take a minute on first start."

## 12. Deliverables

Please provide:

1. **Screens**, in light and dark: Movies list, TV shows list (with seasons visible), password screen. Desktop and phone widths.
2. **States** from §8.
3. **Components** with their variants: movie row, series row, season row, user name/tag, size, search, sort control, refresh control, notices.
4. **Design tokens** that map directly to CSS custom properties: colours for both themes, type scale, spacing scale, radii, and any shadows. A CSS file or a table is ideal, since these will be implemented by hand in plain CSS.
5. **Fonts** (if not using system fonts), with their licence.
6. **Favicon**. Optional, and a simple mark is enough.

Figma or a static HTML/CSS mockup are both fine.

## 13. Out of scope

- Any delete, edit, or "mark as watched" action.
- Per-user pages or statistics.
- Settings, onboarding, or user management.
- Branding beyond a name and an optional favicon. The working name is "Jellyfin Watch-Completion Dashboard"; a shorter name is welcome.# Jellyfin Watch-Completion Dashboard: Design Brief

This brief is for the designer. It describes what the interface needs to show and do, and the constraints it has to work within. It does not prescribe a visual style. That part is yours.

The functional and technical requirements are in `REQUIREMENTS.md`. Where the two documents disagree, `REQUIREMENTS.md` wins.

## 1. What this is

A private web dashboard for the administrator of a Jellyfin media server (a self-hosted alternative to Plex). It lists every movie and every TV season on the server, with cover art and size on disk, and shows which of the server's users have finished watching each one.

The administrator uses it to scan the library and decide, on their own, what to delete to free up space. The dashboard itself never deletes anything and never suggests what to delete.

## 2. Who uses it and how

- **One person:** the server administrator. Technically comfortable, familiar with tools like Radarr and Sonarr.
- **Occasional use:** opened every few weeks when disk space is getting low, not every day.
- **The job:** scroll through a long list, take in each item at a glance (what it is, how big it is, who has finished it), and move on. The typical session is scanning, not reading.
- **Mostly on a desktop browser**, sometimes on a phone.

## 3. The design principle that matters most

**The interface is neutral. It shows facts and never passes judgement.**

This is the hardest constraint in the brief and it affects almost every decision:

- Nothing may look like a recommendation. No "safe to delete" badges, no warning colours on items nobody has watched, no highlighting of large files.
- Avoid colour meanings that imply good or bad. For example, don't show "completed" in green and "not completed" in red, and don't style large sizes in an alarming way. "Nobody has finished this" is just a fact, and it should look as calm as "three people have finished this".
- Sorting by size or by date added is a normal sort, presented the same way as sorting by title.

The administrator makes the decision. The design's job is to make the facts fast to read.

## 4. Reference point

The layout should feel familiar to someone who uses Radarr and Sonarr: a list of media where each row has a poster on the left and details beside it. Use them as a structural reference only. The design should not copy their branding, colours, or look.

## 5. Screens

There are two main screens and a password screen.

### 5.1 Movies

A long list of every movie. Each row shows:

| Element | Notes |
|---|---|
| Poster | Portrait, 2:3 ratio. |
| Title | Can be long (see §7). |
| Year | |
| Size on disk | e.g. "14.6 GB". Can be "Unknown". |
| Who completed it | Zero or more user names. If none: a plain statement, e.g. "Not completed by anyone". |

### 5.2 TV shows

A long list of every series. Each series shows:

| Element | Notes |
|---|---|
| Poster | Series poster, 2:3. |
| Title and year | |
| Total size | Sum of all seasons. |
| Seasons | Each season listed beneath the series (see below). |

Each season shows:

| Element | Notes |
|---|---|
| Season name | e.g. "Season 1". Season posters are optional in the design; if used, a season without its own poster falls back to the series poster. |
| Size on disk | |
| Completed by | Zero or more user names. |
| In progress | Zero or more user names, each with their progress, e.g. "Sam 4 / 10". |

Seasons may be shown inline under each series or in an expandable section. Choose whichever reads better, but the administrator must be able to see every season of a show without leaving the list. If you use expand/collapse, consider an "expand all" control.

A season's completion has three possible states for each user: completed, in progress (with a count), or not started. "Not started" users are not listed, so a season row only names the users who have done something.

### 5.3 Password screen

Shown only if the administrator has set a password. A single password field and a button. This is the only form in the app.

## 6. Controls

Shared by both main screens:

| Control | Priority | Notes |
|---|---|---|
| Switch between Movies and TV shows | Must | Two top-level sections. |
| Search by title | Must | Filters the list as you type. |
| Sort | Should | Title, year, date added, size. Ascending and descending. |
| Refresh now | Should | Fetches fresh data from Jellyfin. A full refresh can take up to about a minute. |
| Last refreshed | Should | e.g. "Updated 12 minutes ago". |
| List or poster grid | Could | An alternative denser grid layout. Design the list first. |

There are no filters beyond search. There is no per-user view and no settings screen. All settings are configured on the server.

## 7. Realistic content

Please design with real-feeling content at the extremes, not just ideal cases.

| What | Typical | Extremes to design for |
|---|---|---|
| Movies in the library | 300–1,000 | 3,000 |
| Series | 50–200 | 500 |
| Seasons per series | 1–5 | 30+ (long-running shows) |
| Episodes per season | 8–13 | 200+ (daily or anime series) |
| Users on the server | 2–5 | 10 |
| User name length | 4–8 characters | 20 characters |
| Users who completed one item | 0–2 | all 10 |
| Size of a movie | 2–15 GB | 80 GB (4K remux) |
| Size of a series | 10–100 GB | 2 TB |
| Title length | Short | "Dr. Strangelove or: How I Learned to Stop Worrying and Love the Bomb" |

Other edge cases:

- A poster that is missing entirely.
- A size that Jellyfin doesn't report ("Unknown"), and a sum that includes unknowns (e.g. "38.1 GB + 2 unknown").
- A series where every season has been completed by every user, and one where no one has watched anything.
- A season with a user at "0 / 10" never appears (they'd be "not started"), but "1 / 200" can.

**On user names:** it may help scanning if each user is visually distinct and consistent across the app (for example, the same colour or mark every time a user appears). If you do this, colour must not be the only way to tell users apart, and the colours must not carry good/bad meaning (see §3).

## 8. States to design

| State | When it happens |
|---|---|
| First load | The app has just started and is fetching data for the first time. This can take up to about a minute. |
| Refreshing | Refresh is running; the existing list stays visible and usable. |
| Stale data | Jellyfin couldn't be reached. The last good data is still shown, with a visible but non-alarming notice explaining that data may be out of date and when it was last updated. |
| No search results | e.g. "No movies match 'xyz'". |
| Empty library | The server has no movies (or no shows). |
| Wrong password | On the password screen. |
| Image loading | Posters load lazily while scrolling; the layout must not jump as they arrive. |

## 9. Technical constraints

These affect what's practical to design:

- **Server-rendered HTML.** Pages are built with Go templates on the server. Interactions (search, sort, expand, refresh) swap in chunks of HTML using htmx. There is no JavaScript framework.
- **Keep motion and client-side behaviour simple.** Small transitions are fine. Anything that needs a lot of custom JavaScript (drag and drop, virtualised scrolling, complex animation) should be discussed first.
- **No external resources at runtime.** The app runs on a home network and embeds everything it needs. Any fonts must be self-hostable (e.g. open-licensed files we can bundle), and no icons or images can come from a CDN. System font stacks are a perfectly good option.
- **Light and dark themes**, following the operating system setting. Designing both is required.
- **Long lists.** Pages can hold a thousand rows. Row designs should be compact enough to scan quickly and simple enough to render quickly.

## 10. Accessibility and responsiveness

- Text and meaningful graphics meet WCAG 2.2 AA contrast in both themes.
- Nothing relies on colour alone, including user identity and completion state.
- Visible keyboard focus on every interactive element. The whole app is usable with a keyboard.
- Respect reduced-motion settings.
- **Desktop first**, but usable on a phone around 380 px wide. On small screens, rows may stack; nothing should require sideways scrolling of the whole page.
- Posters need alt text; the design should assume it will be the item's title.

## 11. Writing

- Sentence case everywhere ("Refresh now", not "Refresh Now").
- Plain, factual wording. "Not completed by anyone", not "Unwatched!" or "Nobody cares about this one".
- Buttons say exactly what they do.
- Notices explain what happened and what the app is doing about it, without apologising or alarming.

Key strings the design needs to accommodate:

- "Movies", "TV shows"
- "Search titles"
- "Sort by: Title / Year / Date added / Size"
- "Refresh now", "Refreshing…", "Updated 12 minutes ago"
- "Completed by", "In progress", "Not completed by anyone"
- "Can't reach Jellyfin. Showing data from 3 hours ago."
- "Loading your library. This can take a minute on first start."

## 12. Deliverables

Please provide:

1. **Screens**, in light and dark: Movies list, TV shows list (with seasons visible), password screen. Desktop and phone widths.
2. **States** from §8.
3. **Components** with their variants: movie row, series row, season row, user name/tag, size, search, sort control, refresh control, notices.
4. **Design tokens** that map directly to CSS custom properties: colours for both themes, type scale, spacing scale, radii, and any shadows. A CSS file or a table is ideal, since these will be implemented by hand in plain CSS.
5. **Fonts** (if not using system fonts), with their licence.
6. **Favicon**. Optional, and a simple mark is enough.

Figma or a static HTML/CSS mockup are both fine.

## 13. Out of scope

- Any delete, edit, or "mark as watched" action.
- Per-user pages or statistics.
- Settings, onboarding, or user management.
- Branding beyond a name and an optional favicon. The working name is "Jellyfin Watch-Completion Dashboard"; a shorter name is welcome.