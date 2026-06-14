package hupu

import (
	"regexp"
	"strings"
)

// postRe matches post links on the Hupu BBS homepage.
// The (?s) flag lets . match newlines so the span can span lines.
var postRe = regexp.MustCompile(`(?s)<a[^>]+href="/([0-9]+)\.html"[^>]*>.*?<span[^>]*class="t-title"[^>]*>([^<]+)</span>`)

// extractPosts parses post links from a Hupu BBS homepage HTML body.
// It deduplicates by post ID and returns at most limit posts, ranked 1-based.
// If limit <= 0 all found posts are returned.
func extractPosts(body []byte, limit int) []Post {
	matches := postRe.FindAllSubmatch(body, -1)
	seen := make(map[string]struct{})
	var posts []Post
	for _, m := range matches {
		id := string(m[1])
		title := strings.TrimSpace(string(m[2]))
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		rank := len(posts) + 1
		posts = append(posts, Post{
			Rank:  rank,
			ID:    id,
			Title: title,
			URL:   "https://bbs.hupu.com/" + id + ".html",
		})
		if limit > 0 && len(posts) >= limit {
			break
		}
	}
	return posts
}
