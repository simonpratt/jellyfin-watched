// Package jellyfin is a read-only client for the Jellyfin HTTP API.
//
// The client's only request primitive is Get: it cannot send POST, PUT or
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
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + path
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("jellyfin: GET %s: %w", path, err)
	}
	req.Header.Set("Authorization", "MediaBrowser Token="+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error includes the URL, which never contains the key.
		return fmt.Errorf("jellyfin: GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("jellyfin: GET %s: unexpected status %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jellyfin: GET %s: decoding response: %w", path, err)
	}
	return nil
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
		"fields":           {"DateCreated"},
	})
}
