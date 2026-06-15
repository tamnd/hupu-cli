// Package hupu is the library behind the hupu command: the HTTP client,
// request shaping, and the typed data models for Hupu (虎扑) sports community.
//
// The client fetches the public BBS homepage at https://bbs.hupu.com/ and
// parses trending post links from the HTML. No authentication is required.
// It sets a desktop User-Agent, paces requests, and retries transient 429/5xx
// errors with exponential backoff.
package hupu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// searchAPIURL is the mobile search API endpoint.
const searchAPIURL = "https://games.mobileapi.hupu.com/7.5.80/search/v2"

// DefaultUserAgent mimics a desktop Chrome browser.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for the Hupu BBS client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://bbs.hupu.com",
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to the Hupu BBS over HTTP.
type Client struct {
	cfg        Config
	httpClient *http.Client
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// Hot fetches trending posts from the Hupu BBS homepage.
// It returns at most limit posts, ranked 1-based by appearance on the page.
// If limit <= 0, all found posts are returned.
func (c *Client) Hot(ctx context.Context, limit int) ([]Post, error) {
	body, err := c.get(ctx, c.cfg.BaseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("hot: %w", err)
	}
	return extractPosts(body, limit), nil
}

// get fetches url with retries and pacing. The full response body is returned.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		b, retry, err := c.do(ctx, url)
		if err == nil {
			return b, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := c.httpClient.Do(req)
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

	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
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
// If limit <= 0, all results from the first page are returned.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Post, error) {
	u := searchAPIURL + "?query=" + url.QueryEscape(query) + "&page=1&type=all"
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var resp searchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("search decode: %w", err)
	}

	var out []Post
	for i, item := range resp.Data.Thread.List {
		out = append(out, Post{
			Rank:  i + 1,
			ID:    fmt.Sprintf("%d", item.TID),
			Title: item.Title,
			URL:   "https://bbs.hupu.com/" + fmt.Sprintf("%d", item.TID) + ".html",
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
