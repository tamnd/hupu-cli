package hupu

// Post is a single discussion thread from the Hupu BBS.
type Post struct {
	Rank  int    `json:"rank"            table:"rank"`
	ID    string `json:"id"  kit:"id"    table:"id"`
	Title string `json:"title"           table:"title"`
	URL   string `json:"url"             table:"url,url"`
}
