// Package jellyfin is a read-only client for the Jellyfin HTTP API.
//
// The client's only request primitive is get: it cannot send POST, PUT or
// DELETE (REQUIREMENTS.md §9.1). The API key is sent in the Authorization
// header only, never in a URL, and never appears in returned errors.
package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultPageSize = 500

// Client talks to one Jellyfin server.
type Client struct {
	base     *url.URL
	apiKey   string
	http     *http.Client
	pageSize int
}

// New returns a client for the server at baseURL. timeout bounds each
// request.
func New(baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("jellyfin: invalid server URL")
	}
	return &Client{
		base:     u,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: timeout},
		pageSize: defaultPageSize,
	}, nil
}

// Get sends GET path?query to Jellyfin and decodes the JSON response into out.
// path must already be escaped.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.get(ctx, path, query, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jellyfin: GET %s: decoding response: %w", path, err)
	}
	return nil
}

// get is the client's only request primitive, and it can only send GET. It
// returns the response if the status is 200 OK, and the caller closes its
// body. path is an escaped path such as "/Items/abc/Images/Primary".
func (c *Client) get(ctx context.Context, path string, query url.Values, accept string) (*http.Response, error) {
	u := *c.base
	u.RawPath = strings.TrimRight(c.base.EscapedPath(), "/") + path
	unescaped, err := url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, fmt.Errorf("jellyfin: GET %s: %w", path, err)
	}
	u.Path = unescaped
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("jellyfin: GET %s: %w", path, err)
	}
	req.Header.Set("Authorization", "MediaBrowser Token="+c.apiKey)
	req.Header.Set("Accept", accept)

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error includes the URL, which never contains the key.
		return nil, fmt.Errorf("jellyfin: GET %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		return nil, fmt.Errorf("jellyfin: GET %s: unexpected status %s", path, resp.Status)
	}
	return resp, nil
}

// Item is the subset of Jellyfin's BaseItemDto the dashboard uses.
type Item struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	Type           string `json:"Type"`
	ProductionYear int    `json:"ProductionYear,omitempty"`
	// DateCreated is when the item was added to the library. Confirmed
	// against Jellyfin 12.1.0: it is only returned when the request asks for
	// fields=DateCreated, formatted like "2026-04-05T21:22:25.7865705Z".
	DateCreated time.Time `json:"DateCreated,omitzero"`
	// ImageTags maps an image type ("Primary", "Logo", …) to a tag that
	// changes whenever that image does. Confirmed against Jellyfin 12.1.0:
	// /Items returns it without asking for any fields, and an item with no
	// poster has no "Primary" key.
	ImageTags map[string]string `json:"ImageTags,omitempty"`
	// MediaSources lists the item's versions (e.g. 1080p and 4K). Only
	// returned when the request asks for fields=MediaSources.
	MediaSources []MediaSource `json:"MediaSources,omitempty"`

	// SeriesID is the series a season or episode belongs to. Confirmed
	// against Jellyfin 12.1.0: returned on seasons and episodes by default.
	SeriesID string `json:"SeriesId,omitempty"`
	// SeasonID is the season an episode belongs to. Confirmed against
	// Jellyfin 12.1.0: absent for an episode Jellyfin couldn't place in a
	// season (it then has SeasonName "Season Unknown", and its series has a
	// Virtual "Season Unknown" season with no IndexNumber and no episodes).
	SeasonID   string `json:"SeasonId,omitempty"`
	SeasonName string `json:"SeasonName,omitempty"`
	// IndexNumber is a season's number: 0 for Specials, absent (nil) for
	// "Season Unknown".
	IndexNumber *int `json:"IndexNumber,omitempty"`
	// UserData is the requesting user's watch state; only returned when the
	// request has a userId.
	UserData *UserData `json:"UserData,omitempty"`
}

// UserData is one user's watch state for an item.
type UserData struct {
	// Played is true once the user has finished the item or marked it
	// watched manually.
	Played bool `json:"Played"`
}

// PrimaryImageTag is the tag of the item's poster, or "" if it has none.
func (it Item) PrimaryImageTag() string { return it.ImageTags["Primary"] }

// MediaSource is one version of an item.
//
// Confirmed against Jellyfin 12.1.0: with fields=MediaSources each movie has
// a MediaSources array (one entry per version) and each entry has Size, the
// file size in bytes as a JSON integer (e.g. 20559445148). Not yet checked on
// 10.10 or 10.11; a missing or null Size decodes as 0, meaning unknown.
type MediaSource struct {
	Size int64 `json:"Size,omitempty"`
}

// User is one Jellyfin user.
type User struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

type itemsPage struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}

