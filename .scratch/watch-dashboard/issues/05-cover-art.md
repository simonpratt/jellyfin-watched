# 05: Cover art passthrough

**What to build:** Movie rows show their posters. The browser requests posters from the dashboard, which fetches them from Jellyfin with the API key, caches them, and serves them back, so the key never reaches the browser. Posters load lazily while scrolling and never make the layout jump.

**Blocked by:** 02 (Design foundation and page shell)

**Status:** done

- [x] Posters are fetched from Jellyfin's primary image endpoint; confirm the endpoint on 10.10 and 10.11
- [x] Fetched images are cached so repeat views don't hit Jellyfin
- [x] The image route only serves images for items that are in the cache (it doesn't proxy arbitrary Jellyfin requests)
- [x] `<img>` tags use lazy loading and a fixed 2:3 slot, so the layout doesn't shift
- [x] A missing poster shows the designed placeholder
- [x] Alt text is the item's title
- [x] Tests prove the API key never appears in image responses or their headers
