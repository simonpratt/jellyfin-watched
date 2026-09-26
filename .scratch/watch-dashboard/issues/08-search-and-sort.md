# 08: Search and sort

**What to build:** On both the Movies and TV shows tabs, the administrator can filter by title as they type, and sort by title, year, date added or size, ascending or descending. Results are swapped in as server-rendered htmx fragments using the designed search and sort controls. Every sort is presented the same way, and none is framed as a recommendation.

**Blocked by:** 04 (Movie size and who completed it), 06 (TV shows list with season completion)

**Status:** ready-for-agent

- [ ] Search filters as you type (debounced), is case-insensitive, and works on both tabs
- [ ] Sorting by title, year, date added and size works in both directions on both tabs; series sort by their total size
- [ ] Items with an unknown size sort in a consistent, documented position
- [ ] The search and sort state lives in the URL, so reloading or sharing the link restores the same view
- [ ] With no JavaScript, the controls still work as a plain form submit
- [ ] The page shows "No movies match '<query>'" (and the TV equivalent), plus an empty-library state for each tab
- [ ] Tests cover filtering and each sort key in both directions
