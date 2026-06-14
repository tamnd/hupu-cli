package hupu_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/hupu-cli/hupu"
)

const mockHTML = `<!DOCTYPE html>
<html>
<body>
<ul>
  <li><a href="/639971907.html" target="_blank" class=" false"><span class="t-title">发给身边的xxn看</span></a></li>
  <li><a href="/639971908.html" target="_blank" class=" false"><span class="t-title">今天训练课总结</span></a></li>
  <li><a href="/639971909.html" target="_blank" class=" false"><span class="t-title">篮球赛精彩集锦</span></a></li>
</ul>
</body>
</html>`

const mockHTMLDup = `<!DOCTYPE html>
<html>
<body>
<ul>
  <li><a href="/111.html" target="_blank" class=" false"><span class="t-title">First post</span></a></li>
  <li><a href="/111.html" target="_blank" class=" false"><span class="t-title">First post</span></a></li>
  <li><a href="/222.html" target="_blank" class=" false"><span class="t-title">Second post</span></a></li>
</ul>
</body>
</html>`

func newTestClient(ts *httptest.Server) *hupu.Client {
	cfg := hupu.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return hupu.NewClient(cfg)
}

func TestHotSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHotParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	posts, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d posts, want 3", len(posts))
	}

	p := posts[0]
	if p.Rank != 1 {
		t.Errorf("rank = %d, want 1", p.Rank)
	}
	if p.ID != "639971907" {
		t.Errorf("id = %q, want 639971907", p.ID)
	}
	if p.Title != "发给身边的xxn看" {
		t.Errorf("title = %q", p.Title)
	}
	if p.URL != "https://bbs.hupu.com/639971907.html" {
		t.Errorf("url = %q", p.URL)
	}
}

func TestHotDeduplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTMLDup))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	posts, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2 (deduped)", len(posts))
	}
	if posts[0].ID != "111" {
		t.Errorf("first post id = %q, want 111", posts[0].ID)
	}
	if posts[1].ID != "222" {
		t.Errorf("second post id = %q, want 222", posts[1].ID)
	}
}

func TestHotRespectsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	posts, err := c.Hot(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2 (limit)", len(posts))
	}
}

func TestHotRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	cfg := hupu.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := hupu.NewClient(cfg)

	start := time.Now()
	posts, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) == 0 {
		t.Error("got 0 posts after retries")
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
