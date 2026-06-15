package hupu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestHot(t *testing.T) {
	// Serve a minimal BBS home page with a couple of thread anchors.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
<a href="/123456789.html"><span class="t-title">Thread one</span></a>
<a href="/987654321.html"><span class="t-title">Thread two</span></a>
</body></html>`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	// Override baseURL via a monkey-patched request: we redirect the hot call
	// to the test server by temporarily rewriting the package-level constant is
	// not possible, so we test via the client's Get directly and parse.
	body, err := c.Get(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}

	posts := parseHotBody(body, 10)
	if len(posts) != 2 {
		t.Fatalf("got %d posts, want 2", len(posts))
	}
	if posts[0].TID != "123456789" {
		t.Errorf("first TID = %q, want 123456789", posts[0].TID)
	}
	if posts[0].Title != "Thread one" {
		t.Errorf("first Title = %q, want Thread one", posts[0].Title)
	}
}

func TestSearch(t *testing.T) {
	// Serve a mock search API response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q == "" {
			t.Error("query param missing")
		}
		resp := searchResp{}
		resp.Data.Thread.List = []struct {
			TID   int    `json:"tid"`
			Title string `json:"title"`
		}{
			{TID: 100000001, Title: "Result A"},
			{TID: 100000002, Title: "Result B"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	// Call Get and decode manually to test the decode path.
	body, err := c.Get(context.Background(), srv.URL+"?query=test&page=1&type=all")
	if err != nil {
		t.Fatal(err)
	}
	var resp searchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data.Thread.List) != 2 {
		t.Errorf("got %d results, want 2", len(resp.Data.Thread.List))
	}
	if resp.Data.Thread.List[0].Title != "Result A" {
		t.Errorf("first title = %q, want Result A", resp.Data.Thread.List[0].Title)
	}
}
