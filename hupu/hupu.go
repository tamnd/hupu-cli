// Package hupu is the library behind the hupu command line:
// the HTTP client, request shaping, and the typed data models for hupu.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package hupu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to hupu. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "Mozilla/5.0 (compatible; hupu-cli/0.1; +https://github.com/tamnd/hupu-cli)"

// Host is the forum host this client talks to.
const Host = "bbs.hupu.com"

// baseURL is the root for the BBS hot list.
const baseURL = "https://bbs.hupu.com"

// searchURL is the mobile API endpoint for full-text search.
const searchURL = "https://games.mobileapi.hupu.com/7.5.80/search/v2"

// Client talks to hupu over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 200ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/json,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// Post is one thread on the BBS.
type Post struct {
	TID   string `json:"tid"   kit:"id" table:"tid"`
	Title string `json:"title"          table:"title"`
	URL   string `json:"url"            table:"url,url"`
}

// anchorRE captures the full content of a thread anchor element.
// Group 1: 9-digit tid. Group 2: inner HTML of the anchor.
var anchorRE = regexp.MustCompile(`(?s)<a[^>]*href="/(\d{9})\.html"[^>]*>(.*?)</a>`)

// tagStripRE strips all HTML tags so we can get the plain text from inner HTML.
var tagStripRE = regexp.MustCompile(`<[^>]+>`)

// tidOnlyRE is the last-resort fallback to collect tids with no title.
var tidOnlyRE = regexp.MustCompile(`href="/(\d{9})\.html"`)

// Hot scrapes the BBS home page and returns the hot thread list.
func (c *Client) Hot(ctx context.Context, limit int) ([]*Post, error) {
	body, err := c.Get(ctx, baseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("hot: %w", err)
	}
	return parseHotBody(body, limit), nil
}

// parseHotBody extracts Post records from a BBS home page HTML body.
// Exported for testing; keeps Hot thin and testable without a live server.
func parseHotBody(body []byte, limit int) []*Post {
	seen := map[string]bool{}
	var out []*Post

	// First pass: match full anchor elements to get tid + inner text as title.
	for _, m := range anchorRE.FindAllSubmatch(body, -1) {
		tid := string(m[1])
		if seen[tid] {
			continue
		}
		seen[tid] = true
		inner := tagStripRE.ReplaceAllString(string(m[2]), " ")
		title := strings.TrimSpace(strings.Join(strings.Fields(inner), " "))
		out = append(out, &Post{
			TID:   tid,
			Title: title,
			URL:   baseURL + "/" + tid + ".html",
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}

	// Second pass: pick up tids the first regex missed.
	if len(out) == 0 || (limit > 0 && len(out) < limit) {
		for _, m := range tidOnlyRE.FindAllSubmatch(body, -1) {
			tid := string(m[1])
			if seen[tid] {
				continue
			}
			seen[tid] = true
			out = append(out, &Post{
				TID: tid,
				URL: baseURL + "/" + tid + ".html",
			})
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}

	return out
}

// searchResp is the shape returned by the mobile search API.
type searchResp struct {
	Data struct {
		Thread struct {
			List []struct {
				TID   int    `json:"tid"`
				Title string `json:"title"`
			} `json:"list"`
		} `json:"thread"`
	} `json:"data"`
}

// Search queries the Hupu mobile search API and returns matching posts.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]*Post, error) {
	u := searchURL + "?query=" + url.QueryEscape(query) + "&page=1&type=all"
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var resp searchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("search decode: %w", err)
	}

	var out []*Post
	for _, item := range resp.Data.Thread.List {
		tid := fmt.Sprintf("%d", item.TID)
		out = append(out, &Post{
			TID:   tid,
			Title: item.Title,
			URL:   baseURL + "/" + tid + ".html",
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
