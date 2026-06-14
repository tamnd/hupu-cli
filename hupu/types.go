package hupu

// Post is a single discussion thread from the Hupu BBS homepage.
type Post struct {
	Rank  int    `json:"rank"`
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