// items fetches every item matching query, one page at a time.
//
// Confirmed against Jellyfin 12.1.0: /Items returns {Items, TotalRecordCount,
// StartIndex}; TotalRecordCount is the full count regardless of limit, and a
// startIndex past the end returns an empty Items array.
func (c *Client) items(ctx context.Context, query url.Values) ([]Item, error) {
	var all []Item
	for {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("startIndex", strconv.Itoa(len(all)))
		q.Set("limit", strconv.Itoa(c.pageSize))

		var page itemsPage
		if err := c.Get(ctx, "/Items", q, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.TotalRecordCount {
			return all, nil
		}
	}
}

// Movies returns every movie in the library, sorted by Jellyfin's sort name.
func (c *Client) Movies(ctx context.Context) ([]Item, error) {
	return c.items(ctx, url.Values{
		"includeItemTypes": {"Movie"},
		"recursive":        {"true"},
		"sortBy":           {"SortName"},
		"sortOrder":        {"Ascending"},
		"fields":           {"DateCreated,MediaSources"},
	})
}

// Users returns every user, in the order Jellyfin lists them.
//
// Confirmed against Jellyfin 12.1.0: GET /Users with an admin API key returns
// a bare JSON array of UserDto (not an {Items} page), including disabled and
// hidden users.
func (c *Client) Users(ctx context.Context) ([]User, error) {
	var users []User
	if err := c.Get(ctx, "/Users", nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// PlayedMovieIDs returns the IDs of the movies Jellyfin reports as played
// (UserData.Played = true) for one user, including those marked watched
// manually.
//
// It filters with isPlayed=true alone, never IsPlayed/IsUnplayed in filters,
// which Jellyfin applies only one of (REQUIREMENTS.md §5). Confirmed against
// Jellyfin 12.1.0: with userId, isPlayed=true returns exactly the movies whose
// UserData.Played is true for that user; without userId the server answers
// 500.
func (c *Client) PlayedMovieIDs(ctx context.Context, userID string) ([]string, error) {
	items, err := c.items(ctx, url.Values{
		"userId":           {userID},
		"includeItemTypes": {"Movie"},
		"recursive":        {"true"},
		"isPlayed":         {"true"},
		"enableImages":     {"false"},
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids, nil
}

// Series returns every series in the library, sorted by Jellyfin's sort
// name.
func (c *Client) Series(ctx context.Context) ([]Item, error) {
	return c.items(ctx, url.Values{
		"includeItemTypes": {"Series"},
		"recursive":        {"true"},
		"sortBy":           {"SortName"},
		"sortOrder":        {"Ascending"},
		"fields":           {"DateCreated"},
	})
}

// Seasons returns every season of every series, including Specials (season
// 0) and seasons with no episodes on disk.
//
// Confirmed against Jellyfin 12.1.0: each season has SeriesId, IndexNumber
// and ImageTags without asking for any fields; ImageTags has no "Primary"
// key when the season has no poster of its own. Jellyfin can list two
// "Season 1"s for one series (e.g. "Season 1" and "Season 01" folders), one
// of them with no episodes.
func (c *Client) Seasons(ctx context.Context) ([]Item, error) {
	return c.items(ctx, url.Values{
		"includeItemTypes": {"Season"},
		"recursive":        {"true"},
	})
}

// Episodes returns every episode that exists in the library, with its file
// sizes. It is fetched once per refresh, not once per user.
//
// Confirmed against Jellyfin 12.1.0: with fields=MediaSources each episode
// has one MediaSources entry per file with Size in bytes, as for movies;
// isMissing=false is accepted without a userId. The test server had no
// missing episodes, so the filter itself couldn't be seen removing any.
func (c *Client) Episodes(ctx context.Context) ([]Item, error) {
	return c.items(ctx, url.Values{
		"includeItemTypes": {"Episode"},
		"recursive":        {"true"},
		"isMissing":        {"false"},
		"fields":           {"MediaSources"},
		"enableImages":     {"false"},
	})
}

// UserEpisodes returns the episodes one user can see, each with their
// UserData.Played, in one paginated request rather than one per season.
//
// Confirmed against Jellyfin 12.1.0: UserData is returned with
// enableUserData=true and a userId, and a user only gets the episodes of
// libraries they can access (two of six users saw 616 of 626 episodes).
func (c *Client) UserEpisodes(ctx context.Context, userID string) ([]Item, error) {
	return c.items(ctx, url.Values{
		"userId":           {userID},
		"includeItemTypes": {"Episode"},
		"recursive":        {"true"},
		"isMissing":        {"false"},
		"enableUserData":   {"true"},
		"enableImages":     {"false"},
	})
}

// maxImageBytes caps a poster download. A 96×144 poster is 5–15 KB.
const maxImageBytes = 4 << 20

// Image is an image downloaded from Jellyfin.
type Image struct {
	Data        []byte
	ContentType string // e.g. "image/webp"
}

// PrimaryImage downloads an item's poster, scaled to fill about 96×144:
// twice the 48×72 slot, for high-density screens.
//
// Confirmed against Jellyfin 12.1.0 (not yet checked on 10.10 or 10.11):
// GET /Items/{id}/Images/Primary returns the image bytes. fillWidth and
// fillHeight scale to about 96×144 (some come back 97×144 or 96×145; the
// page crops to fill) at ~6 KB, against ~500 KB unscaled. The
// response format follows Accept ("Vary: Accept"): image/webp when offered,
// otherwise image/jpeg. An item with no poster answers 404, and a malformed
// ID 400.
func (c *Client) PrimaryImage(ctx context.Context, itemID string) (Image, error) {
	path := "/Items/" + url.PathEscape(itemID) + "/Images/Primary"
	q := url.Values{"fillWidth": {"96"}, "fillHeight": {"144"}, "quality": {"90"}}
	resp, err := c.get(ctx, path, q, "image/webp,image/jpeg;q=0.9")
	if err != nil {
		return Image{}, err
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return Image{}, fmt.Errorf("jellyfin: GET %s: not an image (Content-Type %q)", path, ct)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return Image{}, fmt.Errorf("jellyfin: GET %s: reading image: %w", path, err)
	}
	if len(data) > maxImageBytes {
		return Image{}, fmt.Errorf("jellyfin: GET %s: image larger than %d bytes", path, maxImageBytes)
	}
	return Image{Data: data, ContentType: ct}, nil
}
