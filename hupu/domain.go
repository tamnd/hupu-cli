package hupu

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes hupu as a kit Domain: a driver that a multi-domain
// host enables with a single blank import,
//
//	import _ "github.com/tamnd/hupu-cli/hupu"
//
// The init below registers it; the host then dereferences hupu:// URIs by
// routing to the operations Register installs.
func init() { kit.Register(Domain{}) }

// Domain is the hupu driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "hupu",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "hupu",
			Short:  "A command line for Hupu sports forum.",
			Long: `A command line for Hupu sports forum.

hupu reads public Hupu BBS data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/hupu-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// hot: list the current hot threads from the BBS home page.
	kit.Handle(app, kit.OpMeta{
		Name:    "hot",
		Group:   "read",
		List:    true,
		Summary: "List hot BBS posts",
		URIType: "post",
	}, listHot)

	// search: full-text search via the mobile API.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Search BBS posts",
		URIType: "post",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, listSearch)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type hotInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Query  string  `kit:"arg" help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listHot(ctx context.Context, in hotInput, emit func(*Post) error) error {
	posts, err := in.Client.Hot(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range posts {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func listSearch(ctx context.Context, in searchInput, emit func(*Post) error) error {
	if strings.TrimSpace(in.Query) == "" {
		return errs.Usage("query is required")
	}
	posts, err := in.Client.Search(ctx, in.Query, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range posts {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions, no network ---

// Classify turns a post URL or bare TID into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = postID(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized hupu reference: %q", input)
	}
	return "post", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "post" {
		return "", errs.Usage("hupu has no resource type %q", uriType)
	}
	return fmt.Sprintf("%s/%s.html", baseURL, id), nil
}

// --- helpers ---

// tidRE matches a 9-digit thread id in a URL path.
var tidRE = regexp.MustCompile(`\b(\d{9})\b`)

// postID extracts a thread id from a full URL or a bare tid string.
func postID(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		if m := tidRE.FindStringSubmatch(u.Path); m != nil {
			return m[1]
		}
		return ""
	}
	// bare tid or path
	if m := tidRE.FindStringSubmatch(input); m != nil {
		return m[1]
	}
	return ""
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
